package chat

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"riders-connect/internal/middleware"
	"riders-connect/internal/respond"
)

type Handler struct {
	svc *Service
	hub *Hub
}

func NewHandler(svc *Service, hub *Hub) *Handler {
	return &Handler{svc: svc, hub: hub}
}

func (h *Handler) ListChats(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.GetUserID(r.Context())
	chats, err := h.svc.ListChats(r.Context(), userID)
	if err != nil {
		respond.Error(w, http.StatusInternalServerError, "internal error")
		return
	}
	if chats == nil {
		chats = []*ChatListItem{}
	}
	respond.JSON(w, http.StatusOK, chats)
}

type createChatRequest struct {
	// Direct chat
	UserID   string `json:"user_id"`
	Username string `json:"username"`
	// Group chat
	Title     string   `json:"title"`
	MemberIDs []string `json:"member_ids"`
	Usernames []string `json:"usernames"`
}

func (h *Handler) CreateChat(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.GetUserID(r.Context())
	var req createChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respond.Error(w, http.StatusBadRequest, "invalid request")
		return
	}

	isGroup := strings.TrimSpace(req.Title) != "" || len(req.MemberIDs) > 0 || len(req.Usernames) > 0
	var chat *Chat
	var err error

	if isGroup {
		memberIDs, rerr := h.resolveMembers(r, req.MemberIDs, req.Usernames)
		if rerr != nil {
			h.writeResolveError(w, rerr)
			return
		}
		chat, err = h.svc.CreateGroup(r.Context(), userID, req.Title, memberIDs)
		if err == ErrInvalidGroup {
			respond.Error(w, http.StatusBadRequest, "group requires a title and at least one other member")
			return
		}
	} else {
		targetID, terr := h.resolveOne(r, req.UserID, req.Username)
		if terr != nil {
			h.writeResolveError(w, terr)
			return
		}
		chat, err = h.svc.CreateDirect(r.Context(), userID, targetID)
	}
	if err != nil {
		respond.Error(w, http.StatusInternalServerError, "internal error")
		return
	}

	item, err := h.svc.GetChatItem(r.Context(), chat.ID, userID)
	if err != nil {
		respond.Error(w, http.StatusInternalServerError, "internal error")
		return
	}
	respond.JSON(w, http.StatusOK, item)
}

func (h *Handler) AddMember(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.GetUserID(r.Context())
	chatID, err := uuid.Parse(chi.URLParam(r, "chatID"))
	if err != nil {
		respond.Error(w, http.StatusBadRequest, "invalid chat id")
		return
	}
	var req struct {
		UserID   string `json:"user_id"`
		Username string `json:"username"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respond.Error(w, http.StatusBadRequest, "invalid request")
		return
	}
	targetID, terr := h.resolveOne(r, req.UserID, req.Username)
	if terr != nil {
		h.writeResolveError(w, terr)
		return
	}
	if err := h.svc.AddMember(r.Context(), chatID, userID, targetID); err != nil {
		if err == ErrForbidden {
			respond.Error(w, http.StatusForbidden, "forbidden")
			return
		}
		respond.Error(w, http.StatusInternalServerError, "internal error")
		return
	}
	item, err := h.svc.GetChatItem(r.Context(), chatID, userID)
	if err != nil {
		respond.Error(w, http.StatusInternalServerError, "internal error")
		return
	}
	respond.JSON(w, http.StatusOK, item)
}

// UpdateChat patches a group chat's title and/or avatar, then broadcasts the
// refreshed chat to every member.
func (h *Handler) UpdateChat(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.GetUserID(r.Context())
	chatID, err := uuid.Parse(chi.URLParam(r, "chatID"))
	if err != nil {
		respond.Error(w, http.StatusBadRequest, "invalid chat id")
		return
	}
	var req struct {
		Title     *string `json:"title"`
		AvatarURL *string `json:"avatar_url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respond.Error(w, http.StatusBadRequest, "invalid request")
		return
	}
	if req.Title != nil {
		trimmed := strings.TrimSpace(*req.Title)
		if trimmed == "" {
			respond.Error(w, http.StatusBadRequest, "title cannot be empty")
			return
		}
		req.Title = &trimmed
	}
	if err := h.svc.UpdateChat(r.Context(), chatID, userID, req.Title, req.AvatarURL); err != nil {
		h.writeChatErr(w, err)
		return
	}
	item, err := h.svc.GetChatItem(r.Context(), chatID, userID)
	if err != nil {
		respond.Error(w, http.StatusInternalServerError, "internal error")
		return
	}
	if memberIDs, err := h.svc.ListMemberIDs(r.Context(), chatID); err == nil {
		broadcastChatUpdated(h.hub, memberIDs, item)
	}
	respond.JSON(w, http.StatusOK, item)
}

