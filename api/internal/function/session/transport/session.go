package transport

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"ollama-gateway/internal/function/core/domain"
	"ollama-gateway/internal/middleware"
	"ollama-gateway/pkg/httputil"
)

type SessionHandler struct {
	sessions domain.SessionService
	rag      domain.RAGEngine
}

type sessionChatRequest struct {
	Message string `json:"message"`
}

func NewSessionHandler(sessions domain.SessionService, rag domain.RAGEngine) *SessionHandler {
	return &SessionHandler{sessions: sessions, rag: rag}
}

func (h *SessionHandler) Create(w http.ResponseWriter, r *http.Request) {
	userID := strings.TrimSpace(middleware.UserIDFromContext(r.Context()))
	if userID == "" {
		httputil.WriteError(w, http.StatusUnauthorized, "usuario no autenticado")
		return
	}
	session, err := h.sessions.Create(userID)
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	httputil.WriteJSON(w, http.StatusCreated, map[string]string{"session_id": session.ID})
}

func (h *SessionHandler) Join(w http.ResponseWriter, r *http.Request) {
	sessionID := strings.TrimSpace(r.PathValue("id"))
	userID := strings.TrimSpace(middleware.UserIDFromContext(r.Context()))
	if sessionID == "" {
		httputil.WriteError(w, http.StatusBadRequest, "session_id requerido")
		return
	}
	if userID == "" {
		httputil.WriteError(w, http.StatusUnauthorized, "usuario no autenticado")
		return
	}
	if err := h.sessions.Join(sessionID, userID); err != nil {
		httputil.WriteError(w, http.StatusNotFound, err.Error())
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]string{"status": "joined", "session_id": sessionID})
}

func (h *SessionHandler) GetMessages(w http.ResponseWriter, r *http.Request) {
	sessionID := strings.TrimSpace(r.PathValue("id"))
	if sessionID == "" {
		httputil.WriteError(w, http.StatusBadRequest, "session_id requerido")
		return
	}

	since := parseSince(r.URL.Query().Get("since"))
	messages, err := h.sessions.GetMessages(sessionID, since)
	if err != nil {
		httputil.WriteError(w, http.StatusNotFound, err.Error())
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"session_id": sessionID,
		"messages":   messages,
		"count":      len(messages),
	})
}

func (h *SessionHandler) Chat(w http.ResponseWriter, r *http.Request) {
	sessionID := strings.TrimSpace(r.PathValue("id"))
	userID := strings.TrimSpace(middleware.UserIDFromContext(r.Context()))
	if sessionID == "" {
		httputil.WriteError(w, http.StatusBadRequest, "session_id requerido")
		return
	}
	if userID == "" {
		httputil.WriteError(w, http.StatusUnauthorized, "usuario no autenticado")
		return
	}

	var req sessionChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "body inválido: "+err.Error())
		return
	}
	if strings.TrimSpace(req.Message) == "" {
		httputil.WriteError(w, http.StatusBadRequest, "message requerido")
		return
	}

	if err := h.sessions.Join(sessionID, userID); err != nil {
		httputil.WriteError(w, http.StatusNotFound, err.Error())
		return
	}
	if err := h.sessions.AddMessage(sessionID, domain.Message{Role: "user", Content: req.Message}); err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	prompt := "session_id=" + sessionID + "\nuser=" + userID + "\nuser: " + req.Message
	result, err := h.rag.GenerateWithContext(prompt)
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	assistantMsg := domain.Message{Role: "assistant", Content: result}
	if err := h.sessions.AddMessage(sessionID, assistantMsg); err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"session_id": sessionID,
		"response":   result,
	})
}

func parseSince(raw string) time.Time {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}
	}
	if t, err := time.Parse(time.RFC3339, raw); err == nil {
		return t
	}
	if t, err := time.Parse(time.RFC3339Nano, raw); err == nil {
		return t
	}
	if n, err := strconv.ParseInt(raw, 10, 64); err == nil {
		return time.Unix(n, 0).UTC()
	}
	return time.Time{}
}
