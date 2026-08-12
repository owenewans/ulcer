package events

import (
	"sync"

	"github.com/owenewans/ulcer/internal/model"
	"github.com/owenewans/ulcer/internal/store"
)

type Bus struct {
	store *store.Store
	mu    sync.Mutex
	next  uint64
	subs  map[uint64]chan model.Event
}

func New(store *store.Store) *Bus {
	return &Bus{store: store, subs: make(map[uint64]chan model.Event)}
}

func (b *Bus) Publish(event model.Event) (model.Event, error) {
	event, err := b.store.AppendEvent(event)
	if err != nil {
		return model.Event{}, err
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	for id, subscriber := range b.subs {
		select {
		case subscriber <- event:
		default:
			delete(b.subs, id)
			close(subscriber)
		}
	}
	return event, nil
}

func (b *Bus) Subscribe() (<-chan model.Event, func()) {
	b.mu.Lock()
	id := b.next
	b.next++
	channel := make(chan model.Event, 32)
	b.subs[id] = channel
	b.mu.Unlock()

	return channel, func() {
		b.mu.Lock()
		if subscriber, ok := b.subs[id]; ok {
			delete(b.subs, id)
			close(subscriber)
		}
		b.mu.Unlock()
	}
}
