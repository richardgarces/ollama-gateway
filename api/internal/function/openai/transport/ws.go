package transport

import (
	"encoding/json"
	"net/http"

	"ollama-gateway/internal/function/core/domain"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// ChatWebSocketHandler implementa el endpoint WS para chat streaming tipo OpenAI.
func (h *OpenAIHandler) ChatWebSocketHandler(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			break
		}
		var req struct {
			Model    string           `json:"model"`
			Messages []domain.Message `json:"messages"`
		}
		if err := json.Unmarshal(msg, &req); err != nil {
			_ = conn.WriteMessage(websocket.TextMessage, []byte(`{"error":"invalid request"}`))
			continue
		}
		// Stream chat response
		_ = h.ollama.StreamChat(req.Model, req.Messages, func(chunk string) error {
			resp := map[string]interface{}{
				"choices": []map[string]interface{}{{
					"delta": map[string]string{"content": chunk},
				}},
			}
			data, _ := json.Marshal(resp)
			return conn.WriteMessage(websocket.TextMessage, data)
		})
	}
}
