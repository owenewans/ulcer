package control

import (
	"testing"

	"github.com/owenewans/ulcer/internal/events"
	"github.com/owenewans/ulcer/internal/model"
	"github.com/owenewans/ulcer/internal/store"
)

func TestMarkOfflinePublishesOnlyWhenInstanceExists(t *testing.T) {
	database, err := store.Open("", true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	eventBus := events.New(database)
	server := NewServer(database, eventBus, NewHub())

	server.markOffline("missing")
	eventHistory, err := database.EventsAfter(0, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(eventHistory) != 0 {
		t.Fatalf("missing instance produced events: %+v", eventHistory)
	}

	if err := database.PutInstance(model.Instance{ID: "present", Phase: "connected"}); err != nil {
		t.Fatal(err)
	}
	release := server.hub.Revoke("present")
	server.markOffline("present")
	release()
	blocked, err := database.Instance("present")
	if err != nil {
		t.Fatal(err)
	}
	if blocked.Phase != "connected" {
		t.Fatalf("revoked instance phase = %q, want connected", blocked.Phase)
	}
	eventHistory, err = database.EventsAfter(0, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(eventHistory) != 0 {
		t.Fatalf("revoked instance produced events: %+v", eventHistory)
	}

	server.markOffline("present")
	updated, err := database.Instance("present")
	if err != nil {
		t.Fatal(err)
	}
	if updated.Phase != "offline" {
		t.Fatalf("phase = %q, want offline", updated.Phase)
	}
	eventHistory, err = database.EventsAfter(0, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(eventHistory) != 1 || eventHistory[0].Type != "instance.disconnected" {
		t.Fatalf("unexpected events: %+v", eventHistory)
	}
}

func TestPublishIfEnrolledSuppressesDeletedAndRevokedInstances(t *testing.T) {
	database, err := store.Open("", true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	server := NewServer(database, events.New(database), NewHub())

	if server.publishIfEnrolled("instance.status", "missing", nil) {
		t.Fatal("missing instance event was published")
	}
	if err := database.PutInstance(model.Instance{ID: "present"}); err != nil {
		t.Fatal(err)
	}
	release := server.hub.Revoke("present")
	if server.publishIfEnrolled("instance.status", "present", nil) {
		t.Fatal("revoked instance event was published")
	}
	release()
	if !server.publishIfEnrolled("instance.status", "present", nil) {
		t.Fatal("enrolled instance event was not published")
	}
	eventHistory, err := database.EventsAfter(0, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(eventHistory) != 1 || eventHistory[0].Type != "instance.status" {
		t.Fatalf("unexpected events: %+v", eventHistory)
	}
}
