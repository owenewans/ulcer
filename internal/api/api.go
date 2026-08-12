package api

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	controlv1 "github.com/owenewans/ulcer/gen/control/v1"
	"github.com/owenewans/ulcer/internal/auth"
	"github.com/owenewans/ulcer/internal/control"
	"github.com/owenewans/ulcer/internal/events"
	"github.com/owenewans/ulcer/internal/model"
	"github.com/owenewans/ulcer/internal/pki"
	"github.com/owenewans/ulcer/internal/store"
	"github.com/owenewans/ulcer/versions"
)

const sessionCookie = "ulcer_session"

type API struct {
	store     *store.Store
	auth      *auth.Service
	authority *pki.Authority
	events    *events.Bus
	hub       *control.Hub
	logger    *slog.Logger
	limiter   loginLimiter
}

func New(store *store.Store, auth *auth.Service, authority *pki.Authority, events *events.Bus, hub *control.Hub, logger *slog.Logger) http.Handler {
	api := &API{store: store, auth: auth, authority: authority, events: events, hub: hub, logger: logger}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", api.health)
	mux.HandleFunc("GET /api/v1/bootstrap/status", api.bootstrapStatus)
	mux.HandleFunc("POST /api/v1/bootstrap/start", api.bootstrapStart)
	mux.HandleFunc("POST /api/v1/bootstrap/complete", api.bootstrapComplete)
	mux.HandleFunc("POST /api/v1/auth/login", api.login)
	mux.HandleFunc("POST /api/v1/auth/logout", api.requireAuth(api.logout))
	mux.HandleFunc("GET /api/v1/auth/session", api.session)
	mux.HandleFunc("GET /api/v1/dashboard", api.requireAuth(api.dashboard))
	mux.HandleFunc("GET /api/v1/engines", api.requireAuth(api.engines))
	mux.HandleFunc("GET /api/v1/instances", api.requireAuth(api.instances))
	mux.HandleFunc("POST /api/v1/instances", api.requireAuth(api.createInstance))
	mux.HandleFunc("GET /api/v1/instances/{id}", api.requireAuth(api.instance))
	mux.HandleFunc("PUT /api/v1/instances/{id}/desired", api.requireAuth(api.setDesired))
	mux.HandleFunc("GET /api/v1/events", api.requireAuth(api.eventStream))
	return api.securityHeaders(api.recoverPanic(api.accessLog(mux)))
}

func (a *API) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (a *API) bootstrapStatus(w http.ResponseWriter, _ *http.Request) {
	ready, err := a.store.OperatorReady()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "store_error", "could not read operator state")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ready": ready})
}

func (a *API) bootstrapStart(w http.ResponseWriter, r *http.Request) {
	if !a.allowAuthentication(w) {
		return
	}
	ready, err := a.store.OperatorReady()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "store_error", "could not read operator state")
		return
	}
	if ready {
		writeError(w, http.StatusConflict, "already_configured", "operator is already configured")
		return
	}
	var request struct {
		Token string `json:"token"`
	}
	if !decodeJSON(w, r, &request) {
		return
	}
	if !a.auth.CheckSetupToken(request.Token) {
		a.limiter.failed(time.Now())
		writeError(w, http.StatusUnauthorized, "invalid_setup_token", "setup token is invalid")
		return
	}
	secret, uri, err := a.auth.BeginSetup()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "setup_failed", "could not create totp secret")
		return
	}
	a.limiter.succeeded()
	writeJSON(w, http.StatusOK, map[string]string{"secret": secret, "uri": uri})
}

func (a *API) bootstrapComplete(w http.ResponseWriter, r *http.Request) {
	if !a.allowAuthentication(w) {
		return
	}
	ready, err := a.store.OperatorReady()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "store_error", "could not read operator state")
		return
	}
	if ready {
		writeError(w, http.StatusConflict, "already_configured", "operator is already configured")
		return
	}
	var request struct {
		Token string `json:"token"`
		Code  string `json:"code"`
	}
	if !decodeJSON(w, r, &request) {
		return
	}
	if !a.auth.CheckSetupToken(request.Token) {
		a.limiter.failed(time.Now())
		writeError(w, http.StatusUnauthorized, "invalid_setup_token", "setup token is invalid")
		return
	}
	codes, session, err := a.auth.CompleteSetup(request.Code)
	if err != nil {
		if errors.Is(err, store.ErrAlreadyConfigured) {
			writeError(w, http.StatusConflict, "already_configured", "operator is already configured")
			return
		}
		if errors.Is(err, auth.ErrInvalidCode) {
			a.limiter.failed(time.Now())
			writeError(w, http.StatusUnauthorized, "invalid_totp", "totp code is invalid")
			return
		}
		a.logger.Error("complete operator setup", "error", err)
		writeError(w, http.StatusInternalServerError, "setup_failed", "could not complete operator setup")
		return
	}
	a.limiter.succeeded()
	setSessionCookie(w, r, session)
	_, _ = a.events.Publish(model.Event{Type: "operator.configured"})
	writeJSON(w, http.StatusCreated, map[string]any{"recovery_codes": codes, "expires_in": int(auth.SessionTTL.Seconds())})
}

