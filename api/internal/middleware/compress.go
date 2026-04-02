package middleware

import (
	"compress/gzip"
	"net/http"
	"strings"
)

type gzipResponseWriter struct {
	http.ResponseWriter
	gzipWriter *gzip.Writer
	decided    bool
	compressed bool
}

func Compress(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(strings.ToLower(r.Header.Get("Accept-Encoding")), "gzip") {
			next.ServeHTTP(w, r)
			return
		}

		grw := &gzipResponseWriter{ResponseWriter: w}
		defer grw.Close()
		next.ServeHTTP(grw, r)
	})
}

func (w *gzipResponseWriter) WriteHeader(statusCode int) {
	if !w.decided {
		w.decideCompression()
	}
	w.ResponseWriter.WriteHeader(statusCode)
}

func (w *gzipResponseWriter) Write(data []byte) (int, error) {
	if !w.decided {
		w.decideCompression()
	}
	if w.compressed && w.gzipWriter != nil {
		return w.gzipWriter.Write(data)
	}
	return w.ResponseWriter.Write(data)
}

func (w *gzipResponseWriter) Flush() {
	if w.compressed && w.gzipWriter != nil {
		_ = w.gzipWriter.Flush()
	}
	if flusher, ok := w.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func (w *gzipResponseWriter) Close() {
	if w.compressed && w.gzipWriter != nil {
		_ = w.gzipWriter.Close()
	}
}

func (w *gzipResponseWriter) decideCompression() {
	w.decided = true
	contentType := strings.ToLower(w.Header().Get("Content-Type"))
	if strings.Contains(contentType, "text/event-stream") {
		w.compressed = false
		return
	}
	w.compressed = true
	w.Header().Set("Content-Encoding", "gzip")
	w.Header().Set("Vary", "Accept-Encoding")
	w.Header().Del("Content-Length")
	w.gzipWriter = gzip.NewWriter(w.ResponseWriter)
}
