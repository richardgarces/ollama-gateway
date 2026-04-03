package observability

import (
	"math"
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
	window  time.Duration
	clients map[string]*visitor
}

type RateLimitDecision struct {
	Allowed    bool
	Limit      int
	Remaining  int
	ResetAt    time.Time
	RetryAfter int
}

func NewRateLimiter(limit int, window time.Duration) *RateLimiter {
	return &RateLimiter{
		window:  window,
		clients: make(map[string]*visitor),
	}
}

func (l *RateLimiter) Allow(key string) bool {
	if l == nil {
		return true
	}
	decision := l.Check(key, 1, true)
	return decision.Allowed
}

func (l *RateLimiter) Check(key string, limit int, consume bool) RateLimitDecision {
	if l == nil || limit <= 0 {
		return RateLimitDecision{Allowed: true, Limit: limit}
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
		state = &visitor{count: 0, windowStart: now, lastSeen: now}
		l.clients[key] = state
	}

	state.lastSeen = now
	if now.Sub(state.windowStart) >= l.window {
		state.count = 0
		state.windowStart = now
	}

	allowed := state.count < limit
	if allowed && consume {
		state.count++
	}
	remaining := limit - state.count
	if remaining < 0 {
		remaining = 0
	}
	resetAt := state.windowStart.Add(l.window)
	retryAfter := 0
	if !allowed {
		retryAfter = int(math.Ceil(time.Until(resetAt).Seconds()))
		if retryAfter < 1 {
			retryAfter = 1
		}
	}

	return RateLimitDecision{
		Allowed:    allowed,
		Limit:      limit,
		Remaining:  remaining,
		ResetAt:    resetAt,
		RetryAfter: retryAfter,
	}
}
