package transport

import (
	"encoding/json"
	"net/http"
	"strings"

	sqlreviewservice "ollama-gateway/internal/function/sqlreview"
	"ollama-gateway/pkg/httputil"
)

type SQLReviewHandler struct {
	svc *sqlreviewservice.Service
}

type reviewRequest struct {
	SQL     string `json:"sql"`
	Dialect string `json:"dialect"`
}

func NewSQLReviewHandler(svc *sqlreviewservice.Service) *SQLReviewHandler {
	return &SQLReviewHandler{svc: svc}
}

func (h *SQLReviewHandler) Review(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.svc == nil {
		httputil.WriteError(w, http.StatusInternalServerError, "sql review service no disponible")
		return
	}

	var req reviewRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "body invalido: "+err.Error())
		return
	}
	req.SQL = strings.TrimSpace(req.SQL)
	req.Dialect = strings.TrimSpace(req.Dialect)
	if req.SQL == "" {
		httputil.WriteError(w, http.StatusBadRequest, "sql es requerido")
		return
	}
	if req.Dialect == "" {
		httputil.WriteError(w, http.StatusBadRequest, "dialect es requerido")
		return
	}

	result, err := h.svc.ReviewMigration(req.SQL, req.Dialect)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	httputil.WriteJSON(w, http.StatusOK, result)
}
