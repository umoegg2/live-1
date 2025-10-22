package backend

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"
)

// Client interacts with the orchestration backend.
type Client struct {
	baseURL string
	http    *http.Client
}

// NewClient builds a new backend client with the provided base URL and http client.
func NewClient(baseURL string, httpClient *http.Client) (*Client, error) {
	if httpClient == nil {
		return nil, errors.New("http client cannot be nil")
	}

	baseURL = strings.TrimRight(baseURL, "/")
	if baseURL == "" {
		return nil, errors.New("base URL cannot be empty")
	}

	if _, err := url.Parse(baseURL); err != nil {
		return nil, fmt.Errorf("invalid base URL: %w", err)
	}

	return &Client{baseURL: baseURL, http: httpClient}, nil
}

// Register informs the backend about the client's capabilities.
func (c *Client) Register(ctx context.Context, payload RegisterRequest) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal register payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/register", strings.NewReader(string(body)))
	if err != nil {
		return fmt.Errorf("build register request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("send register request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 5120))
		return fmt.Errorf("register failed: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(data)))
	}

	return nil
}

// PollCommand performs a long poll against the backend, returning a command if available.
func (c *Client) PollCommand(ctx context.Context, clientID string) (*CommandResponse, error) {
	if strings.TrimSpace(clientID) == "" {
		return nil, errors.New("client id cannot be empty")
	}

	u, err := url.Parse(c.baseURL)
	if err != nil {
		return nil, fmt.Errorf("parse base url: %w", err)
	}
	u.Path = path.Join(u.Path, "command", clientID)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("build poll request: %w", err)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("send poll request: %w", err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusNoContent:
		return nil, nil
	case http.StatusOK:
		var command CommandResponse
		decoder := json.NewDecoder(resp.Body)
		if err := decoder.Decode(&command); err != nil {
			return nil, fmt.Errorf("decode command response: %w", err)
		}
		return &command, nil
	default:
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 5120))
		return nil, fmt.Errorf("poll failed: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(data)))
	}
}

// WithLongPollTimeout returns a context with deadline to avoid waiting forever.
func WithLongPollTimeout(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if deadline, ok := ctx.Deadline(); ok {
		remaining := time.Until(deadline)
		if remaining < timeout {
			timeout = remaining
		}
	}
	return context.WithTimeout(ctx, timeout)
}