func (a *API) login(w http.ResponseWriter, r *http.Request) {
	if !a.allowAuthentication(w) {
		return
	}
	var request struct {
		Code string `json:"code"`
	}
	if !decodeJSON(w, r, &request) {
		return
	}
	session, err := a.auth.Login(request.Code)
	if err != nil {
		if errors.Is(err, auth.ErrInvalidCode) {
			a.limiter.failed(time.Now())
			writeError(w, http.StatusUnauthorized, "invalid_code", "authentication code is invalid")
			return
		}
		a.logger.Error("authenticate operator", "error", err)
		writeError(w, http.StatusInternalServerError, "auth_failed", "could not authenticate operator")
		return
	}
	a.limiter.succeeded()
	setSessionCookie(w, r, session)
	writeJSON(w, http.StatusOK, map[string]any{"authenticated": true, "expires_in": int(auth.SessionTTL.Seconds())})
}

func (a *API) logout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(sessionCookie); err == nil {
		_ = a.store.DeleteSession(cookie.Value)
	}
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Path: "/", MaxAge: -1, HttpOnly: true, SameSite: http.SameSiteStrictMode, Secure: requestIsSecure(r)})
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) session(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]bool{"authenticated": a.authenticated(r)})
}

func (a *API) dashboard(w http.ResponseWriter, _ *http.Request) {
	instances, err := a.store.Instances()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "store_error", "could not list instances")
		return
	}
	usage, err := a.store.Usage()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "store_error", "could not read traffic")
		return
	}
	var dashboard model.Dashboard
	dashboard.Now = time.Now().UTC()
	dashboard.Traffic = usage
	dashboard.Instances.Total = len(instances)
	for _, instance := range instances {
		if a.hub.Online(instance.ID) {
			dashboard.Instances.Online++
		}
		switch instance.Phase {
		case "running":
			dashboard.Instances.Running++
		case "failed":
			dashboard.Instances.Failed++
		}
	}
	writeJSON(w, http.StatusOK, dashboard)
}

func (a *API) engines(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(versions.EnginesJSON)
}

func (a *API) instances(w http.ResponseWriter, _ *http.Request) {
	instances, err := a.store.Instances()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "store_error", "could not list instances")
		return
	}
	type view struct {
		model.Instance
		Online bool `json:"online"`
	}
	response := make([]view, 0, len(instances))
	for _, instance := range instances {
		response = append(response, view{Instance: instance, Online: a.hub.Online(instance.ID)})
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": response})
}

func (a *API) createInstance(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Name string `json:"name"`
	}
	if !decodeJSON(w, r, &request) {
		return
	}
	request.Name = strings.TrimSpace(request.Name)
	if request.Name == "" || len(request.Name) > 80 {
		writeError(w, http.StatusBadRequest, "invalid_name", "name must contain 1 to 80 characters")
		return
	}
	id := uuid.NewString()
	instance := model.Instance{ID: id, Name: request.Name, CreatedAt: time.Now().UTC(), Capabilities: []string{}, Phase: "enrolled"}
	bundle, err := a.authority.IssueInstance(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "certificate_error", "could not issue instance certificate")
		return
	}
	if err := a.store.PutInstance(instance); err != nil {
		writeError(w, http.StatusInternalServerError, "store_error", "could not create instance")
		return
	}
	_, _ = a.events.Publish(model.Event{Type: "instance.enrolled", ResourceID: id, Data: map[string]any{"name": request.Name}})
	writeJSON(w, http.StatusCreated, map[string]any{"instance": instance, "credentials": bundle})
}

func (a *API) instance(w http.ResponseWriter, r *http.Request) {
	instance, err := a.store.Instance(r.PathValue("id"))
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "instance does not exist")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "store_error", "could not read instance")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"instance": instance, "online": a.hub.Online(instance.ID)})
}

