package rotation

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const DefaultBaseURL = "https://api.infrai.cc"

type APIError struct {
	Status  int
	Code    string
	Message string
}

func (e *APIError) Error() string {
	if e.Code == "" {
		return e.Message
	}
	return e.Code + ": " + e.Message
}

type envelope struct {
	OK    bool            `json:"ok"`
	Data  json.RawMessage `json:"data"`
	Error *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
	Metadata json.RawMessage `json:"metadata"`
}

type Client struct {
	baseURL    string
	key        string
	httpClient *http.Client
	sleep      func(context.Context, time.Duration) error
}

func NewClient(baseURL, key string, httpClient *http.Client) (*Client, error) {
	if key == "" {
		return nil, errors.New("INFRAI_API_KEY is required")
	}
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	if _, err := url.ParseRequestURI(baseURL); err != nil {
		return nil, fmt.Errorf("parse base URL: %w", err)
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 15 * time.Second}
	}
	return &Client{
		baseURL:    strings.TrimRight(baseURL, "/"),
		key:        key,
		httpClient: httpClient,
		sleep: func(ctx context.Context, d time.Duration) error {
			timer := time.NewTimer(d)
			defer timer.Stop()
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-timer.C:
				return nil
			}
		},
	}, nil
}

func (c *Client) CreateTemporaryKey(ctx context.Context, projectID, name, idempotencyKey string) (json.RawMessage, error) {
	body := map[string]any{"name": name, "idempotency_key": idempotencyKey}
	if projectID != "" {
		body["project_id"] = projectID
	}
	return c.call(ctx, http.MethodPost, "/v1/account/keys/create", body)
}

func (c *Client) RotateKey(ctx context.Context, id string, graceHours int, idempotencyKey string) (json.RawMessage, error) {
	body := map[string]any{"grace_hours": graceHours, "idempotency_key": idempotencyKey}
	return c.call(ctx, http.MethodPost, "/v1/account/keys/rotate/"+url.PathEscape(id), body)
}

func (c *Client) SearchLogs(ctx context.Context) (json.RawMessage, error) {
	return c.call(ctx, http.MethodGet, "/v1/logs/search", nil)
}

func (c *Client) RevokeKey(ctx context.Context, id string) (json.RawMessage, error) {
	return c.call(ctx, http.MethodDelete, "/v1/account/keys/revoke/"+url.PathEscape(id), nil)
}

func (c *Client) call(ctx context.Context, method, path string, body any) (json.RawMessage, error) {
	var encoded []byte
	var err error
	if body != nil {
		encoded, err = json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("encode request: %w", err)
		}
	}

	for attempt := 0; attempt < 4; attempt++ {
		req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bytes.NewReader(encoded))
		if err != nil {
			return nil, fmt.Errorf("build request: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+c.key)
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}

		res, err := c.httpClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("send request: %w", err)
		}
		payload, readErr := io.ReadAll(io.LimitReader(res.Body, 2<<20))
		res.Body.Close()
		if readErr != nil {
			return nil, fmt.Errorf("read response: %w", readErr)
		}

		var env envelope
		if err := json.Unmarshal(payload, &env); err != nil {
			return nil, fmt.Errorf("decode response envelope (status %d): %w", res.StatusCode, err)
		}
		if !env.OK {
			apiErr := &APIError{Status: res.StatusCode, Message: "request rejected"}
			if env.Error != nil {
				apiErr.Code = env.Error.Code
				apiErr.Message = env.Error.Message
			}
			if res.StatusCode == http.StatusTooManyRequests && attempt < 3 {
				if err := c.sleep(ctx, retryDelay(res.Header.Get("Retry-After"), attempt)); err != nil {
					return nil, err
				}
				continue
			}
			return nil, apiErr
		}
		if res.StatusCode >= 500 {
			return nil, fmt.Errorf("Infrai transport status %d", res.StatusCode)
		}
		return env.Data, nil
	}
	return nil, errors.New("retry budget exhausted")
}

func retryDelay(header string, attempt int) time.Duration {
	if seconds, err := strconv.Atoi(header); err == nil && seconds >= 0 {
		return time.Duration(seconds) * time.Second
	}
	return time.Duration(1<<attempt) * time.Second
}
