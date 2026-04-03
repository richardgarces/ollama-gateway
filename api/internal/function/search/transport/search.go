package transport

import (
	"encoding/json"
	"io"
	"net/http"
	"sort"

	"ollama-gateway/internal/function/core/domain"
	"ollama-gateway/pkg/httputil"
	"ollama-gateway/pkg/reposcope"
)

type SearchHandler struct {
	ollama domain.OllamaClient
	qdrant domain.VectorStore
	repos  []string
}

func NewSearchHandler(o domain.OllamaClient, q domain.VectorStore, repos []string) *SearchHandler {
	return &SearchHandler{ollama: o, qdrant: q, repos: reposcope.CanonicalizeRoots(repos)}
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
	collections := h.collectionsForRequest(r)
	merged := make([]map[string]interface{}, 0)
	for _, collection := range collections {
		res, err := h.qdrant.Search(collection, emb, req.Top)
		if err != nil {
			continue
		}
		items, ok := res["result"].([]interface{})
		if !ok {
			continue
		}
		for _, item := range items {
			if m, ok := item.(map[string]interface{}); ok {
				merged = append(merged, m)
			}
		}
	}

	if len(merged) == 0 {
		httputil.WriteJSON(w, http.StatusOK, map[string]interface{}{"result": []interface{}{}})
		return
	}

	sort.Slice(merged, func(i, j int) bool {
		return scoreOf(merged[i]) > scoreOf(merged[j])
	})
	if req.Top > 0 && len(merged) > req.Top {
		merged = merged[:req.Top]
	}
	res := map[string]interface{}{"result": toInterfaceSlice(merged)}

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

func (h *SearchHandler) collectionsForRequest(r *http.Request) []string {
	repoFilter := r.URL.Query().Get("repo")
	if repoFilter != "" {
		if matched, ok := reposcope.MatchRepoFilter(repoFilter, h.repos); ok {
			return []string{reposcope.CollectionName(matched)}
		}
	}

	if len(h.repos) == 0 {
		return []string{"repo_docs"}
	}
	out := make([]string, 0, len(h.repos))
	seen := make(map[string]struct{})
	for _, repo := range h.repos {
		name := reposcope.CollectionName(repo)
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		out = append(out, name)
	}
	return out
}

func scoreOf(item map[string]interface{}) float64 {
	v, ok := item["score"]
	if !ok {
		return 0
	}
	s, ok := v.(float64)
	if !ok {
		return 0
	}
	return s
}

func toInterfaceSlice(items []map[string]interface{}) []interface{} {
	out := make([]interface{}, 0, len(items))
	for _, it := range items {
		out = append(out, it)
	}
	return out
}

// exported for tests
type ioReader interface{ io.Reader }
