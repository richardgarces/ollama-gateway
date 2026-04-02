package handlers

import (
	"encoding/json"
	"io"
	"net/http"

	"ollama-gateway/internal/services"
	"ollama-gateway/pkg/httputil"
)

type SearchHandler struct {
	ollama *services.OllamaService
	qdrant *services.QdrantService
}

func NewSearchHandler(o *services.OllamaService, q *services.QdrantService) *SearchHandler {
	return &SearchHandler{ollama: o, qdrant: q}
}

type searchReq struct {
	Query string `json:"query"`
	Top   int    `json:"top"`
}

func (h *SearchHandler) Handle(w http.ResponseWriter, r *http.Request) {
	var req searchReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if req.Top <= 0 {
		req.Top = 5
	}
	emb, err := h.ollama.GetEmbedding("default", req.Query)
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	res, err := h.qdrant.Search("repo_docs", emb, req.Top)
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	httputil.WriteJSON(w, http.StatusOK, res)
}

// exported for tests
type ioReader interface{ io.Reader }
