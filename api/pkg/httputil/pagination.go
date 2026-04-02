package httputil

import (
	"math"
	"net/http"
	"strconv"
)

func ParsePagination(r *http.Request) (page int, pageSize int) {
	page = 1
	pageSize = 20

	if v := r.URL.Query().Get("page"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 {
			page = parsed
		}
	}
	if v := r.URL.Query().Get("page_size"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 {
			pageSize = parsed
		}
	}

	if pageSize > 200 {
		pageSize = 200
	}

	return page, pageSize
}

func WritePaginatedJSON(w http.ResponseWriter, items interface{}, total int, page int, pageSize int) {
	pages := 0
	if pageSize > 0 {
		pages = int(math.Ceil(float64(total) / float64(pageSize)))
	}
	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"data":      items,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
		"pages":     pages,
	})
}
