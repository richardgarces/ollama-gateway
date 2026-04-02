package httpclient

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type rtFunc func(*http.Request) (*http.Response, error)

func (f rtFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func TestNormalizeOptionsAndClient(t *testing.T) {
	n := normalizeOptions(Options{})
	if n.Timeout <= 0 || n.MaxRetries <= 0 {
		t.Fatalf("expected normalized defaults")
	}
	cl := NewResilientClient(Options{})
	if cl == nil || cl.Transport == nil {
		t.Fatalf("expected non-nil client transport")
	}
}

func TestRetryRoundTripperSuccessAfterRetry(t *testing.T) {
	attempts := 0
	rt := NewRetryRoundTripper(rtFunc(func(r *http.Request) (*http.Response, error) {
		attempts++
		if attempts == 1 {
			return &http.Response{StatusCode: 500, Body: io.NopCloser(strings.NewReader("err")), Header: make(http.Header)}, nil
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("ok")), Header: make(http.Header)}, nil
	}), 2)

	req, _ := http.NewRequest(http.MethodGet, "http://example.com", nil)
	resp, err := rt.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip() error = %v", err)
	}
	if resp.StatusCode != 200 || attempts != 2 {
		t.Fatalf("expected retry and success, status=%d attempts=%d", resp.StatusCode, attempts)
	}
}

func TestRetryRoundTripperContextCanceled(t *testing.T) {
	rt := NewRetryRoundTripper(rtFunc(func(r *http.Request) (*http.Response, error) {
		return nil, context.DeadlineExceeded
	}), 3)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "http://example.com", nil)
	_, err := rt.RoundTrip(req)
	if err == nil {
		t.Fatalf("expected context error")
	}
}

func TestSleepWithContext(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Millisecond)
	defer cancel()
	if sleepWithContext(ctx, 20*time.Millisecond) {
		t.Fatalf("expected sleepWithContext false when context is canceled")
	}
}
