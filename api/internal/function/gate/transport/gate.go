package transport

import (
	"net/http"
	"strings"

	gateservice "ollama-gateway/internal/function/gate"
	"ollama-gateway/pkg/httputil"
)

type Handler struct {
	svc *gateservice.Service
}

func NewHandler(svc *gateservice.Service) *Handler {
	return &Handler{svc: svc}
}

func (h *Handler) CheckDeployGate(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.svc == nil {
		httputil.WriteError(w, http.StatusInternalServerError, "gate service no disponible")
		return
	}

	profile := strings.TrimSpace(r.URL.Query().Get("profile"))
	environment := strings.TrimSpace(r.URL.Query().Get("env"))

	var (
		result gateservice.GateResult
		err    error
	)
	if profile == "" && environment == "" {
		result, err = h.svc.CheckDeployGate()
	} else {
		result, err = h.svc.CheckDeployGateWith(profile, environment)
	}
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	httputil.WriteJSON(w, http.StatusOK, result)
}
