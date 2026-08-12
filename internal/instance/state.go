package instance

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"

	"github.com/dgraph-io/badger/v4"
	controlv1 "github.com/owenewans/ulcer/gen/control/v1"
)

const stateKey = "runtime/state"

type runtimeState struct {
	AppliedGeneration uint64 `json:"applied_generation"`
	SpecDigest        string `json:"spec_digest"`
	Phase             string `json:"phase"`
	Reason            string `json:"reason,omitempty"`
}

type stateStore struct {
	db *badger.DB
}

func newStateStore() (*stateStore, error) {
	db, err := badger.Open(badger.DefaultOptions("").WithInMemory(true).WithLogger(nil))
	if err != nil {
		return nil, err
	}
	return &stateStore{db: db}, nil
}

func (s *stateStore) close() error {
	return s.db.Close()
}

func (s *stateStore) load() (runtimeState, error) {
	state := runtimeState{Phase: "idle"}
	err := s.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(stateKey))
		if errors.Is(err, badger.ErrKeyNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		return item.Value(func(value []byte) error { return json.Unmarshal(value, &state) })
	})
	return state, err
}

func (s *stateStore) apply(desired *controlv1.DesiredState) (runtimeState, error) {
	state, err := s.load()
	if err != nil {
		return runtimeState{}, err
	}
	if desired.Generation < state.AppliedGeneration {
		return state, errors.New("desired generation regressed")
	}
	if desired.Generation == state.AppliedGeneration && state.SpecDigest != "" && desired.SpecDigest != state.SpecDigest {
		return state, errors.New("digest changed without a new generation")
	}
	digest := sha256.Sum256(desired.CanonicalSpec)
	if hex.EncodeToString(digest[:]) != desired.SpecDigest {
		return state, errors.New("canonical spec does not match its digest")
	}
	var spec struct {
		Engine       string          `json:"engine"`
		Artifact     string          `json:"artifact"`
		DesiredPhase string          `json:"desired_phase"`
		Config       json.RawMessage `json:"config"`
	}
	if err := json.Unmarshal(desired.CanonicalSpec, &spec); err != nil {
		return state, err
	}
	state.AppliedGeneration = desired.Generation
	state.SpecDigest = desired.SpecDigest
	state.Phase = "unsupported"
	state.Reason = "adapter " + spec.Engine + " is catalogued but not enabled in this build"
	if spec.DesiredPhase == "stopped" {
		state.Phase = "stopped"
		state.Reason = ""
	}
	value, err := json.Marshal(state)
	if err != nil {
		return runtimeState{}, err
	}
	err = s.db.Update(func(txn *badger.Txn) error { return txn.Set([]byte(stateKey), value) })
	return state, err
}
