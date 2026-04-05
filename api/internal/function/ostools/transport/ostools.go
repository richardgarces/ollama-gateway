package transport

import (
	"encoding/json"
	"net/http"

	"ollama-gateway/internal/function/ostools"
	"ollama-gateway/pkg/httputil"
)

type Handler struct {
	svc *ostools.Service
}

func NewHandler(svc *ostools.Service) *Handler {
	return &Handler{svc: svc}
}

// --- Request types ---

type readFileReq struct {
	Path string `json:"path"`
}

type writeFileReq struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

type deleteFileReq struct {
	Path string `json:"path"`
}

type listDirReq struct {
	Path string `json:"path"`
}

type execReq struct {
	Command string   `json:"command"`
	Args    []string `json:"args"`
}

type fileExistsReq struct {
	Path string `json:"path"`
}

// --- Handlers ---

func (h *Handler) ReadFile(w http.ResponseWriter, r *http.Request) {
	var req readFileReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "body inválido: "+err.Error())
		return
	}
	if req.Path == "" {
		httputil.WriteError(w, http.StatusBadRequest, "path requerido")
		return
	}
	content, err := h.svc.ReadFile(r.Context(), req.Path)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"path":    req.Path,
		"content": content,
		"bytes":   len(content),
	})
}

func (h *Handler) WriteFile(w http.ResponseWriter, r *http.Request) {
	var req writeFileReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "body inválido: "+err.Error())
		return
	}
	if req.Path == "" {
		httputil.WriteError(w, http.StatusBadRequest, "path requerido")
		return
	}
	if err := h.svc.WriteFile(r.Context(), req.Path, req.Content); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"path":    req.Path,
		"written": len(req.Content),
		"ok":      true,
	})
}

func (h *Handler) DeleteFile(w http.ResponseWriter, r *http.Request) {
	var req deleteFileReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "body inválido: "+err.Error())
		return
	}
	if req.Path == "" {
		httputil.WriteError(w, http.StatusBadRequest, "path requerido")
		return
	}
	if err := h.svc.DeleteFile(r.Context(), req.Path); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"path":    req.Path,
		"deleted": true,
	})
}

func (h *Handler) ListDir(w http.ResponseWriter, r *http.Request) {
	var req listDirReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "body inválido: "+err.Error())
		return
	}
	if req.Path == "" {
		req.Path = "."
	}
	entries, err := h.svc.ListDir(r.Context(), req.Path)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"path":    req.Path,
		"entries": entries,
		"count":   len(entries),
	})
}

func (h *Handler) Exec(w http.ResponseWriter, r *http.Request) {
	var req execReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "body inválido: "+err.Error())
		return
	}
	if req.Command == "" {
		httputil.WriteError(w, http.StatusBadRequest, "command requerido")
		return
	}
	result, err := h.svc.Exec(r.Context(), req.Command, req.Args)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	httputil.WriteJSON(w, http.StatusOK, result)
}

func (h *Handler) FileExists(w http.ResponseWriter, r *http.Request) {
	var req fileExistsReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "body inválido: "+err.Error())
		return
	}
	if req.Path == "" {
		httputil.WriteError(w, http.StatusBadRequest, "path requerido")
		return
	}
	exists, isDir, err := h.svc.FileExists(r.Context(), req.Path)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"path":   req.Path,
		"exists": exists,
		"is_dir": isDir,
	})
}
