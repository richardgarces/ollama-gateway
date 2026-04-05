package httpclient

import (
	"net"
	"net/http"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

type Options struct {
	Timeout             time.Duration
	MaxRetries          int
	MaxConnsPerHost     int
	MaxIdleConns        int
	MaxIdleConnsPerHost int
	IdleConnTimeout     time.Duration
	TLSHandshakeTimeout time.Duration
	DialTimeout         time.Duration
}

func NewResilientClient(opts Options) *http.Client {
	n := normalizeOptions(opts)

	transport := &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           (&net.Dialer{Timeout: n.DialTimeout, KeepAlive: 30 * time.Second}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxConnsPerHost:       n.MaxConnsPerHost,
		MaxIdleConns:          n.MaxIdleConns,
		MaxIdleConnsPerHost:   n.MaxIdleConnsPerHost,
		IdleConnTimeout:       n.IdleConnTimeout,
		TLSHandshakeTimeout:   n.TLSHandshakeTimeout,
		ExpectContinueTimeout: 1 * time.Second,
	}

	return &http.Client{
		Timeout:   n.Timeout,
		Transport: otelhttp.NewTransport(NewRetryRoundTripper(transport, n.MaxRetries)),
	}
}

func normalizeOptions(opts Options) Options {
	if opts.Timeout <= 0 {
		opts.Timeout = 30 * time.Second
	}
	if opts.MaxRetries <= 0 {
		opts.MaxRetries = 3
	}
	if opts.MaxIdleConns <= 0 {
		opts.MaxIdleConns = 100
	}
	if opts.MaxConnsPerHost <= 0 {
		opts.MaxConnsPerHost = 64
	}
	if opts.MaxIdleConnsPerHost <= 0 {
		opts.MaxIdleConnsPerHost = 10
	}
	if opts.IdleConnTimeout <= 0 {
		opts.IdleConnTimeout = 90 * time.Second
	}
	if opts.TLSHandshakeTimeout <= 0 {
		opts.TLSHandshakeTimeout = 10 * time.Second
	}
	if opts.DialTimeout <= 0 {
		opts.DialTimeout = 30 * time.Second
	}
	return opts
}
