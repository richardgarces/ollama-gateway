package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strings"

	"ollama-gateway/internal/domain"
	"ollama-gateway/internal/services"
	"ollama-gateway/pkg/httputil"
)

type ReviewHandler struct {
	reviewService *services.ReviewService
}

type reviewDiffRequest struct {
	Diff string `json:"diff"`
}

type reviewFileRequest struct {
	Path string `json:"path"`
}

func NewReviewHandler(reviewService *services.ReviewService) *ReviewHandler {
	return &ReviewHandler{reviewService: reviewService}
}

func (h *ReviewHandler) ReviewDiff(w http.ResponseWriter, r *http.Request) {
	var req reviewDiffRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "body inválido: "+err.Error())
		return
	}
	if req.Diff == "" {
		httputil.WriteError(w, http.StatusBadRequest, "diff requerido")
		return
	}

	comments, err := h.reviewService.ReviewDiff(req.Diff)
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	httputil.WriteJSON(w, http.StatusOK, domain.ReviewResult{
		Comments: comments,
		Summary:  services.BuildReviewSummary(comments),
	})
}

func (h *ReviewHandler) ReviewFile(w http.ResponseWriter, r *http.Request) {
	var req reviewFileRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "body inválido: "+err.Error())
		return
	}
	if req.Path == "" {
		httputil.WriteError(w, http.StatusBadRequest, "path requerido")
		return
	}

	comments, err := h.reviewService.ReviewFile(req.Path)
	if err != nil {
		msg := err.Error()
		switch {
		case strings.Contains(msg, "path") || strings.Contains(msg, "REPO_ROOT"):
			httputil.WriteError(w, http.StatusBadRequest, msg)
		case errors.Is(err, os.ErrNotExist):
			httputil.WriteError(w, http.StatusNotFound, "archivo no encontrado")
		default:
			httputil.WriteError(w, http.StatusInternalServerError, msg)
		}
		return
	}

	httputil.WriteJSON(w, http.StatusOK, domain.ReviewResult{
		Comments: comments,
		Summary:  services.BuildReviewSummary(comments),
	})
}
