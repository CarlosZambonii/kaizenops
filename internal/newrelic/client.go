// Package newrelic envia eventos já enriquecidos com limites de controle
// (UCL, LCL, linha central, Cpk) para a New Relic Event API. A inteligência
// estatística vive no motor SPC em Go; este client só transporta os
// atributos já calculados para que o New Relic os plote via NRQL.
package newrelic

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Event é um evento custom enviado ao New Relic. Attributes deve incluir
// ucl, lcl, center_line e cpk quando aplicável — quem monta o map é o
// caller (motor SPC), não este pacote.
type Event struct {
	EventType  string
	Timestamp  time.Time
	Attributes map[string]any
}

// Client é um client HTTP para a New Relic Event API (Insights Insert API).
type Client struct {
	httpClient   *http.Client
	accountID    string
	insertAPIKey string
	baseURL      string
	maxRetries   int
	retryWait    time.Duration
	sleep        func(time.Duration)
}

// NewClient cria um Client autenticado com o Insert API Key (Insights
// insert key) da conta New Relic. Se httpClient for nil, usa
// http.DefaultClient.
func NewClient(accountID, insertAPIKey string, httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}

	return &Client{
		httpClient:   httpClient,
		accountID:    accountID,
		insertAPIKey: insertAPIKey,
		baseURL:      "https://insights-collector.newrelic.com",
		maxRetries:   1,
		retryWait:    500 * time.Millisecond,
		sleep:        func(d time.Duration) { time.Sleep(d) },
	}
}

// SendEvents envia um lote de eventos para a Event API via POST, com corpo
// JSON comprimido em gzip. Faz 1 retry simples (sleep fixo curto, sem
// backoff exponencial) em resposta 5xx. Retorna erro claro, incluindo o
// status HTTP, se a resposta final não for 2xx.
func (c *Client) SendEvents(ctx context.Context, events []Event) error {
	body, err := encodeEvents(events)
	if err != nil {
		return fmt.Errorf("encoding events: %w", err)
	}

	var lastErr error

	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		resp, err := c.post(ctx, body)
		if err != nil {
			return fmt.Errorf("sending events to New Relic: %w", err)
		}

		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			_ = resp.Body.Close()
			return nil
		}

		respBody, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		lastErr = fmt.Errorf("new relic event api returned status %d: %s", resp.StatusCode, string(respBody))

		if resp.StatusCode < 500 {
			return lastErr
		}
		if attempt == c.maxRetries {
			break
		}
		c.sleep(c.retryWait)
	}

	return fmt.Errorf("sending events to New Relic: %w", lastErr)
}

func (c *Client) post(ctx context.Context, body []byte) (*http.Response, error) {
	compressed, err := gzipCompress(body)
	if err != nil {
		return nil, fmt.Errorf("compressing request body: %w", err)
	}

	url := fmt.Sprintf("%s/v1/accounts/%s/events", c.baseURL, c.accountID)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(compressed))
	if err != nil {
		return nil, fmt.Errorf("building request: %w", err)
	}
	req.Header.Set("Api-Key", c.insertAPIKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Content-Encoding", "gzip")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("doing request: %w", err)
	}

	return resp, nil
}

// encodeEvents serializa os eventos no formato esperado pela Event API:
// cada evento é um único objeto JSON com eventType e timestamp (unix
// millis) inline junto dos atributos, não aninhados.
func encodeEvents(events []Event) ([]byte, error) {
	payload := make([]map[string]any, 0, len(events))

	for _, e := range events {
		obj := make(map[string]any, len(e.Attributes)+2)
		for k, v := range e.Attributes {
			obj[k] = v
		}
		obj["eventType"] = e.EventType
		obj["timestamp"] = e.Timestamp.UnixMilli()
		payload = append(payload, obj)
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshaling events: %w", err)
	}

	return data, nil
}

func gzipCompress(data []byte) ([]byte, error) {
	var buf bytes.Buffer

	gw := gzip.NewWriter(&buf)
	if _, err := gw.Write(data); err != nil {
		_ = gw.Close()
		return nil, fmt.Errorf("writing gzip data: %w", err)
	}
	if err := gw.Close(); err != nil {
		return nil, fmt.Errorf("closing gzip writer: %w", err)
	}

	return buf.Bytes(), nil
}
