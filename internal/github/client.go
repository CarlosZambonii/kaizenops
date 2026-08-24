package github

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"
)

// Client é um client REST da API do GitHub Actions, usado para backfill e
// reconciliação. Respeita rate limiting e faz backoff exponencial em erros
// transitórios.
type Client struct {
	httpClient *http.Client
	tokens     *TokenManager
	baseURL    string
	maxRetries int
	backoffMin time.Duration
	backoffMax time.Duration
	sleep      func(ctx context.Context, d time.Duration) error
}

// NewClient cria um Client autenticado via tokens.
func NewClient(tokens *TokenManager, httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}

	return &Client{
		httpClient: httpClient,
		tokens:     tokens,
		baseURL:    "https://api.github.com",
		maxRetries: 5,
		backoffMin: 500 * time.Millisecond,
		backoffMax: 30 * time.Second,
		sleep:      sleepWithContext,
	}
}

func sleepWithContext(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}

	timer := time.NewTimer(d)
	defer timer.Stop()

	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Do executa uma requisição autenticada contra a API do GitHub, com retry
// em erros 5xx e em rate limiting (403/429), respeitando os headers
// Retry-After e X-RateLimit-Reset quando presentes.
func (c *Client) Do(ctx context.Context, method, path string, body []byte) (*http.Response, error) {
	var lastErr error

	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		resp, err := c.do(ctx, method, path, body)

		var retryWait time.Duration
		switch {
		case err != nil:
			lastErr = err
		case !isRetryable(resp.StatusCode):
			return resp, nil
		default:
			lastErr = fmt.Errorf("request failed with status %d", resp.StatusCode)
			if wait, ok := retryAfter(resp); ok {
				retryWait = wait
			}
			_ = resp.Body.Close()
		}

		if attempt == c.maxRetries {
			break
		}
		if retryWait == 0 {
			retryWait = c.backoffFor(attempt + 1)
		}
		if err := c.sleep(ctx, retryWait); err != nil {
			return nil, err
		}
	}

	return nil, fmt.Errorf("calling GitHub API %s %s: %w", method, path, lastErr)
}

func (c *Client) do(ctx context.Context, method, path string, body []byte) (*http.Response, error) {
	token, err := c.tokens.Token(ctx)
	if err != nil {
		return nil, fmt.Errorf("getting installation token: %w", err)
	}

	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return nil, fmt.Errorf("building request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	return c.httpClient.Do(req)
}

func (c *Client) backoffFor(attempt int) time.Duration {
	d := c.backoffMin
	for i := 1; i < attempt; i++ {
		d *= 2
		if d >= c.backoffMax {
			return c.backoffMax
		}
	}
	return d
}

func isRetryable(status int) bool {
	return status == http.StatusForbidden ||
		status == http.StatusTooManyRequests ||
		status >= http.StatusInternalServerError
}

// retryAfter lê Retry-After ou X-RateLimit-Reset e retorna quanto esperar
// antes de tentar de novo.
func retryAfter(resp *http.Response) (time.Duration, bool) {
	if v := resp.Header.Get("Retry-After"); v != "" {
		if secs, err := strconv.Atoi(v); err == nil {
			return time.Duration(secs) * time.Second, true
		}
	}

	if v := resp.Header.Get("X-RateLimit-Remaining"); v == "0" {
		if reset := resp.Header.Get("X-RateLimit-Reset"); reset != "" {
			if unix, err := strconv.ParseInt(reset, 10, 64); err == nil {
				wait := time.Until(time.Unix(unix, 0))
				if wait > 0 {
					return wait, true
				}
			}
		}
	}

	return 0, false
}
