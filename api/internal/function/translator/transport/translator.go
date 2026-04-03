package transport

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strings"

	"ollama-gateway/internal/function/core/domain"
	"ollama-gateway/pkg/httputil"
)

type TranslatorHandler struct {
	svc domain.TranslatorService
}

type translateRequest struct {
	Code string `json:"code"`
	From string `json:"from"`
	To   string `json:"to"`
}

type translateFileRequest struct {
	Path string `json:"path"`
	To   string `json:"to"`
}

func NewTranslatorHandler(svc domain.TranslatorService) *TranslatorHandler {
	return &TranslatorHandler{svc: svc}
}

func (h *TranslatorHandler) Translate(w http.ResponseWriter, r *http.Request) {
	var req translateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "body inválido: "+err.Error())
		return
	}
	if strings.TrimSpace(req.Code) == "" {
		httputil.WriteError(w, http.StatusBadRequest, "code requerido")
		return
	}
	if strings.TrimSpace(req.To) == "" {
		httputil.WriteError(w, http.StatusBadRequest, "to requerido")
		return
	}

	translated, err := h.svc.Translate(req.Code, req.From, req.To)
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]string{"translated_code": translated})
}

func (h *TranslatorHandler) TranslateFile(w http.ResponseWriter, r *http.Request) {
	var req translateFileRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "body inválido: "+err.Error())
		return
	}
	if strings.TrimSpace(req.Path) == "" {
		httputil.WriteError(w, http.StatusBadRequest, "path requerido")
		return
	}
	if strings.TrimSpace(req.To) == "" {
		httputil.WriteError(w, http.StatusBadRequest, "to requerido")
		return
	}

	translated, err := h.svc.TranslateFile(req.Path, req.To)
	if err != nil {
		msg := err.Error()
		switch {
		case strings.Contains(strings.ToLower(msg), "path") || strings.Contains(msg, "REPO_ROOT"):
			httputil.WriteError(w, http.StatusBadRequest, msg)
		case errors.Is(err, os.ErrNotExist):
			httputil.WriteError(w, http.StatusNotFound, "archivo no encontrado")
		default:
			httputil.WriteError(w, http.StatusInternalServerError, msg)
		}
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]string{"translated_code": translated})
}
