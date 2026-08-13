package api

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	controlv1 "github.com/owenewans/ulcer/gen/control/v1"
	"github.com/owenewans/ulcer/internal/config"
	"github.com/owenewans/ulcer/internal/control"
	"github.com/owenewans/ulcer/internal/events"
	"github.com/owenewans/ulcer/internal/model"
	"github.com/owenewans/ulcer/internal/pki"
	"github.com/owenewans/ulcer/internal/store"
)

const testSession = "test-session"

func TestDeleteInstanceRouteRevokesAndPublishes(t *testing.T) {
	handler, database, eventBus, hub := newTestAPI(t, config.Host{PublicName: "localhost", PublicGRPC: "localhost:8443"})
	instance := model.Instance{ID: "instance-id", Name: "edge-01", CreatedAt: time.Now().UTC(), Phase: "connected"}
	if err := database.PutInstance(instance); err != nil {
		t.Fatal(err)
	}
	if _, err := database.ApplyMeters(instance.ID, []*controlv1.MeterDelta{{Sequence: 1, UplinkBytes: 10, DownlinkBytes: 20}}); err != nil {
		t.Fatal(err)
	}
	reservation, reserved := hub.Reserve(instance.ID)
	if !reserved {
		t.Fatal("connection was not reserved")
	}
	outbound, disconnected, detach, attached := hub.Activate(reservation)
	if !attached {
		t.Fatal("connection was not attached")
	}
	_, unsubscribe := eventBus.Subscribe()
	defer unsubscribe()

	response := serveAuthenticated(handler, http.MethodDelete, "/api/v1/instances/"+instance.ID, "")
	if response.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d, body = %s", response.Code, response.Body.String())
	}
	if response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("Cache-Control = %q", response.Header().Get("Cache-Control"))
	}
	if _, open := <-outbound; open {
		t.Fatal("delete did not close the active stream")
	}
	select {
	case <-disconnected:
	default:
		t.Fatal("delete did not signal stream termination")
	}
	if detach() {
		t.Fatal("deleted connection retained offline ownership")
	}
	if _, err := database.Instance(instance.ID); err != store.ErrNotFound {
		t.Fatalf("instance lookup error = %v", err)
	}
	usage, err := database.Usage()
	if err != nil {
		t.Fatal(err)
	}
	if usage != (model.Usage{UplinkBytes: 10, DownlinkBytes: 20}) {
		t.Fatalf("usage changed: %+v", usage)
	}
	eventHistory, err := database.EventsAfter(0, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(eventHistory) != 1 || eventHistory[0].Type != "instance.deleted" || eventHistory[0].ResourceID != instance.ID {
		t.Fatalf("unexpected events: %+v", eventHistory)
	}

	response = serveAuthenticated(handler, http.MethodDelete, "/api/v1/instances/"+instance.ID, "")
	if response.Code != http.StatusNotFound {
		t.Fatalf("second delete status = %d, body = %s", response.Code, response.Body.String())
	}
}

func TestMetaRouteReturnsExactAuthenticatedContract(t *testing.T) {
	image := "registry.example/ulcer-instance@sha256:" + strings.Repeat("a", 64)
	handler, _, _, _ := newTestAPI(t, config.Host{
		PublicName: "host.example", PublicGRPC: "host.example:8443", InstanceImage: image,
	})

	unauthenticated := httptest.NewRecorder()
	handler.ServeHTTP(unauthenticated, httptest.NewRequest(http.MethodGet, "/api/v1/meta", nil))
	if unauthenticated.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated status = %d", unauthenticated.Code)
	}

	response := serveAuthenticated(handler, http.MethodGet, "/api/v1/meta", "")
	if response.Code != http.StatusOK {
		t.Fatalf("meta status = %d, body = %s", response.Code, response.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	expectedKeys := []string{
		"version", "revision", "source_ref", "source_url", "license_url", "grpc_endpoint",
		"grpc_server_name", "instance_image", "ssh_install_available",
	}
	if len(payload) != len(expectedKeys) {
		t.Fatalf("meta has unexpected fields: %+v", payload)
	}
	for _, key := range expectedKeys {
		if _, exists := payload[key]; !exists {
			t.Errorf("meta is missing %q", key)
		}
	}
	if payload["grpc_endpoint"] != "host.example:8443" || payload["grpc_server_name"] != "host.example" {
		t.Fatalf("unexpected gRPC metadata: %+v", payload)
	}
	if payload["instance_image"] != image || payload["ssh_install_available"] != true {
		t.Fatalf("unexpected image metadata: %+v", payload)
	}
	if !strings.Contains(payload["source_url"].(string), "github.com/owenewans/ulcer") {
		t.Fatalf("unexpected source URL: %+v", payload)
	}
	if response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("Cache-Control = %q", response.Header().Get("Cache-Control"))
	}
}

func TestSSHRoutesRejectBlockedTargetAndUnavailableImageWithoutSecrets(t *testing.T) {
	handler, _, _, _ := newTestAPI(t, config.Host{PublicName: "localhost", PublicGRPC: "localhost:8443"})
	response := serveAuthenticated(handler, http.MethodPost, "/api/v1/ssh/host-key", `{"host":"127.0.0.1","port":22}`)
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "target") {
		t.Fatalf("host key response = %d %s", response.Code, response.Body.String())
	}

	const secret = "never-return-this-password"
	response = serveAuthenticated(handler, http.MethodPost, "/api/v1/instances/ssh", `{"name":"edge-01","host":"192.168.1.10","port":22,"user":"root","host_key_sha256":"SHA256:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA","password":"`+secret+`"}`)
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("install response = %d %s", response.Code, response.Body.String())
	}
	if strings.Contains(response.Body.String(), secret) {
		t.Fatal("SSH error response contained the password")
	}
}

func TestSSHInstallAvailabilityRequiresValidRuntimeMetadata(t *testing.T) {
	image := "registry.example/ulcer-instance@sha256:" + strings.Repeat("a", 64)
	for _, test := range []struct {
		name          string
		configuration config.Host
	}{
		{"empty image", config.Host{PublicName: "host.example", PublicGRPC: "host.example:8443"}},
		{"tagged image", config.Host{PublicName: "host.example", PublicGRPC: "host.example:8443", InstanceImage: "registry.example/instance:latest"}},
		{"invalid endpoint", config.Host{PublicName: "host.example", PublicGRPC: "https://host.example:8443", InstanceImage: image}},
	} {
		t.Run(test.name, func(t *testing.T) {
			handler, _, _, _ := newTestAPI(t, test.configuration)
			response := serveAuthenticated(handler, http.MethodGet, "/api/v1/meta", "")
			var payload struct {
				SSHInstallAvailable bool `json:"ssh_install_available"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
				t.Fatal(err)
			}
			if payload.SSHInstallAvailable {
				t.Fatal("SSH installation was incorrectly available")
			}
		})
	}
}

func newTestAPI(t *testing.T, configuration config.Host) (http.Handler, *store.Store, *events.Bus, *control.Hub) {
	t.Helper()
	database, err := store.Open("", true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if err := database.CreateSession(testSession, time.Minute); err != nil {
		t.Fatal(err)
	}
	authority, _, err := pki.Ensure(t.TempDir(), "localhost")
	if err != nil {
		t.Fatal(err)
	}
	eventBus := events.New(database)
	hub := control.NewHub()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return New(database, nil, authority, eventBus, hub, logger, configuration), database, eventBus, hub
}

func serveAuthenticated(handler http.Handler, method, path, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	request.AddCookie(&http.Cookie{Name: sessionCookie, Value: testSession})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}
