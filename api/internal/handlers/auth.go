package handlers

import (
	"encoding/json"
	"net/http"
	"time"

	"ollama-gateway/internal/domain"
	"ollama-gateway/pkg/httputil"

	"github.com/golang-jwt/jwt/v5"
)

type AuthHandler struct {
	jwtSecret []byte
}

func NewAuthHandler(jwtSecret []byte) *AuthHandler {
	return &AuthHandler{jwtSecret: jwtSecret}
}

func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req domain.LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "body inválido: "+err.Error())
		return
	}

	if req.Username == "" || req.Password == "" {
		httputil.WriteError(w, http.StatusBadRequest, "username y password requeridos")
		return
	}

	// TODO: Validar contra base de datos real
	// Por ahora, demo user
	if req.Username != "admin" || req.Password != "admin" {
		httputil.WriteError(w, http.StatusUnauthorized, "credenciales inválidas")
		return
	}

	token := h.generateToken(req.Username)
	httputil.WriteJSON(w, http.StatusOK, domain.LoginResponse{Token: token})
}

func (h *AuthHandler) generateToken(user string) string {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"user": user,
		"exp":  time.Now().Add(time.Hour * 24).Unix(),
	})

	t, _ := token.SignedString(h.jwtSecret)
	return t
}
