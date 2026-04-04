package transport

import (
	"encoding/json"
	"net/http"
	"strings"

	contextservice "ollama-gateway/internal/function/context"
	"ollama-gateway/pkg/httputil"
)

type Handler struct {
	svc *contextservice.Service
}

type resolveRequest struct {
	FilePath string `json:"file_path"`
	Prompt   string `json:"prompt"`
	TopK     int    `json:"top_k"`
	MaxDepth int    `json:"max_depth"`
}

func NewHandler(svc *contextservice.Service) *Handler {
	return &Handler{svc: svc}
}

func (h *Handler) Resolve(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.svc == nil {
		httputil.WriteError(w, http.StatusInternalServerError, "context service no disponible")
		return
	}

	var req resolveRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "body inválido: "+err.Error())
		return
	}
	if strings.TrimSpace(req.FilePath) == "" && strings.TrimSpace(req.Prompt) == "" {
		httputil.WriteError(w, http.StatusBadRequest, "file_path o prompt requerido")
		return
	}

	files, err := h.svc.ResolveContextFiles(contextservice.ResolveInput{
		FilePath: req.FilePath,
		Prompt:   req.Prompt,
		TopK:     req.TopK,
		MaxDepth: req.MaxDepth,
	})
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"file_path": req.FilePath,
		"prompt":    req.Prompt,
		"top_k":     req.TopK,
		"max_depth": req.MaxDepth,
		"files":     files,
	})
}
