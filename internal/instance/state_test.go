package instance

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"

	controlv1 "github.com/owenewans/ulcer/gen/control/v1"
)

func TestStateRejectsDigestChangeAtSameGeneration(t *testing.T) {
	state, err := newStateStore()
	if err != nil {
		t.Fatal(err)
	}
	defer state.close()
	spec := []byte(`{"engine":"xray","artifact":"sha256:one","desired_phase":"stopped","config":{}}`)
	first := &controlv1.DesiredState{
		Generation: 1, SpecDigest: digest(spec), CanonicalSpec: spec,
	}
	if _, err := state.apply(first); err != nil {
		t.Fatal(err)
	}
	first.SpecDigest = "two"
	if _, err := state.apply(first); err == nil {
		t.Fatal("expected digest conflict")
	}
}

func TestUnknownAdapterIsReportedHonestly(t *testing.T) {
	state, err := newStateStore()
	if err != nil {
		t.Fatal(err)
	}
	defer state.close()
	spec := []byte(`{"engine":"xray","artifact":"sha256:one","desired_phase":"running","config":{}}`)
	applied, err := state.apply(&controlv1.DesiredState{
		Generation: 1, SpecDigest: digest(spec), CanonicalSpec: spec,
	})
	if err != nil {
		t.Fatal(err)
	}
	if applied.Phase != "unsupported" {
		t.Fatalf("expected unsupported, got %s", applied.Phase)
	}
}

func digest(value []byte) string {
	sum := sha256.Sum256(value)
	return hex.EncodeToString(sum[:])
}
