package github

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func newTestClient(t *testing.T, server *httptest.Server) *Client {
	t.Helper()

	pemKey, _ := generateTestPrivateKeyPEM(t)
	auth, err := NewAppAuthenticator(1, pemKey)
	if err != nil {
		t.Fatalf("NewAppAuthenticator() error = %v", err)
	}

	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		fmt.Fprintf(w, `{"token": "test-token", "expires_at": %q}`, time.Now().Add(time.Hour).Format(time.RFC3339))
	}))
	t.Cleanup(tokenServer.Close)

	tokens := NewTokenManager(auth, 1, tokenServer.Client())
	tokens.baseURL = tokenServer.URL

	client := NewClient(tokens, server.Client())
	client.baseURL = server.URL
	client.backoffMin = time.Millisecond
	client.backoffMax = 5 * time.Millisecond
	client.sleep = func(ctx context.Context, d time.Duration) error {
		// Sem espera real nos testes; ainda observa ctx para exercitar o
		// caminho de cancelamento.
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			return nil
		}
	}

	return client
}

func TestClientDoSucceedsOnFirstTry(t *testing.T) {
	var requests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
			t.Errorf("Authorization header = %q", got)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := newTestClient(t, server)

	resp, err := client.Do(context.Background(), http.MethodGet, "/repos/carlosz/kaizenops/actions/runs", nil)
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if requests != 1 {
		t.Fatalf("requests = %d, want 1", requests)
	}
}

func TestClientDoRetriesOn5xx(t *testing.T) {
	var requests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if requests < 3 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := newTestClient(t, server)

	resp, err := client.Do(context.Background(), http.MethodGet, "/repos/carlosz/kaizenops/actions/runs", nil)
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	defer resp.Body.Close()

	if requests != 3 {
		t.Fatalf("requests = %d, want 3", requests)
	}
}

func TestClientDoRespectsRetryAfter(t *testing.T) {
	var requests int
	var sleptFor time.Duration

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if requests == 1 {
			w.Header().Set("Retry-After", "7")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := newTestClient(t, server)
	client.sleep = func(ctx context.Context, d time.Duration) error {
		sleptFor = d
		return nil
	}

	resp, err := client.Do(context.Background(), http.MethodGet, "/x", nil)
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	defer resp.Body.Close()

	if sleptFor != 7*time.Second {
		t.Fatalf("sleptFor = %v, want 7s", sleptFor)
	}
}

func TestClientDoGivesUpAfterMaxRetries(t *testing.T) {
	var requests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client := newTestClient(t, server)
	client.maxRetries = 2

	_, err := client.Do(context.Background(), http.MethodGet, "/x", nil)
	if err == nil {
		t.Fatal("Do() expected error after exhausting retries")
	}
	if requests != 3 {
		t.Fatalf("requests = %d, want 3 (1 initial + 2 retries)", requests)
	}
}

func TestClientDoStopsOnContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client := newTestClient(t, server)
	ctx, cancel := context.WithCancel(context.Background())
	client.sleep = func(ctx context.Context, d time.Duration) error {
		cancel()
		return ctx.Err()
	}

	_, err := client.Do(ctx, http.MethodGet, "/x", nil)
	if err == nil {
		t.Fatal("Do() expected error on context cancellation")
	}
}

func TestBackoffForGrowsExponentiallyAndCaps(t *testing.T) {
	c := &Client{backoffMin: time.Second, backoffMax: 8 * time.Second}

	tests := []struct {
		attempt int
		want    time.Duration
	}{
		{1, 1 * time.Second},
		{2, 2 * time.Second},
		{3, 4 * time.Second},
		{4, 8 * time.Second},
		{5, 8 * time.Second},
	}

	for _, tt := range tests {
		if got := c.backoffFor(tt.attempt); got != tt.want {
			t.Errorf("backoffFor(%d) = %v, want %v", tt.attempt, got, tt.want)
		}
	}
}
