package handlers

import (
	"net/http"

	"ollama-gateway/pkg/httputil"
)

func Health(w http.ResponseWriter, r *http.Request) {
	httputil.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

import (
	"net/http"

	"ollama-gateway/pkg/httputil"
)

func Health(w http.ResponseWriter, r *http.Request) {
	httputil.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
