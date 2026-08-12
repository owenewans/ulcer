package control

import "testing"

func TestReplacementConnectionOwnsOfflineTransition(t *testing.T) {
	hub := NewHub()
	oldMessages, detachOld := hub.Attach("instance")
	_, detachCurrent := hub.Attach("instance")

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
	_, detach := hub.Attach("instance")
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
	if !detach() {
		t.Fatal("disconnected stream must retain offline transition ownership")
	}
}
