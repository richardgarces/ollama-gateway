package handlers

import (
	"encoding/json"
	"net/http"
	"strings"

	"ollama-gateway/pkg/httputil"
)

type sqlGenService interface {
	GenerateQuery(description, dialect string) (string, error)
	GenerateMigration(description, dialect string) (string, error)
	ExplainQuery(sql string) (string, error)
}

type SQLGenHandler struct {
	svc sqlGenService
}

type sqlQueryRequest struct {
	Description string `json:"description"`
	Dialect     string `json:"dialect"`
}

type sqlMigrationRequest struct {
	Description string `json:"description"`
	Dialect     string `json:"dialect"`
}

type sqlExplainRequest struct {
	SQL string `json:"sql"`
}

func NewSQLGenHandler(svc sqlGenService) *SQLGenHandler {
	return &SQLGenHandler{svc: svc}
}

func (h *SQLGenHandler) GenerateQuery(w http.ResponseWriter, r *http.Request) {
	var req sqlQueryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "body inválido: "+err.Error())
		return
	}
	if strings.TrimSpace(req.Description) == "" {
		httputil.WriteError(w, http.StatusBadRequest, "description requerido")
		return
	}
	if strings.TrimSpace(req.Dialect) == "" {
		httputil.WriteError(w, http.StatusBadRequest, "dialect requerido")
		return
	}

	sqlText, err := h.svc.GenerateQuery(req.Description, req.Dialect)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "dialect") || strings.Contains(strings.ToLower(err.Error()), "requerido") {
			httputil.WriteError(w, http.StatusBadRequest, err.Error())
			return
		}
		httputil.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]string{
		"dialect": req.Dialect,
		"sql":     sqlText,
		"warning": "SQL generado por IA. Revísalo manualmente antes de ejecutarlo.",
	})
}

func (h *SQLGenHandler) GenerateMigration(w http.ResponseWriter, r *http.Request) {
	var req sqlMigrationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "body inválido: "+err.Error())
		return
	}
	if strings.TrimSpace(req.Description) == "" {
		httputil.WriteError(w, http.StatusBadRequest, "description requerido")
		return
	}
	if strings.TrimSpace(req.Dialect) == "" {
		httputil.WriteError(w, http.StatusBadRequest, "dialect requerido")
		return
	}

	migrationSQL, err := h.svc.GenerateMigration(req.Description, req.Dialect)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "dialect") || strings.Contains(strings.ToLower(err.Error()), "requerido") {
			httputil.WriteError(w, http.StatusBadRequest, err.Error())
			return
		}
		httputil.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]string{
		"dialect":   req.Dialect,
		"migration": migrationSQL,
		"warning":   "SQL generado por IA. Revísalo manualmente antes de ejecutarlo.",
	})
}

func (h *SQLGenHandler) ExplainQuery(w http.ResponseWriter, r *http.Request) {
	var req sqlExplainRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "body inválido: "+err.Error())
		return
	}
	if strings.TrimSpace(req.SQL) == "" {
		httputil.WriteError(w, http.StatusBadRequest, "sql requerido")
		return
	}

	explanation, err := h.svc.ExplainQuery(req.SQL)
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]string{
		"explanation": explanation,
		"warning":     "Análisis generado por IA. Verifica recomendaciones antes de aplicar cambios.",
	})
}
