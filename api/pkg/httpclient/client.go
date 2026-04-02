package httpclient

import (
	"net"
	"net/http"
	"time"
)

type Options struct {
	Timeout             time.Duration
	MaxRetries          int
	MaxIdleConns        int
	MaxIdleConnsPerHost int
	IdleConnTimeout     time.Duration
	TLSHandshakeTimeout time.Duration
}

func NewResilientClient(opts Options) *http.Client {
	n := normalizeOptions(opts)

	transport := &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           (&net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          n.MaxIdleConns,
		MaxIdleConnsPerHost:   n.MaxIdleConnsPerHost,
		IdleConnTimeout:       n.IdleConnTimeout,
		TLSHandshakeTimeout:   n.TLSHandshakeTimeout,
		ExpectContinueTimeout: 1 * time.Second,
	}

	return &http.Client{
		Timeout:   n.Timeout,
		Transport: NewRetryRoundTripper(transport, n.MaxRetries),
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
	if opts.MaxIdleConnsPerHost <= 0 {
		opts.MaxIdleConnsPerHost = 10
	}
	if opts.IdleConnTimeout <= 0 {
		opts.IdleConnTimeout = 90 * time.Second
	}
	if opts.TLSHandshakeTimeout <= 0 {
		opts.TLSHandshakeTimeout = 10 * time.Second
	}
	return opts
}
