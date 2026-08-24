package newrelic

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func sampleEvents() []Event {
	return []Event{
		{
			EventType: "KaizenOpsSPC",
			Timestamp: time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC),
			Attributes: map[string]any{
				"ucl":         12.5,
				"lcl":         2.1,
				"center_line": 7.3,
				"cpk":         1.42,
			},
		},
	}
}

// decodeGzipJSON descomprime o corpo da requisição e decodifica o JSON
// esperado (array de objetos com eventType/timestamp inline).
func decodeGzipJSON(t *testing.T, body io.Reader) []map[string]any {
	t.Helper()

	zr, err := gzip.NewReader(body)
	if err != nil {
		t.Fatalf("body não é gzip válido: %v", err)
	}
	defer zr.Close()

	raw, err := io.ReadAll(zr)
	if err != nil {
		t.Fatalf("lendo corpo descomprimido: %v", err)
	}

	var out []map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("corpo descomprimido não é o JSON esperado: %v\nraw: %s", err, raw)
	}

	return out
}

func TestSendEvents_SuccessSendsCorrectHeadersAndGzipBody(t *testing.T) {
	var gotAPIKey, gotContentEncoding, gotContentType, gotPath string
	var gotBody []map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAPIKey = r.Header.Get("Api-Key")
		gotContentEncoding = r.Header.Get("Content-Encoding")
		gotContentType = r.Header.Get("Content-Type")
		gotPath = r.URL.Path
		gotBody = decodeGzipJSON(t, r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := NewClient("12345", "insert-key-abc", nil)
	c.baseURL = srv.URL

	if err := c.SendEvents(context.Background(), sampleEvents()); err != nil {
		t.Fatalf("SendEvents retornou erro inesperado: %v", err)
	}

	if gotAPIKey != "insert-key-abc" {
		t.Errorf("header Api-Key = %q, esperado %q", gotAPIKey, "insert-key-abc")
	}
	if gotContentEncoding != "gzip" {
		t.Errorf("header Content-Encoding = %q, esperado %q", gotContentEncoding, "gzip")
	}
	if gotContentType != "application/json" {
		t.Errorf("header Content-Type = %q, esperado %q", gotContentType, "application/json")
	}
	if gotPath != "/v1/accounts/12345/events" {
		t.Errorf("path = %q, esperado %q", gotPath, "/v1/accounts/12345/events")
	}

	if len(gotBody) != 1 {
		t.Fatalf("esperado 1 evento no corpo, veio %d", len(gotBody))
	}

	evt := gotBody[0]
	if evt["eventType"] != "KaizenOpsSPC" {
		t.Errorf("eventType = %v, esperado KaizenOpsSPC", evt["eventType"])
	}
	wantTimestamp := float64(time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC).UnixMilli())
	if evt["timestamp"] != wantTimestamp {
		t.Errorf("timestamp = %v, esperado %v", evt["timestamp"], wantTimestamp)
	}
	if evt["ucl"] != 12.5 || evt["lcl"] != 2.1 || evt["center_line"] != 7.3 || evt["cpk"] != 1.42 {
		t.Errorf("atributos não vieram inline como esperado: %v", evt)
	}
}

func TestSendEvents_RetriesOnceOn500ThenSucceeds(t *testing.T) {
	var calls int32
	var sleptFor time.Duration

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		if n == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte("internal error"))
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := NewClient("12345", "insert-key-abc", nil)
	c.baseURL = srv.URL
	c.retryWait = time.Millisecond
	c.sleep = func(d time.Duration) { sleptFor = d }

	if err := c.SendEvents(context.Background(), sampleEvents()); err != nil {
		t.Fatalf("SendEvents retornou erro inesperado após retry: %v", err)
	}

	if got := atomic.LoadInt32(&calls); got != 2 {
		t.Fatalf("esperado 2 chamadas (1 falha + 1 retry), veio %d", got)
	}
	if sleptFor != time.Millisecond {
		t.Errorf("sleep chamado com %v, esperado %v", sleptFor, time.Millisecond)
	}
}

func TestSendEvents_FailsAfterRetryExhaustedOn500(t *testing.T) {
	var calls int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("still broken"))
	}))
	defer srv.Close()

	c := NewClient("12345", "insert-key-abc", nil)
	c.baseURL = srv.URL
	c.retryWait = time.Millisecond
	c.sleep = func(time.Duration) {}

	err := c.SendEvents(context.Background(), sampleEvents())
	if err == nil {
		t.Fatal("esperado erro após esgotar retries, veio nil")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("erro não menciona o status 500: %v", err)
	}

	if got := atomic.LoadInt32(&calls); got != 2 {
		t.Fatalf("esperado 2 chamadas (1 tentativa + 1 retry), veio %d", got)
	}
}

func TestSendEvents_FailsWithoutRetryOn4xx(t *testing.T) {
	var calls int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte("invalid api key"))
	}))
	defer srv.Close()

	c := NewClient("12345", "bad-key", nil)
	c.baseURL = srv.URL
	c.sleep = func(time.Duration) {
		t.Fatal("sleep não deveria ser chamado em erro 4xx")
	}

	err := c.SendEvents(context.Background(), sampleEvents())
	if err == nil {
		t.Fatal("esperado erro em resposta 403, veio nil")
	}
	if !strings.Contains(err.Error(), "403") {
		t.Errorf("erro não menciona o status 403: %v", err)
	}
	if !strings.Contains(err.Error(), "invalid api key") {
		t.Errorf("erro não inclui o corpo da resposta: %v", err)
	}

	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("esperado exatamente 1 chamada (sem retry em 4xx), veio %d", got)
	}
}

func TestNewClient_DefaultsHTTPClientWhenNil(t *testing.T) {
	c := NewClient("acc", "key", nil)
	if c.httpClient != http.DefaultClient {
		t.Error("esperado http.DefaultClient quando httpClient é nil")
	}
}
