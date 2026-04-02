package httputil

import (
	"encoding/json"
	"fmt"
	"net/http"
)

type APIError struct {
	Error string `json:"error"`
	Code  int    `json:"code"`
}

func WriteJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func WriteError(w http.ResponseWriter, status int, msg string) {
	WriteJSON(w, status, APIError{Error: msg, Code: status})
}

func WriteSSEHeaders(w http.ResponseWriter) error {
	if _, ok := w.(http.Flusher); !ok {
		return fmt.Errorf("streaming no soportado por ResponseWriter")
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	return nil
}

func WriteSSEData(w http.ResponseWriter, payload interface{}) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "data: %s\n\n", data); err != nil {
		return err
	}
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}
	return nil
}

func WriteSSEDone(w http.ResponseWriter) error {
	if _, err := fmt.Fprint(w, "data: [DONE]\n\n"); err != nil {
		return err
	}
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}
	return nil
}
