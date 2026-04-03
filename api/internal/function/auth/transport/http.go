package transport

type Handler = AuthHandler

func NewHandler(jwtSecret []byte) *Handler {
	return NewAuthHandler(jwtSecret)
}
