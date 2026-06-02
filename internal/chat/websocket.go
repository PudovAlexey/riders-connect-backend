package chat

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"riders-connect/internal/middleware"
	"riders-connect/internal/models"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin:     func(r *http.Request) bool { return true },
}

const (
	writeWait  = 10 * time.Second
	pongWait   = 60 * time.Second
	pingPeriod = 54 * time.Second
	maxMsgSize = 8192
)

type geoService interface {
	UpdateLocation(ctx context.Context, userID uuid.UUID, lat, lon float64) error
}

type profileService interface {
	GetUser(ctx context.Context, id uuid.UUID) (*models.User, error)
}

type wsIncoming struct {
	Type           string          `json:"type"`
	ChatID         string          `json:"chat_id"`
	Content        string          `json:"content"`
	MessageType    string          `json:"message_type"`
	AttachmentURL  string          `json:"attachment_url"`
	AttachmentMeta json.RawMessage `json:"attachment_meta"`
	Lat            float64         `json:"lat"`
	Lon            float64         `json:"lon"`
}

type wsOutgoing struct {
	Type    string   `json:"type"`
	Message *Message `json:"message,omitempty"`
}

type wsLocationUpdate struct {
	Type      string    `json:"type"`
	UserID    uuid.UUID `json:"user_id"`
	Lat       float64   `json:"lat"`
	Lon       float64   `json:"lon"`
	Name      string    `json:"name"`
	AvatarURL string    `json:"avatar_url"`
}

type wsPresence struct {
	Type   string    `json:"type"`
	UserID uuid.UUID `json:"user_id"`
}

type wsOnlineUsers struct {
	Type    string      `json:"type"`
	UserIDs []uuid.UUID `json:"user_ids"`
}

type WSHandler struct {
	hub        *Hub
	svc        *Service
	geoSvc     geoService
	profileSvc profileService
}

func NewWSHandler(hub *Hub, svc *Service, geoSvc geoService, profileSvc profileService) *WSHandler {
	return &WSHandler{hub: hub, svc: svc, geoSvc: geoSvc, profileSvc: profileSvc}
}

func (h *WSHandler) Handle(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	client := &Client{UserID: userID, Send: make(chan []byte, 256)}
	h.hub.register <- client

	// Notify all that this user is online
	if data, err := json.Marshal(wsPresence{Type: "user_online", UserID: userID}); err == nil {
		h.hub.BroadcastAll(data)
	}
	// Send current online list to this client
	if data, err := json.Marshal(wsOnlineUsers{Type: "online_users", UserIDs: h.hub.ConnectedUserIDs()}); err == nil {
		select {
		case client.Send <- data:
		default:
		}
	}

	go h.writePump(conn, client)
	h.readPump(conn, client, userID)
}

func (h *WSHandler) readPump(conn *websocket.Conn, client *Client, userID uuid.UUID) {
	ctx := context.Background()
	defer func() {
		h.hub.unregister <- client
		conn.Close()
		// Notify all that this user is offline
		if data, err := json.Marshal(wsPresence{Type: "user_offline", UserID: userID}); err == nil {
			h.hub.BroadcastAll(data)
		}
	}()
	conn.SetReadLimit(maxMsgSize)
	_ = conn.SetReadDeadline(time.Now().Add(pongWait))
	conn.SetPongHandler(func(string) error {
		return conn.SetReadDeadline(time.Now().Add(pongWait))
	})

	for {
		_, raw, err := conn.ReadMessage()
		if err != nil {
			break
		}
		var in wsIncoming
		if err := json.Unmarshal(raw, &in); err != nil {
			continue
		}
		switch in.Type {
		case "send_message":
			if in.Content == "" && in.AttachmentURL == "" {
				continue
			}
			chatID, err := uuid.Parse(in.ChatID)
			if err != nil {
				continue
			}
			msg, err := h.svc.SendMessage(ctx, chatID, userID, SendMessageParams{
				Content:        in.Content,
				MessageType:    in.MessageType,
				AttachmentURL:  in.AttachmentURL,
				AttachmentMeta: in.AttachmentMeta,
			})
			if err != nil {
				log.Printf("ws send_message: %v", err)
				continue
			}
			memberIDs, err := h.svc.ListMemberIDs(ctx, chatID)
			if err != nil {
				continue
			}
			data, _ := json.Marshal(wsOutgoing{Type: "new_message", Message: msg})
			for _, id := range memberIDs {
				h.hub.SendToUser(id, data)
			}

		case "update_location":
			if err := h.geoSvc.UpdateLocation(ctx, userID, in.Lat, in.Lon); err != nil {
				log.Printf("ws update_location: %v", err)
				continue
			}
			update := wsLocationUpdate{Type: "location_update", UserID: userID, Lat: in.Lat, Lon: in.Lon}
			if u, err := h.profileSvc.GetUser(ctx, userID); err == nil && u != nil {
				update.Name = u.Name
				update.AvatarURL = u.AvatarURL
			}
			data, _ := json.Marshal(update)
			h.hub.BroadcastAll(data)
		}
	}
}

func (h *WSHandler) writePump(conn *websocket.Conn, client *Client) {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		conn.Close()
	}()
	for {
		select {
		case msg, ok := <-client.Send:
			_ = conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				_ = conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			if err := conn.WriteMessage(websocket.TextMessage, msg); err != nil {
				return
			}
		case <-ticker.C:
			_ = conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}
