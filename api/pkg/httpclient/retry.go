package httpclient

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"math/rand"
	"net/http"
	"time"

	"ollama-gateway/internal/middleware"
)

type retryRoundTripper struct {
	next        http.RoundTripper
	maxAttempts int
	baseBackoff time.Duration
	maxBackoff  time.Duration
	jitterRatio float64
}

func NewRetryRoundTripper(next http.RoundTripper, maxAttempts int) http.RoundTripper {
	if next == nil {
		next = http.DefaultTransport
	}
	if maxAttempts <= 0 {
		maxAttempts = 3
	}
	return &retryRoundTripper{
		next:        next,
		maxAttempts: maxAttempts,
		baseBackoff: 200 * time.Millisecond,
		maxBackoff:  2 * time.Second,
		jitterRatio: 0.2,
	}
}

func (r *retryRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if req == nil {
		return nil, fmt.Errorf("nil request")
	}

	var lastResp *http.Response
	var lastErr error

	for attempt := 1; attempt <= r.maxAttempts; attempt++ {
		attemptReq, err := cloneForAttempt(req, attempt)
		if err != nil {
			return nil, err
		}

		resp, err := r.next.RoundTrip(attemptReq)
		if !r.shouldRetry(req, resp, err, attempt) {
			return resp, err
		}

		lastResp = resp
		lastErr = err

		if resp != nil && resp.Body != nil {
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
		}

		backoff := r.backoffFor(attempt)
		logRetry(req, attempt, r.maxAttempts, backoff, resp, err)
		if !sleepWithContext(req.Context(), backoff) {
			if req.Context().Err() != nil {
				return nil, req.Context().Err()
			}
			break
		}
	}

	if lastErr != nil {
		return nil, lastErr
	}
	return lastResp, nil
}

func (r *retryRoundTripper) shouldRetry(req *http.Request, resp *http.Response, err error, attempt int) bool {
	if attempt >= r.maxAttempts {
		return false
	}

	if !isReplayableRequest(req) {
		return false
	}

	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return false
		}
		return true
	}

	if resp == nil {
		return false
	}

	return resp.StatusCode >= 500 && resp.StatusCode <= 599
}

func isReplayableRequest(req *http.Request) bool {
	if req.Body == nil || req.Body == http.NoBody {
		return true
	}
	if req.Method == http.MethodPost && req.GetBody == nil {
		return false
	}
	return req.GetBody != nil
}

func cloneForAttempt(req *http.Request, attempt int) (*http.Request, error) {
	if attempt == 1 {
		return req, nil
	}
	cloned := req.Clone(req.Context())
	if req.Body == nil || req.Body == http.NoBody {
		return cloned, nil
	}
	if req.GetBody == nil {
		return nil, fmt.Errorf("request body no reusable para reintento")
	}
	body, err := req.GetBody()
	if err != nil {
		return nil, err
	}
	cloned.Body = body
	return cloned, nil
}

func (r *retryRoundTripper) backoffFor(attempt int) time.Duration {
	if attempt <= 0 {
		attempt = 1
	}
	backoff := r.baseBackoff << (attempt - 1)
	if backoff > r.maxBackoff {
		backoff = r.maxBackoff
	}
	if r.jitterRatio <= 0 || backoff <= 0 {
		return backoff
	}
	base := float64(backoff)
	window := base * r.jitterRatio
	shift := (2*rand.Float64() - 1) * window
	out := base + shift
	if out < 0 {
		out = 0
	}
	return time.Duration(math.Round(out))
}

func sleepWithContext(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}

func logRetry(req *http.Request, attempt int, maxAttempts int, backoff time.Duration, resp *http.Response, err error) {
	requestID := middleware.RequestIDFromContext(req.Context())
	if requestID == "" {
		requestID = "unknown"
	}
	if err != nil {
		log.Printf("request_id=%s http_retry attempt=%d/%d method=%s url=%s reason=connection_error error=%v backoff=%s",
			requestID, attempt, maxAttempts, req.Method, req.URL.String(), err, backoff)
		return
	}
	status := 0
	if resp != nil {
		status = resp.StatusCode
	}
	log.Printf("request_id=%s http_retry attempt=%d/%d method=%s url=%s reason=status_%d backoff=%s",
		requestID, attempt, maxAttempts, req.Method, req.URL.String(), status, backoff)
}