// RemoveMember drops a participant (or the caller themselves) from a group chat.
// The removed member is told to drop the chat; remaining members get a refresh.
func (h *Handler) RemoveMember(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.GetUserID(r.Context())
	chatID, err := uuid.Parse(chi.URLParam(r, "chatID"))
	if err != nil {
		respond.Error(w, http.StatusBadRequest, "invalid chat id")
		return
	}
	targetID, err := uuid.Parse(chi.URLParam(r, "userID"))
	if err != nil {
		respond.Error(w, http.StatusBadRequest, "invalid user id")
		return
	}
	if err := h.svc.RemoveMember(r.Context(), chatID, userID, targetID); err != nil {
		h.writeChatErr(w, err)
		return
	}
	broadcastChatRemoved(h.hub, targetID, chatID)
	if memberIDs, err := h.svc.ListMemberIDs(r.Context(), chatID); err == nil && len(memberIDs) > 0 {
		if item, err := h.svc.GetChatItem(r.Context(), chatID, memberIDs[0]); err == nil {
			broadcastChatUpdated(h.hub, memberIDs, item)
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

// writeChatErr maps chat-mutation service errors to HTTP responses.
func (h *Handler) writeChatErr(w http.ResponseWriter, err error) {
	switch err {
	case ErrForbidden:
		respond.Error(w, http.StatusForbidden, "forbidden")
	case ErrNotFound:
		respond.Error(w, http.StatusNotFound, "not found")
	default:
		respond.Error(w, http.StatusInternalServerError, "internal error")
	}
}

func (h *Handler) GetMessages(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.GetUserID(r.Context())
	chatID, err := uuid.Parse(chi.URLParam(r, "chatID"))
	if err != nil {
		respond.Error(w, http.StatusBadRequest, "invalid chat id")
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	msgs, err := h.svc.GetMessages(r.Context(), chatID, userID, limit, offset)
	if err == ErrForbidden {
		respond.Error(w, http.StatusForbidden, "forbidden")
		return
	}
	if err == ErrNotFound {
		respond.Error(w, http.StatusNotFound, "not found")
		return
	}
	if err != nil {
		respond.Error(w, http.StatusInternalServerError, "internal error")
		return
	}
	if msgs == nil {
		msgs = []*Message{}
	}
	respond.JSON(w, http.StatusOK, msgs)
}

func (h *Handler) SendMessage(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.GetUserID(r.Context())
	chatID, err := uuid.Parse(chi.URLParam(r, "chatID"))
	if err != nil {
		respond.Error(w, http.StatusBadRequest, "invalid chat id")
		return
	}
	var req struct {
		Content        string          `json:"content"`
		MessageType    string          `json:"message_type"`
		AttachmentURL  string          `json:"attachment_url"`
		AttachmentMeta json.RawMessage `json:"attachment_meta"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respond.Error(w, http.StatusBadRequest, "invalid request")
		return
	}
	if req.Content == "" && req.AttachmentURL == "" {
		respond.Error(w, http.StatusBadRequest, "content or attachment_url required")
		return
	}
	msg, err := h.svc.SendMessage(r.Context(), chatID, userID, SendMessageParams{
		Content:        req.Content,
		MessageType:    req.MessageType,
		AttachmentURL:  req.AttachmentURL,
		AttachmentMeta: req.AttachmentMeta,
	})
	if err == ErrInvalidMessageType {
		respond.Error(w, http.StatusBadRequest, "invalid message_type")
		return
	}
	if err == ErrForbidden {
		respond.Error(w, http.StatusForbidden, "forbidden")
		return
	}
	if err == ErrNotFound {
		respond.Error(w, http.StatusNotFound, "not found")
		return
	}
	if err != nil {
		respond.Error(w, http.StatusInternalServerError, "internal error")
		return
	}
	respond.JSON(w, http.StatusCreated, msg)
}

func (h *Handler) EditMessage(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.GetUserID(r.Context())
	chatID, err := uuid.Parse(chi.URLParam(r, "chatID"))
	if err != nil {
		respond.Error(w, http.StatusBadRequest, "invalid chat id")
		return
	}
	messageID, err := uuid.Parse(chi.URLParam(r, "messageID"))
	if err != nil {
		respond.Error(w, http.StatusBadRequest, "invalid message id")
		return
	}
	var req struct {
		Content string `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respond.Error(w, http.StatusBadRequest, "invalid request")
		return
	}
	msg, err := h.svc.EditMessage(r.Context(), chatID, messageID, userID, req.Content)
	if err != nil {
		h.writeMessageErr(w, err)
		return
	}
	if memberIDs, err := h.svc.ListMemberIDs(r.Context(), chatID); err == nil {
		broadcastMessageEdited(h.hub, memberIDs, msg)
	}
	respond.JSON(w, http.StatusOK, msg)
}

func (h *Handler) DeleteMessage(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.GetUserID(r.Context())
	chatID, err := uuid.Parse(chi.URLParam(r, "chatID"))
	if err != nil {
		respond.Error(w, http.StatusBadRequest, "invalid chat id")
		return
	}
	messageID, err := uuid.Parse(chi.URLParam(r, "messageID"))
	if err != nil {
		respond.Error(w, http.StatusBadRequest, "invalid message id")
		return
	}
	scope := r.URL.Query().Get("scope")
	if scope == "" {
		scope = DeleteScopeMe
	}
	if err := h.svc.DeleteMessage(r.Context(), chatID, messageID, userID, scope); err != nil {
		h.writeMessageErr(w, err)
		return
	}
	// Only "delete for all" is visible to other members.
	if scope == DeleteScopeAll {
		if memberIDs, err := h.svc.ListMemberIDs(r.Context(), chatID); err == nil {
			broadcastMessageDeleted(h.hub, memberIDs, chatID, messageID)
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

// MarkRead advances the caller's read cursor for a chat and broadcasts the
// resulting receipt. WebSocket is the primary path; this is the REST fallback.
func (h *Handler) MarkRead(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.GetUserID(r.Context())
	chatID, err := uuid.Parse(chi.URLParam(r, "chatID"))
	if err != nil {
		respond.Error(w, http.StatusBadRequest, "invalid chat id")
		return
	}
	lastReadAt, err := h.svc.MarkRead(r.Context(), chatID, userID)
	if err != nil {
		h.writeMessageErr(w, err)
		return
	}
	if lastReadAt != nil {
		if memberIDs, err := h.svc.ListMemberIDs(r.Context(), chatID); err == nil {
			broadcastReadReceipt(h.hub, memberIDs, chatID, userID, *lastReadAt)
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

// writeMessageErr maps edit/delete service errors to HTTP responses.
func (h *Handler) writeMessageErr(w http.ResponseWriter, err error) {
	switch err {
	case ErrInvalidScope, ErrEmptyContent:
		respond.Error(w, http.StatusBadRequest, err.Error())
	case ErrForbidden:
		respond.Error(w, http.StatusForbidden, "forbidden")
	case ErrNotFound, ErrMessageNotFound:
		respond.Error(w, http.StatusNotFound, "not found")
	default:
		respond.Error(w, http.StatusInternalServerError, "internal error")
	}
}

// resolveOne resolves a single participant from either a user_id or a username.
func (h *Handler) resolveOne(r *http.Request, userIDStr, username string) (uuid.UUID, error) {
	if strings.TrimSpace(username) != "" {
		return h.svc.ResolveUsername(r.Context(), username)
	}
	return uuid.Parse(userIDStr)
}

func (h *Handler) resolveMembers(r *http.Request, ids, usernames []string) ([]uuid.UUID, error) {
	out := make([]uuid.UUID, 0, len(ids)+len(usernames))
	for _, idStr := range ids {
		id, err := uuid.Parse(idStr)
		if err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	for _, uname := range usernames {
		id, err := h.svc.ResolveUsername(r.Context(), uname)
		if err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, nil
}

func (h *Handler) writeResolveError(w http.ResponseWriter, err error) {
	if err == ErrUserNotFound {
		respond.Error(w, http.StatusNotFound, "user not found")
		return
	}
	respond.Error(w, http.StatusBadRequest, "invalid user reference")
}
