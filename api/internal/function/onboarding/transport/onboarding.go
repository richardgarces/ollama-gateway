package transport

import (
	"encoding/json"
	"net/http"
	"strings"

	onboardingservice "ollama-gateway/internal/function/onboarding"
	"ollama-gateway/pkg/httputil"
)

type Handler struct {
	svc *onboardingservice.Service
}

type generateGuideRequest struct {
	Role  string `json:"role"`
	Apply bool   `json:"apply"`
}

func NewHandler(svc *onboardingservice.Service) *Handler {
	return &Handler{svc: svc}
}

func (h *Handler) GenerateGuide(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.svc == nil {
		httputil.WriteError(w, http.StatusInternalServerError, "onboarding service no disponible")
		return
	}

	var req generateGuideRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "body invalido: "+err.Error())
		return
	}
	req.Role = strings.TrimSpace(req.Role)
	if req.Role == "" {
		httputil.WriteError(w, http.StatusBadRequest, "role es requerido")
		return
	}

	guide, err := h.svc.GenerateGuide(req.Role)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	if req.Apply {
		path, err := h.svc.SaveGuide(guide)
		if err != nil {
			httputil.WriteError(w, http.StatusBadRequest, err.Error())
			return
		}
		guide.Applied = true
		guide.OutputPath = path
	}

	httputil.WriteJSON(w, http.StatusOK, guide)
}
