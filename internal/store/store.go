package store

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/dgraph-io/badger/v4"
	controlv1 "github.com/owenewans/ulcer/gen/control/v1"
	"github.com/owenewans/ulcer/internal/model"
)

var (
	ErrNotFound          = errors.New("not found")
	ErrAlreadyConfigured = errors.New("operator already configured")
)

const (
	operatorReadyKey  = "auth/operator-ready"
	pendingTOTPKey    = "auth/pending-totp"
	totpSecretKey     = "auth/totp-secret"
	totpStepKey       = "auth/totp-step"
	recoveryPrefix    = "auth/recovery/"
	sessionPrefix     = "auth/session/"
	instancePrefix    = "instance/"
	meterAckPrefix    = "meter/ack/"
	usageTotalKey     = "meter/usage/total"
	eventSequenceKey  = "meta/events-sequence"
	eventPrefix       = "events/"
	defaultEventLimit = 500
)

type Store struct {
	db *badger.DB
}

func Open(path string, inMemory bool) (*Store, error) {
	opts := badger.DefaultOptions(path).WithLogger(nil)
	if inMemory {
		opts = opts.WithInMemory(true)
	}
	db, err := badger.Open(opts)
	if err != nil {
		return nil, fmt.Errorf("open badger: %w", err)
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) OperatorReady() (bool, error) {
	var ready bool
	err := s.db.View(func(txn *badger.Txn) error {
		_, err := txn.Get([]byte(operatorReadyKey))
		if errors.Is(err, badger.ErrKeyNotFound) {
			return nil
		}
		ready = err == nil
		return err
	})
	return ready, err
}

func (s *Store) PendingTOTP() (string, error) {
	return s.getString(pendingTOTPKey)
}

func (s *Store) CompleteOperator(secret string, totpStep uint64, recoveryHashes []string, sessionToken string, sessionTTL time.Duration) error {
	return s.db.Update(func(txn *badger.Txn) error {
		if _, err := txn.Get([]byte(operatorReadyKey)); err == nil {
			return ErrAlreadyConfigured
		} else if !errors.Is(err, badger.ErrKeyNotFound) {
			return err
		}
		if err := txn.Set([]byte(totpSecretKey), []byte(secret)); err != nil {
			return err
		}
		if err := setUint64(txn, []byte(totpStepKey), totpStep); err != nil {
			return err
		}
		if err := txn.Set([]byte(operatorReadyKey), []byte{1}); err != nil {
			return err
		}
		if err := txn.Delete([]byte(pendingTOTPKey)); err != nil {
			return err
		}
		for _, hash := range recoveryHashes {
			if err := txn.Set([]byte(recoveryPrefix+hash), []byte{1}); err != nil {
				return err
			}
		}
		return txn.SetEntry(badger.NewEntry([]byte(sessionPrefix+SessionHash(sessionToken)), []byte{1}).WithTTL(sessionTTL))
	})
}

func (s *Store) TOTPSecret() (string, error) {
	return s.getString(totpSecretKey)
}

func (s *Store) ConsumeTOTPStep(step uint64) (bool, error) {
	consumed := false
	err := s.db.Update(func(txn *badger.Txn) error {
		key := []byte(totpStepKey)
		if item, err := txn.Get(key); err == nil {
			var previous uint64
			if err := item.Value(func(data []byte) error {
				if len(data) != 8 {
					return fmt.Errorf("invalid TOTP step")
				}
				previous = binary.BigEndian.Uint64(data)
				return nil
			}); err != nil {
				return err
			}
			if step <= previous {
				return nil
			}
		} else if !errors.Is(err, badger.ErrKeyNotFound) {
			return err
		}
		if err := setUint64(txn, key, step); err != nil {
			return err
		}
		consumed = true
		return nil
	})
	return consumed, err
}

func (s *Store) SetPendingTOTP(secret string) error {
	return s.db.Update(func(txn *badger.Txn) error {
		if err := txn.Delete([]byte(totpStepKey)); err != nil {
			return err
		}
		return txn.Set([]byte(pendingTOTPKey), []byte(secret))
	})
}

func (s *Store) ConsumeRecoveryCode(code string) (bool, error) {
	hash := sha256.Sum256([]byte(strings.ToUpper(strings.TrimSpace(code))))
	key := []byte(recoveryPrefix + hex.EncodeToString(hash[:]))
	found := false
	err := s.db.Update(func(txn *badger.Txn) error {
		if _, err := txn.Get(key); errors.Is(err, badger.ErrKeyNotFound) {
			return nil
		} else if err != nil {
			return err
		}
		found = true
		return txn.Delete(key)
	})
	return found, err
}

func SessionHash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func (s *Store) CreateSession(token string, ttl time.Duration) error {
	entry := badger.NewEntry([]byte(sessionPrefix+SessionHash(token)), []byte{1}).WithTTL(ttl)
	return s.db.Update(func(txn *badger.Txn) error { return txn.SetEntry(entry) })
}

func (s *Store) SessionValid(token string) (bool, error) {
	if token == "" {
		return false, nil
	}
	valid := false
	err := s.db.View(func(txn *badger.Txn) error {
		_, err := txn.Get([]byte(sessionPrefix + SessionHash(token)))
		if errors.Is(err, badger.ErrKeyNotFound) {
			return nil
		}
		valid = err == nil
		return err
	})
	return valid, err
}

func (s *Store) DeleteSession(token string) error {
	return s.db.Update(func(txn *badger.Txn) error {
		return txn.Delete([]byte(sessionPrefix + SessionHash(token)))
	})
}

func (s *Store) PutInstance(instance model.Instance) error {
	value, err := json.Marshal(instance)
	if err != nil {
		return err
	}
	return s.db.Update(func(txn *badger.Txn) error {
		return txn.Set([]byte(instancePrefix+instance.ID), value)
	})
}

func (s *Store) DeleteInstance(id string) (model.Instance, error) {
	var deleted model.Instance
	for range 8 {
		deleted = model.Instance{}
		err := s.db.Update(func(txn *badger.Txn) error {
			key := []byte(instancePrefix + id)
			item, err := txn.Get(key)
			if errors.Is(err, badger.ErrKeyNotFound) {
				return ErrNotFound
			}
			if err != nil {
				return err
			}
			if err := item.Value(func(value []byte) error { return json.Unmarshal(value, &deleted) }); err != nil {
				return err
			}
			if err := txn.Delete(key); err != nil {
				return err
			}
			return txn.Delete([]byte(meterAckPrefix + id))
		})
		if !errors.Is(err, badger.ErrConflict) {
			return deleted, err
		}
	}
	return model.Instance{}, badger.ErrConflict
}

func (s *Store) Instance(id string) (model.Instance, error) {
	var instance model.Instance
	err := s.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(instancePrefix + id))
		if errors.Is(err, badger.ErrKeyNotFound) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		return item.Value(func(value []byte) error { return json.Unmarshal(value, &instance) })
	})
	return instance, err
}

