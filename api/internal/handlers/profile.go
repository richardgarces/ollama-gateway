package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"ollama-gateway/internal/domain"
	"ollama-gateway/internal/middleware"
	"ollama-gateway/pkg/httputil"
)

type ProfileHandler struct {
	profiles profileService
}

type profileService interface {
	GetByUserID(ctx context.Context, userID string) (*domain.Profile, error)
	Upsert(ctx context.Context, profile domain.Profile) (*domain.Profile, error)
}

func NewProfileHandler(profiles profileService) *ProfileHandler {
	return &ProfileHandler{profiles: profiles}
}

type profileUpdateRequest struct {
	PreferredModel string   `json:"preferred_model"`
	Temperature    *float64 `json:"temperature,omitempty"`
	SystemPrompt   string   `json:"system_prompt"`
	MaxTokens      *int     `json:"max_tokens,omitempty"`
}

func (h *ProfileHandler) Get(w http.ResponseWriter, r *http.Request) {
	userID := strings.TrimSpace(middleware.UserIDFromContext(r.Context()))
	if userID == "" {
		httputil.WriteError(w, http.StatusUnauthorized, "user_id no presente en token")
		return
	}
	if h.profiles == nil {
		httputil.WriteError(w, http.StatusServiceUnavailable, "profile service no disponible")
		return
	}

	profile, err := h.profiles.GetByUserID(r.Context(), userID)
	if err != nil {
		httputil.WriteError(w, http.StatusNotFound, "perfil no encontrado")
		return
	}

	if profile == nil {
		httputil.WriteError(w, http.StatusNotFound, "perfil no encontrado")
		return
	}

	httputil.WriteJSON(w, http.StatusOK, profile)
}

func (h *ProfileHandler) Put(w http.ResponseWriter, r *http.Request) {
	userID := strings.TrimSpace(middleware.UserIDFromContext(r.Context()))
	if userID == "" {
		httputil.WriteError(w, http.StatusUnauthorized, "user_id no presente en token")
		return
	}
	if h.profiles == nil {
		httputil.WriteError(w, http.StatusServiceUnavailable, "profile service no disponible")
		return
	}

	var req profileUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "body invalido: "+err.Error())
		return
	}

	profile := domain.Profile{
		UserID:         userID,
		PreferredModel: strings.TrimSpace(req.PreferredModel),
		SystemPrompt:   strings.TrimSpace(req.SystemPrompt),
	}
	if req.Temperature != nil {
		profile.Temperature = *req.Temperature
	}
	if req.MaxTokens != nil {
		profile.MaxTokens = *req.MaxTokens
	}

	updated, err := h.profiles.Upsert(r.Context(), profile)
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	httputil.WriteJSON(w, http.StatusOK, updated)
}
