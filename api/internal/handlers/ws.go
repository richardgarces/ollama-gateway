package handlers

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"

	"ollama-gateway/internal/domain"

	"github.com/golang-jwt/jwt/v5"
	"github.com/gorilla/websocket"
)

var errStreamCanceled = errors.New("stream canceled")

type WSHandler struct {
	rag       domain.RAGEngine
	jwtSecret []byte
	upgrader  websocket.Upgrader
}

type wsChatRequest struct {
	Type     string           `json:"type"`
	Model    string           `json:"model"`
	Messages []domain.Message `json:"messages"`
	Stream   bool             `json:"stream"`
}

func NewWSHandler(rag domain.RAGEngine, jwtSecret []byte) *WSHandler {
	return &WSHandler{
		rag:       rag,
		jwtSecret: jwtSecret,
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return true
			},
		},
	}
}

func (h *WSHandler) Handle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	token := strings.TrimSpace(r.URL.Query().Get("token"))
	if token == "" {
		http.Error(w, `{"error":"missing token"}`, http.StatusUnauthorized)
		return
	}
	if _, err := h.validateToken(token); err != nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	conn, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	var writeMu sync.Mutex
	writeJSON := func(payload map[string]interface{}) error {
		writeMu.Lock()
		defer writeMu.Unlock()
		return conn.WriteJSON(payload)
	}

	var stateMu sync.Mutex
	streamActive := false
	var cancelCurrent func()

	for {
		var req wsChatRequest
		if err := conn.ReadJSON(&req); err != nil {
			stateMu.Lock()
			if cancelCurrent != nil {
				cancelCurrent()
			}
			stateMu.Unlock()
			return
		}

		switch strings.ToLower(strings.TrimSpace(req.Type)) {
		case "ping":
			_ = writeJSON(map[string]interface{}{"type": "pong"})
			continue
		case "cancel":
			stateMu.Lock()
			cancel := cancelCurrent
			stateMu.Unlock()
			if cancel != nil {
				cancel()
			}
			_ = writeJSON(map[string]interface{}{"type": "canceled"})
			continue
		}

		if len(req.Messages) == 0 {
			_ = writeJSON(map[string]interface{}{"type": "error", "error": "messages requerido"})
			continue
		}

		prompt := joinMessages(req.Messages)
		prompt = withRequestIDPrompt(r, prompt)

		if !req.Stream {
			stateMu.Lock()
			busy := streamActive
			stateMu.Unlock()
			if busy {
				_ = writeJSON(map[string]interface{}{"type": "error", "error": "stream activo, envía cancel antes de un nuevo prompt"})
				continue
			}
			result, err := h.rag.GenerateWithContext(prompt)
			if err != nil {
				_ = writeJSON(map[string]interface{}{"type": "error", "error": err.Error()})
				continue
			}
			_ = writeJSON(map[string]interface{}{"type": "message", "content": result})
			_ = writeJSON(map[string]interface{}{"type": "done"})
			continue
		}

		stateMu.Lock()
		if streamActive {
			stateMu.Unlock()
			_ = writeJSON(map[string]interface{}{"type": "error", "error": "stream activo, envía cancel antes de un nuevo prompt"})
			continue
		}
		cancelFlag := &atomic.Bool{}
		cancelCurrent = func() {
			cancelFlag.Store(true)
		}
		streamActive = true
		stateMu.Unlock()

		go func(streamPrompt string, canceled *atomic.Bool) {
			defer func() {
				stateMu.Lock()
				streamActive = false
				cancelCurrent = nil
				stateMu.Unlock()
			}()

			err := h.rag.StreamGenerateWithContext(streamPrompt, func(chunk string) error {
				if canceled.Load() {
					return errStreamCanceled
				}
				return writeJSON(map[string]interface{}{"type": "chunk", "content": chunk})
			})

			if err != nil {
				if errors.Is(err, errStreamCanceled) {
					return
				}
				_ = writeJSON(map[string]interface{}{"type": "error", "error": err.Error()})
				return
			}

			if !canceled.Load() {
				_ = writeJSON(map[string]interface{}{"type": "done"})
			}
		}(prompt, cancelFlag)
	}
}

func (h *WSHandler) validateToken(tokenStr string) (string, error) {
	token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return h.jwtSecret, nil
	})
	if err != nil || !token.Valid {
		return "", fmt.Errorf("invalid token")
	}
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return "", fmt.Errorf("invalid token claims")
	}
	for _, key := range []string{"user", "sub", "user_id", "username", "email"} {
		if raw, ok := claims[key].(string); ok {
			if trimmed := strings.TrimSpace(raw); trimmed != "" {
				return trimmed, nil
			}
		}
	}
	return "anonymous", nil
}