func (s *Store) Instances() ([]model.Instance, error) {
	instances := make([]model.Instance, 0)
	err := s.db.View(func(txn *badger.Txn) error {
		iterator := txn.NewIterator(badger.DefaultIteratorOptions)
		defer iterator.Close()
		prefix := []byte(instancePrefix)
		for iterator.Seek(prefix); iterator.ValidForPrefix(prefix); iterator.Next() {
			var instance model.Instance
			if err := iterator.Item().Value(func(value []byte) error {
				return json.Unmarshal(value, &instance)
			}); err != nil {
				return err
			}
			instances = append(instances, instance)
		}
		return nil
	})
	slices.SortFunc(instances, func(a, b model.Instance) int { return a.CreatedAt.Compare(b.CreatedAt) })
	return instances, err
}

func (s *Store) UpdateInstance(id string, mutate func(*model.Instance) error) (model.Instance, error) {
	var updated model.Instance
	err := s.db.Update(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(instancePrefix + id))
		if errors.Is(err, badger.ErrKeyNotFound) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if err := item.Value(func(value []byte) error { return json.Unmarshal(value, &updated) }); err != nil {
			return err
		}
		if err := mutate(&updated); err != nil {
			return err
		}
		value, err := json.Marshal(updated)
		if err != nil {
			return err
		}
		return txn.Set([]byte(instancePrefix+id), value)
	})
	if errors.Is(err, badger.ErrConflict) {
		if _, lookupErr := s.Instance(id); errors.Is(lookupErr, ErrNotFound) {
			return updated, ErrNotFound
		}
	}
	return updated, err
}

