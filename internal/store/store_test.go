package store

import (
	"math"
	"testing"
	"time"

	controlv1 "github.com/owenewans/ulcer/gen/control/v1"
	"github.com/owenewans/ulcer/internal/model"
)

func TestSessionStoresOnlyHashAndExpires(t *testing.T) {
	database := openTestStore(t)
	token := "session-secret"
	if err := database.CreateSession(token, time.Minute); err != nil {
		t.Fatal(err)
	}
	valid, err := database.SessionValid(token)
	if err != nil || !valid {
		t.Fatalf("expected valid session, valid=%v err=%v", valid, err)
	}
	if _, err := database.getString(sessionPrefix + token); err == nil {
		t.Fatal("raw session token was stored as a key")
	}
}

func TestMeterApplicationIsContiguousAndIdempotent(t *testing.T) {
	database := openTestStore(t)
	deltas := []*controlv1.MeterDelta{
		{Sequence: 1, UplinkBytes: 10, DownlinkBytes: 20},
		{Sequence: 2, UplinkBytes: 30, DownlinkBytes: 40},
	}
	ack, err := database.ApplyMeters("instance", deltas)
	if err != nil || ack != 2 {
		t.Fatalf("first apply: ack=%d err=%v", ack, err)
	}
	ack, err = database.ApplyMeters("instance", deltas)
	if err != nil || ack != 2 {
		t.Fatalf("duplicate apply: ack=%d err=%v", ack, err)
	}
	usage, err := database.Usage()
	if err != nil {
		t.Fatal(err)
	}
	if usage != (model.Usage{UplinkBytes: 40, DownlinkBytes: 60}) {
		t.Fatalf("unexpected usage: %+v", usage)
	}

	ack, err = database.ApplyMeters("gap", []*controlv1.MeterDelta{{Sequence: 2, UplinkBytes: 99}})
	if err != nil || ack != 0 {
		t.Fatalf("gap should not advance: ack=%d err=%v", ack, err)
	}
}

func TestInstanceGenerationMutation(t *testing.T) {
	database := openTestStore(t)
	instance := model.Instance{ID: "one", Name: "edge", CreatedAt: time.Now().UTC()}
	if err := database.PutInstance(instance); err != nil {
		t.Fatal(err)
	}
	updated, err := database.UpdateInstance("one", func(instance *model.Instance) error {
		instance.DesiredGeneration++
		instance.DesiredDigest = "digest"
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.DesiredGeneration != 1 || updated.DesiredDigest != "digest" {
		t.Fatalf("unexpected instance: %+v", updated)
	}
}

func TestTOTPStepCanOnlyBeConsumedOnce(t *testing.T) {
	database := openTestStore(t)
	first, err := database.ConsumeTOTPStep(42)
	if err != nil || !first {
		t.Fatalf("first consume: consumed=%v err=%v", first, err)
	}
	second, err := database.ConsumeTOTPStep(42)
	if err != nil || second {
		t.Fatalf("duplicate consume: consumed=%v err=%v", second, err)
	}
	older, err := database.ConsumeTOTPStep(41)
	if err != nil || older {
		t.Fatalf("older consume: consumed=%v err=%v", older, err)
	}
}

func TestMeterApplicationRejectsCounterOverflow(t *testing.T) {
	database := openTestStore(t)
	if _, err := database.ApplyMeters("instance", []*controlv1.MeterDelta{{Sequence: 1, UplinkBytes: math.MaxUint64}}); err != nil {
		t.Fatal(err)
	}
	if _, err := database.ApplyMeters("instance", []*controlv1.MeterDelta{{Sequence: 2, UplinkBytes: 1}}); err == nil {
		t.Fatal("expected traffic counter overflow")
	}
}

func openTestStore(t *testing.T) *Store {
	t.Helper()
	database, err := Open("", true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	return database
}
