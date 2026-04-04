package middleware

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
)

func WithDeprecationHeaders(next http.Handler, successorPath, sunsetDate string) http.Handler {
	if next == nil {
		return http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})
	}
	successorPath = strings.TrimSpace(successorPath)
	sunsetDate = strings.TrimSpace(sunsetDate)
	if sunsetDate == "" {
		sunsetDate = "2026-12-31"
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Deprecation", "true")
		w.Header().Set("X-API-Deprecated", "true")
		w.Header().Set("X-API-Sunset-Date", sunsetDate)
		w.Header().Set("Sunset", sunsetDate+"T23:59:59Z")
		if successorPath != "" {
			w.Header().Set("Link", "<"+successorPath+">; rel=\"successor-version\"")
			w.Header().Set("Warning", "299 - \"Deprecated API: migrate to "+successorPath+" before "+sunsetDate+"\"")
		}
		next.ServeHTTP(w, r)
	})
}

func WithJSONFieldAliases(next http.Handler, aliases map[string]string) http.Handler {
	if next == nil || len(aliases) == 0 {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r == nil {
			next.ServeHTTP(w, r)
			return
		}
		if !shouldTranslateJSON(r.Method, r.Header.Get("Content-Type")) {
			next.ServeHTTP(w, r)
			return
		}
		body, err := readRequestBody(r)
		if err != nil || len(bytes.TrimSpace(body)) == 0 {
			next.ServeHTTP(w, r)
			return
		}

		var payload map[string]json.RawMessage
		if err := json.Unmarshal(body, &payload); err != nil {
			next.ServeHTTP(w, r)
			return
		}

		translated := make([]string, 0, len(aliases))
		for oldField, newField := range aliases {
			oldField = strings.TrimSpace(oldField)
			newField = strings.TrimSpace(newField)
			if oldField == "" || newField == "" {
				continue
			}
			oldValue, ok := payload[oldField]
			if !ok {
				continue
			}
			if _, hasNew := payload[newField]; hasNew {
				continue
			}
			payload[newField] = oldValue
			translated = append(translated, oldField+"->"+newField)
		}

		if len(translated) > 0 {
			updated, err := json.Marshal(payload)
			if err == nil {
				setRequestBody(r, updated)
				w.Header().Set("X-API-Translated-Fields", strings.Join(translated, ","))
			}
		}

		next.ServeHTTP(w, r)
	})
}

func shouldTranslateJSON(method, contentType string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch:
	default:
		return false
	}
	contentType = strings.ToLower(strings.TrimSpace(contentType))
	if contentType == "" {
		return true
	}
	return strings.Contains(contentType, "application/json")
}

func readRequestBody(r *http.Request) ([]byte, error) {
	if r == nil || r.Body == nil {
		return nil, nil
	}
	defer r.Body.Close()
	return bytes.Clone(mustReadAll(r.Body)), nil
}

func mustReadAll(body httpBody) []byte {
	buf := new(bytes.Buffer)
	_, _ = buf.ReadFrom(body)
	return buf.Bytes()
}

type httpBody interface {
	Read([]byte) (int, error)
}

func setRequestBody(r *http.Request, body []byte) {
	if r == nil {
		return
	}
	r.Body = http.NoBody
	if len(body) > 0 {
		r.Body = ioNopCloser{Reader: bytes.NewReader(body)}
	}
	r.ContentLength = int64(len(body))
	r.Header.Set("Content-Length", strconv.Itoa(len(body)))
}

type ioNopCloser struct {
	Reader *bytes.Reader
}

func (c ioNopCloser) Read(p []byte) (int, error) {
	if c.Reader == nil {
		return 0, nil
	}
	return c.Reader.Read(p)
}

func (c ioNopCloser) Close() error { return nil }
