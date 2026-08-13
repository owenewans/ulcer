package control

import (
	"sync"

	controlv1 "github.com/owenewans/ulcer/gen/control/v1"
)

type Hub struct {
	mu          sync.Mutex
	connections map[string]*connection
	pending     map[string]map[*Reservation]struct{}
	blocked     map[string]int
}

type Reservation struct {
	id         string
	connection *connection
	canceled   bool
}

type connection struct {
	messages chan *controlv1.HostMessage
	done     chan struct{}
	closed   bool
}

func NewHub() *Hub {
	return &Hub{
		connections: make(map[string]*connection),
		pending:     make(map[string]map[*Reservation]struct{}),
		blocked:     make(map[string]int),
	}
}

func (h *Hub) Reserve(id string) (*Reservation, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.blocked[id] > 0 {
		return nil, false
	}
	reservation := &Reservation{id: id, connection: &connection{
		messages: make(chan *controlv1.HostMessage, 16),
		done:     make(chan struct{}),
	}}
	if h.pending[id] == nil {
		h.pending[id] = make(map[*Reservation]struct{})
	}
	h.pending[id][reservation] = struct{}{}
	return reservation, true
}

func (h *Hub) Activate(reservation *Reservation) (<-chan *controlv1.HostMessage, <-chan struct{}, func() bool, bool) {
	h.mu.Lock()
	pending, exists := h.pending[reservation.id]
	if exists {
		_, exists = pending[reservation]
		delete(pending, reservation)
		if len(pending) == 0 {
			delete(h.pending, reservation.id)
		}
	}
	if !exists || reservation.canceled || h.blocked[reservation.id] > 0 {
		closeConnection(reservation.connection)
		h.mu.Unlock()
		return nil, nil, func() bool { return false }, false
	}
	if previous, ok := h.connections[reservation.id]; ok {
		closeConnection(previous)
	}
	current := reservation.connection
	h.connections[reservation.id] = current
	h.mu.Unlock()

	return current.messages, current.done, func() bool {
		h.mu.Lock()
		defer h.mu.Unlock()
		if attached, ok := h.connections[reservation.id]; ok && attached == current {
			delete(h.connections, reservation.id)
			closeConnection(current)
			return true
		}
		return false
	}, true
}

func (h *Hub) Cancel(reservation *Reservation) {
	if reservation == nil {
		return
	}
	h.mu.Lock()
	if pending := h.pending[reservation.id]; pending != nil {
		delete(pending, reservation)
		if len(pending) == 0 {
			delete(h.pending, reservation.id)
		}
	}
	reservation.canceled = true
	closeConnection(reservation.connection)
	h.mu.Unlock()
}

func (h *Hub) Revoke(id string) func() {
	h.mu.Lock()
	h.blocked[id]++
	for reservation := range h.pending[id] {
		reservation.canceled = true
		closeConnection(reservation.connection)
	}
	delete(h.pending, id)
	if current, ok := h.connections[id]; ok {
		delete(h.connections, id)
		closeConnection(current)
	}
	h.mu.Unlock()

	var once sync.Once
	return func() {
		once.Do(func() {
			h.mu.Lock()
			h.blocked[id]--
			if h.blocked[id] == 0 {
				delete(h.blocked, id)
			}
			h.mu.Unlock()
		})
	}
}

func (h *Hub) IfNotRevoked(id string, action func()) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.blocked[id] > 0 {
		return false
	}
	action()
	return true
}

func (h *Hub) IfOnline(id string, action func()) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	current, online := h.connections[id]
	if h.blocked[id] > 0 || !online || current.closed {
		return false
	}
	action()
	return true
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
		closeConnection(current)
		return false
	}
}

func (h *Hub) Online(id string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	current, ok := h.connections[id]
	return ok && !current.closed
}

func closeConnection(connection *connection) {
	if !connection.closed {
		close(connection.done)
		close(connection.messages)
		connection.closed = true
	}
}
