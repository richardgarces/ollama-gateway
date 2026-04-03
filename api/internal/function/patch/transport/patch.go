package transport

import (
	"encoding/json"
	"net/http"
	"strconv"

	"ollama-gateway/internal/function/core/domain"
	"ollama-gateway/pkg/httputil"
)

type PatchHandler struct {
	repoRoot     string
	patchService domain.PatchService
}

func NewPatchHandler(repoRoot string, patchService domain.PatchService) *PatchHandler {
	return &PatchHandler{repoRoot: repoRoot, patchService: patchService}
}

func (h *PatchHandler) Apply(w http.ResponseWriter, r *http.Request) {
	var req domain.PatchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "body invalido: "+err.Error())
		return
	}
	if req.Response == "" {
		httputil.WriteError(w, http.StatusBadRequest, "response requerido")
		return
	}

	blocks := h.patchService.ExtractCodeBlocks(req.Response)
	diffs := h.patchService.ExtractDiff(req.Response)

	applied := 0
	if req.Apply {
		for _, d := range diffs {
			if err := h.patchService.ApplyPatch(h.repoRoot, d); err != nil {
				httputil.WriteError(w, http.StatusBadRequest, "fallo aplicando patch: "+err.Error())
				return
			}
			applied++
		}
	}

	httputil.WriteJSON(w, http.StatusOK, domain.PatchResponse{
		CodeBlocks: blocks,
		Diffs:      diffs,
		Applied:    req.Apply,
		AppliedNum: applied,
	})
}

func (h *PatchHandler) Preview(w http.ResponseWriter, r *http.Request) {
	response := r.URL.Query().Get("response")
	if response == "" {
		httputil.WriteError(w, http.StatusBadRequest, "query param response requerido")
		return
	}

	limit := 200
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 && parsed <= 1000 {
			limit = parsed
		}
	}

	diffs := h.patchService.ExtractDiff(response)
	if len(diffs) > limit {
		diffs = diffs[:limit]
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"diffs": diffs,
		"count": len(diffs),
	})
}
