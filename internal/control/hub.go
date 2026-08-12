package control

import (
	"sync"

	controlv1 "github.com/owenewans/ulcer/gen/control/v1"
)

type Hub struct {
	mu          sync.Mutex
	connections map[string]*connection
}

type connection struct {
	messages chan *controlv1.HostMessage
	closed   bool
}

func NewHub() *Hub {
	return &Hub{connections: make(map[string]*connection)}
}

func (h *Hub) Attach(id string) (<-chan *controlv1.HostMessage, func() bool) {
	h.mu.Lock()
	if previous, ok := h.connections[id]; ok && !previous.closed {
		close(previous.messages)
		previous.closed = true
	}
	current := &connection{messages: make(chan *controlv1.HostMessage, 16)}
	h.connections[id] = current
	h.mu.Unlock()

	return current.messages, func() bool {
		h.mu.Lock()
		defer h.mu.Unlock()
		if attached, ok := h.connections[id]; ok && attached == current {
			delete(h.connections, id)
			if !current.closed {
				close(current.messages)
				current.closed = true
			}
			return true
		}
		return false
	}
}

func (h *Hub) Send(id string, message *controlv1.HostMessage) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	current, ok := h.connections[id]
	if !ok || current.closed {
		return false
	}
	select {
	case current.messages <- message:
		return true
	default:
		close(current.messages)
		current.closed = true
		return false
	}
}

func (h *Hub) Online(id string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	current, ok := h.connections[id]
	return ok && !current.closed
}
