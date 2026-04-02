package handlers

import (
	"encoding/json"
	"io"
	"net/http"

	"ollama-gateway/internal/domain"
	"ollama-gateway/pkg/httputil"
)

type SearchHandler struct {
	ollama domain.OllamaClient
	qdrant domain.VectorStore
}

func NewSearchHandler(o domain.OllamaClient, q domain.VectorStore) *SearchHandler {
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

	if r.URL.Query().Get("page") != "" || r.URL.Query().Get("page_size") != "" {
		rawItems, ok := res["result"].([]interface{})
		if !ok {
			httputil.WriteJSON(w, http.StatusOK, res)
			return
		}
		page, pageSize := httputil.ParsePagination(r)
		total := len(rawItems)
		start := (page - 1) * pageSize
		if start > total {
			start = total
		}
		end := start + pageSize
		if end > total {
			end = total
		}
		httputil.WritePaginatedJSON(w, rawItems[start:end], total, page, pageSize)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, res)
}

// exported for tests
type ioReader interface{ io.Reader }
