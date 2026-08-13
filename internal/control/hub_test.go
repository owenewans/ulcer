package control

import (
	"testing"

	controlv1 "github.com/owenewans/ulcer/gen/control/v1"
)

func TestReplacementConnectionOwnsOfflineTransition(t *testing.T) {
	hub := NewHub()
	oldMessages, detachOld := attachTestConnection(t, hub, "instance")
	_, detachCurrent := attachTestConnection(t, hub, "instance")

	if _, open := <-oldMessages; open {
		t.Fatal("replaced connection channel should be closed")
	}
	if detachOld() {
		t.Fatal("replaced connection must not own the offline transition")
	}
	if !hub.Online("instance") {
		t.Fatal("replacement connection should remain online")
	}
	if !detachCurrent() {
		t.Fatal("current connection should own the offline transition")
	}
	if hub.Online("instance") {
		t.Fatal("detached current connection should be offline")
	}
}

func TestBackpressureDisconnectsWithoutLosingOfflineOwnership(t *testing.T) {
	hub := NewHub()
	reservation, reserved := hub.Reserve("instance")
	if !reserved {
		t.Fatal("connection was not reserved")
	}
	_, disconnected, detach, attached := hub.Activate(reservation)
	if !attached {
		t.Fatal("connection was not attached")
	}
	for index := 0; index < 16; index++ {
		if !hub.Send("instance", nil) {
			t.Fatalf("message %d should fit", index)
		}
	}
	if hub.Send("instance", nil) {
		t.Fatal("overflowing message should force reconnect")
	}
	if hub.Online("instance") {
		t.Fatal("backpressured connection must not be reported online")
	}
	select {
	case <-disconnected:
	default:
		t.Fatal("backpressure did not signal stream termination")
	}
	if !detach() {
		t.Fatal("disconnected stream must retain offline transition ownership")
	}
}

func TestRevokeClosesConnectionAndRefusesStaleReservation(t *testing.T) {
	hub := NewHub()
	messages, detach := attachTestConnection(t, hub, "instance")
	stale, reserved := hub.Reserve("instance")
	if !reserved {
		t.Fatal("stale connection was not reserved")
	}

	release := hub.Revoke("instance")
	if _, open := <-messages; open {
		t.Fatal("revocation did not close the outbound channel")
	}
	if detach() {
		t.Fatal("revoked connection must not own an offline transition")
	}
	if hub.Online("instance") {
		t.Fatal("revoked instance is online")
	}
	if _, reserved := hub.Reserve("instance"); reserved {
		t.Fatal("connection reserved while deletion was in progress")
	}
	release()
	if _, _, _, attached := hub.Activate(stale); attached {
		t.Fatal("stale connection attached after revocation")
	}
	reservation, reserved := hub.Reserve("instance")
	if !reserved {
		t.Fatal("new reservation remained blocked after deletion completed")
	}
	hub.Cancel(reservation)
}

func TestRevokeInterruptsBufferedOutboundMessages(t *testing.T) {
	hub := NewHub()
	reservation, reserved := hub.Reserve("instance")
	if !reserved {
		t.Fatal("connection was not reserved")
	}
	messages, disconnected, _, attached := hub.Activate(reservation)
	if !attached {
		t.Fatal("connection was not attached")
	}
	if !hub.Send("instance", nil) {
		t.Fatal("message was not buffered")
	}
	release := hub.Revoke("instance")
	defer release()
	select {
	case <-disconnected:
	default:
		t.Fatal("revocation did not signal termination")
	}
	if len(messages) != 1 {
		t.Fatalf("buffered message count = %d, want 1", len(messages))
	}
}

func TestIfOnlineRequiresAnActiveUnrevokedConnection(t *testing.T) {
	hub := NewHub()
	called := false
	if hub.IfOnline("instance", func() { called = true }) || called {
		t.Fatal("offline instance ran online action")
	}
	_, detach := attachTestConnection(t, hub, "instance")
	if !hub.IfOnline("instance", func() { called = true }) || !called {
		t.Fatal("online instance did not run action")
	}
	release := hub.Revoke("instance")
	if hub.IfOnline("instance", func() {}) {
		t.Fatal("revoked instance was online")
	}
	release()
	if detach() {
		t.Fatal("revoked connection retained ownership")
	}
}

func attachTestConnection(t *testing.T, hub *Hub, id string) (<-chan *controlv1.HostMessage, func() bool) {
	t.Helper()
	reservation, reserved := hub.Reserve(id)
	if !reserved {
		t.Fatal("connection was not reserved")
	}
	messages, _, detach, attached := hub.Activate(reservation)
	if !attached {
		t.Fatal("connection was not attached")
	}
	return messages, detach
}