func (s *Store) ApplyMeters(instanceID string, deltas []*controlv1.MeterDelta) (uint64, error) {
	var acknowledged uint64
	err := s.db.Update(func(txn *badger.Txn) error {
		if _, err := txn.Get([]byte(instancePrefix + instanceID)); errors.Is(err, badger.ErrKeyNotFound) {
			return ErrNotFound
		} else if err != nil {
			return err
		}
		ackKey := []byte(meterAckPrefix + instanceID)
		acknowledged, _ = getUint64(txn, ackKey)
		usage, err := getUsage(txn)
		if err != nil {
			return err
		}
		for _, delta := range deltas {
			if delta == nil {
				return fmt.Errorf("meter delta is nil")
			}
			if delta.Sequence <= acknowledged {
				continue
			}
			if delta.Sequence != acknowledged+1 {
				break
			}
			if ^uint64(0)-usage.UplinkBytes < delta.UplinkBytes || ^uint64(0)-usage.DownlinkBytes < delta.DownlinkBytes {
				return fmt.Errorf("traffic counter overflow")
			}
			usage.UplinkBytes += delta.UplinkBytes
			usage.DownlinkBytes += delta.DownlinkBytes
			acknowledged = delta.Sequence
		}
		if err := setUint64(txn, ackKey, acknowledged); err != nil {
			return err
		}
		value, err := json.Marshal(usage)
		if err != nil {
			return err
		}
		return txn.Set([]byte(usageTotalKey), value)
	})
	if errors.Is(err, badger.ErrConflict) {
		if _, lookupErr := s.Instance(instanceID); errors.Is(lookupErr, ErrNotFound) {
			return acknowledged, ErrNotFound
		}
	}
	return acknowledged, err
}

func (s *Store) Usage() (model.Usage, error) {
	var usage model.Usage
	err := s.db.View(func(txn *badger.Txn) error {
		var err error
		usage, err = getUsage(txn)
		return err
	})
	return usage, err
}

func (s *Store) AppendEvent(event model.Event) (model.Event, error) {
	err := s.db.Update(func(txn *badger.Txn) error {
		sequence, _ := getUint64(txn, []byte(eventSequenceKey))
		sequence++
		event.ID = sequence
		if event.At.IsZero() {
			event.At = time.Now().UTC()
		}
		value, err := json.Marshal(event)
		if err != nil {
			return err
		}
		if err := txn.Set(eventKey(sequence), value); err != nil {
			return err
		}
		if err := setUint64(txn, []byte(eventSequenceKey), sequence); err != nil {
			return err
		}
		if sequence > defaultEventLimit {
			return txn.Delete(eventKey(sequence - defaultEventLimit))
		}
		return nil
	})
	return event, err
}

func (s *Store) EventsAfter(after uint64, limit int) ([]model.Event, error) {
	if limit <= 0 || limit > defaultEventLimit {
		limit = defaultEventLimit
	}
	events := make([]model.Event, 0, limit)
	err := s.db.View(func(txn *badger.Txn) error {
		iterator := txn.NewIterator(badger.DefaultIteratorOptions)
		defer iterator.Close()
		start := eventKey(after + 1)
		prefix := []byte(eventPrefix)
		for iterator.Seek(start); iterator.ValidForPrefix(prefix) && len(events) < limit; iterator.Next() {
			var event model.Event
			if err := iterator.Item().Value(func(value []byte) error {
				return json.Unmarshal(value, &event)
			}); err != nil {
				return err
			}
			events = append(events, event)
		}
		return nil
	})
	return events, err
}

func (s *Store) getString(key string) (string, error) {
	var value string
	err := s.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(key))
		if errors.Is(err, badger.ErrKeyNotFound) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		return item.Value(func(data []byte) error {
			value = string(data)
			return nil
		})
	})
	return value, err
}

func eventKey(sequence uint64) []byte {
	return []byte(fmt.Sprintf("%s%020d", eventPrefix, sequence))
}

func getUint64(txn *badger.Txn, key []byte) (uint64, error) {
	item, err := txn.Get(key)
	if errors.Is(err, badger.ErrKeyNotFound) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	var value uint64
	err = item.Value(func(data []byte) error {
		if len(data) != 8 {
			return fmt.Errorf("invalid uint64 at %q", key)
		}
		value = binary.BigEndian.Uint64(data)
		return nil
	})
	return value, err
}

func setUint64(txn *badger.Txn, key []byte, value uint64) error {
	data := make([]byte, 8)
	binary.BigEndian.PutUint64(data, value)
	return txn.Set(key, data)
}

func getUsage(txn *badger.Txn) (model.Usage, error) {
	var usage model.Usage
	item, err := txn.Get([]byte(usageTotalKey))
	if errors.Is(err, badger.ErrKeyNotFound) {
		return usage, nil
	}
	if err != nil {
		return usage, err
	}
	err = item.Value(func(value []byte) error { return json.Unmarshal(value, &usage) })
	return usage, err
}