func (a *API) setDesired(w http.ResponseWriter, r *http.Request) {
	var spec model.InstanceSpec
	if !decodeJSON(w, r, &spec) {
		return
	}
	if spec.Engine == "" || spec.Artifact == "" || (spec.DesiredPhase != "running" && spec.DesiredPhase != "stopped") {
		writeError(w, http.StatusBadRequest, "invalid_spec", "engine, artifact and a running/stopped desired phase are required")
		return
	}
	if spec.DesiredPhase == "running" {
		writeError(w, http.StatusUnprocessableEntity, "adapter_unavailable", "no runtime adapter is enabled in this foundation release")
		return
	}
	canonical, digest, err := model.CanonicalSpec(spec)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_spec", "spec could not be canonicalized")
		return
	}
	instance, err := a.store.UpdateInstance(r.PathValue("id"), func(instance *model.Instance) error {
		if subtle.ConstantTimeCompare([]byte(instance.DesiredDigest), []byte(digest)) == 1 {
			return nil
		}
		instance.DesiredGeneration++
		instance.DesiredDigest = digest
		instance.DesiredSpec = canonical
		return nil
	})
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "instance does not exist")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "store_error", "could not update desired state")
		return
	}
	delivered := a.hub.Send(instance.ID, &controlv1.HostMessage{Body: &controlv1.HostMessage_Desired{Desired: &controlv1.DesiredState{
		InstanceId: instance.ID, Generation: instance.DesiredGeneration, SpecDigest: instance.DesiredDigest, CanonicalSpec: instance.DesiredSpec,
	}}})
	_, _ = a.events.Publish(model.Event{Type: "instance.desired", ResourceID: instance.ID, Data: map[string]any{"generation": instance.DesiredGeneration, "delivered": delivered}})
	writeJSON(w, http.StatusOK, map[string]any{"instance": instance, "delivered": delivered})
}

func (a *API) eventStream(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "stream_unsupported", "streaming is not supported")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache, no-transform")
	w.Header().Set("X-Accel-Buffering", "no")
	after := uint64(0)
	if header := r.Header.Get("Last-Event-ID"); header != "" {
		after, _ = strconv.ParseUint(header, 10, 64)
	}
	channel, unsubscribe := a.events.Subscribe()
	defer unsubscribe()
	history, err := a.store.EventsAfter(after, 500)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "store_error", "could not read events")
		return
	}
	for _, event := range history {
		writeEvent(w, event)
		after = event.ID
	}
	flusher.Flush()
	keepalive := time.NewTicker(20 * time.Second)
	defer keepalive.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case event, ok := <-channel:
			if !ok {
				return
			}
			if event.ID <= after {
				continue
			}
			writeEvent(w, event)
			after = event.ID
			flusher.Flush()
		case <-keepalive.C:
			_, _ = fmt.Fprint(w, ": keepalive\n\n")
			flusher.Flush()
		}
	}
}

func (a *API) authenticated(r *http.Request) bool {
	cookie, err := r.Cookie(sessionCookie)
	if err != nil {
		return false
	}
	valid, err := a.store.SessionValid(cookie.Value)
	return err == nil && valid
}

func (a *API) allowAuthentication(w http.ResponseWriter) bool {
	if a.limiter.allowed(time.Now()) {
		return true
	}
	w.Header().Set("Retry-After", "30")
	writeError(w, http.StatusTooManyRequests, "rate_limited", "too many failed authentication attempts")
	return false
}

func (a *API) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !a.authenticated(r) {
			writeError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
			return
		}
		next(w, r)
	}
}

func (a *API) securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			w.Header().Set("Cache-Control", "no-store")
		}
		w.Header().Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		next.ServeHTTP(w, r)
	})
}

func (a *API) accessLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		next.ServeHTTP(w, r)
		method := strings.NewReplacer("\r", "", "\n", "").Replace(r.Method)
		path := strings.NewReplacer("\r", "", "\n", "").Replace(r.URL.Path)
		a.logger.Debug("http request", "method", method, "path", path, "duration", time.Since(started))
	})
}

func (a *API) recoverPanic(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if recovered := recover(); recovered != nil {
				a.logger.Error("http panic", "error", recovered)
				writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func decodeJSON(w http.ResponseWriter, r *http.Request, destination any) bool {
	defer r.Body.Close()
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "request body must be valid JSON")
		return false
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "invalid_json", "request body must contain one JSON value")
		return false
	}
	return true
}

func setSessionCookie(w http.ResponseWriter, r *http.Request, token string) {
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookie, Value: token, Path: "/", MaxAge: int(auth.SessionTTL.Seconds()),
		HttpOnly: true, SameSite: http.SameSiteStrictMode, Secure: requestIsSecure(r),
	})
}

func requestIsSecure(r *http.Request) bool {
	return r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}

func writeEvent(w http.ResponseWriter, event model.Event) {
	data, _ := json.Marshal(event)
	_, _ = fmt.Fprintf(w, "id: %d\nevent: %s\ndata: %s\n\n", event.ID, event.Type, data)
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{"error": map[string]string{"code": code, "message": message}})
}
