package sdk

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type Client struct {
	baseURL    string
	jwtToken   string
	httpClient *http.Client
}

type Option func(*Client)

func WithHTTPClient(httpClient *http.Client) Option {
	return func(c *Client) {
		if httpClient != nil {
			c.httpClient = httpClient
		}
	}
}

func WithTimeout(timeout time.Duration) Option {
	return func(c *Client) {
		if timeout > 0 {
			c.httpClient.Timeout = timeout
		}
	}
}

func NewClient(baseURL string, jwtToken string, opts ...Option) (*Client, error) {
	trimmed := strings.TrimSpace(baseURL)
	if trimmed == "" {
		return nil, fmt.Errorf("baseURL es requerido")
	}

	c := &Client{
		baseURL:  strings.TrimRight(trimmed, "/"),
		jwtToken: strings.TrimSpace(jwtToken),
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
	for _, opt := range opts {
		if opt != nil {
			opt(c)
		}
	}
	return c, nil
}

func (c *Client) Generate(ctx context.Context, body map[string]interface{}) (map[string]interface{}, error) {
	return c.postJSON(ctx, "/api/v1/generate", body)
}

func (c *Client) Search(ctx context.Context, body map[string]interface{}) (map[string]interface{}, error) {
	return c.postJSON(ctx, "/api/v1/search", body)
}

func (c *Client) CreateJob(ctx context.Context, body map[string]interface{}) (map[string]interface{}, error) {
	return c.postJSON(ctx, "/api/jobs", body)
}

func (c *Client) GetJob(ctx context.Context, id string) (map[string]interface{}, error) {
	return c.getJSON(ctx, "/api/jobs/"+strings.TrimSpace(id))
}

func (c *Client) GetOpenAPISpec(ctx context.Context) (map[string]interface{}, error) {
	return c.getJSON(ctx, "/api/spec")
}

func (c *Client) postJSON(ctx context.Context, path string, body map[string]interface{}) (map[string]interface{}, error) {
	if body == nil {
		body = map[string]interface{}{}
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal body: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.jwtToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.jwtToken)
	}
	return c.do(req)
}

func (c *Client) getJSON(ctx context.Context, path string) (map[string]interface{}, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return nil, err
	}
	if c.jwtToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.jwtToken)
	}
	return c.do(req)
}

func (c *Client) do(req *http.Request) (map[string]interface{}, error) {
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	payload, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var out map[string]interface{}
	if len(bytes.TrimSpace(payload)) > 0 {
		if err := json.Unmarshal(payload, &out); err != nil {
			return nil, fmt.Errorf("decode response: %w", err)
		}
	} else {
		out = map[string]interface{}{}
	}

	if resp.StatusCode >= 400 {
		if msg, ok := out["error"].(string); ok && strings.TrimSpace(msg) != "" {
			return nil, fmt.Errorf("http %d: %s", resp.StatusCode, msg)
		}
		return nil, fmt.Errorf("http %d", resp.StatusCode)
	}
	return out, nil
}
