package transport

import (
	"net/http"

	runtimeconfig "ollama-gateway/internal/function/runtime_config"
	"ollama-gateway/pkg/httputil"
)

type Handler struct {
	svc *runtimeconfig.Service
}

func NewHandler(svc *runtimeconfig.Service) *Handler {
	return &Handler{svc: svc}
}

func (h *Handler) Reload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httputil.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if h == nil || h.svc == nil {
		httputil.WriteError(w, http.StatusInternalServerError, "runtime config service no disponible")
		return
	}

	result, err := h.svc.Reload()
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"message": "configuracion recargada sin reinicio",
		"result":  result,
	})
}
