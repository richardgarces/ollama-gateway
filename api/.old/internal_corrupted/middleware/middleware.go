package middleware

import (
	"log"
	"net/http"
	"time"
)

type statusRecorder struct {
	http.ResponseWriter
	statusCode int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.statusCode = code
	r.ResponseWriter.WriteHeader(code)
}

func Logging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, statusCode: http.StatusOK}

		next.ServeHTTP(rec, r)

		log.Printf("%s %s %d %s", r.Method, r.URL.Path, rec.statusCode, time.Since(start))
	})
}

func CORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}
	"log"
	"net/http"
	"time"
)

type statusRecorder struct {
	http.ResponseWriter
	statusCode int
}































}	})		next.ServeHTTP(w, r)		}			return			w.WriteHeader(http.StatusNoContent)		if r.Method == http.MethodOptions {		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")		w.Header().Set("Access-Control-Allow-Origin", "*")	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {func CORS(next http.Handler) http.Handler {}	})		log.Printf("%s %s %d %s", r.Method, r.URL.Path, rec.statusCode, time.Since(start))		next.ServeHTTP(rec, r)		rec := &statusRecorder{ResponseWriter: w, statusCode: http.StatusOK}		start := time.Now()	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {func Logging(next http.Handler) http.Handler {}	r.ResponseWriter.WriteHeader(code)	r.statusCode = codefunc (r *statusRecorder) WriteHeader(code int) {