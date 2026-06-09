package chat

import (
	"sync"

	"github.com/google/uuid"
)

type Client struct {
	UserID uuid.UUID
	Send   chan []byte
}

type Hub struct {
	mu         sync.RWMutex
	clients    map[uuid.UUID]*Client
	register   chan *Client
	unregister chan *Client
}

func NewHub() *Hub {
	return &Hub{
		clients:    make(map[uuid.UUID]*Client),
		register:   make(chan *Client),
		unregister: make(chan *Client),
	}
}

func (h *Hub) Run() {
	for {
		select {
		case c := <-h.register:
			h.mu.Lock()
			h.clients[c.UserID] = c
			h.mu.Unlock()
		case c := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[c.UserID]; ok {
				delete(h.clients, c.UserID)
				close(c.Send)
			}
			h.mu.Unlock()
		}
	}
}

func (h *Hub) SendToUser(userID uuid.UUID, data []byte) {
	h.mu.RLock()
	c, ok := h.clients[userID]
	h.mu.RUnlock()
	if !ok {
		return
	}
	select {
	case c.Send <- data:
	default:
	}
}

// SendToUsers delivers data to each of the given users (best-effort).
func (h *Hub) SendToUsers(userIDs []uuid.UUID, data []byte) {
	for _, id := range userIDs {
		h.SendToUser(id, data)
	}
}

func (h *Hub) BroadcastAll(data []byte) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, c := range h.clients {
		select {
		case c.Send <- data:
		default:
		}
	}
}

func (h *Hub) ConnectedUserIDs() []uuid.UUID {
	h.mu.RLock()
	defer h.mu.RUnlock()
	ids := make([]uuid.UUID, 0, len(h.clients))
	for id := range h.clients {
		ids = append(ids, id)
	}
	return ids
}
