package observability

import (
	"sync"
	"time"
)

type visitor struct {
	count       int
	windowStart time.Time
	lastSeen    time.Time
}

type RateLimiter struct {
	mu      sync.Mutex
	limit   int
	window  time.Duration
	clients map[string]*visitor
}

func NewRateLimiter(limit int, window time.Duration) *RateLimiter {
	return &RateLimiter{
		limit:   limit,
		window:  window,
		clients: make(map[string]*visitor),
	}
}

func (l *RateLimiter) Allow(key string) bool {
	if l == nil || l.limit <= 0 {
		return true
	}

	now := time.Now()

	l.mu.Lock()
	defer l.mu.Unlock()

	for clientKey, state := range l.clients {
		if now.Sub(state.lastSeen) > 2*l.window {
			delete(l.clients, clientKey)
		}
	}

	state := l.clients[key]
	if state == nil {
		l.clients[key] = &visitor{count: 1, windowStart: now, lastSeen: now}
		return true
	}

	state.lastSeen = now
	if now.Sub(state.windowStart) >= l.window {
		state.count = 1
		state.windowStart = now
		return true
	}

	if state.count >= l.limit {
		return false
	}

	state.count++
	return true
}
