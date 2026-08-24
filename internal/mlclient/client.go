package mlclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// defaultFailureThreshold e defaultOpenDuration definem o comportamento
// padrão do circuit breaker interno do Client quando nenhum outro valor é
// configurado explicitamente.
const (
	defaultFailureThreshold = 3
	defaultOpenDuration     = 30 * time.Second
)

// AnomalyRequest é o payload enviado a POST /anomaly do serviço de ML.
type AnomalyRequest struct {
	JobName         string
	DurationSeconds float64
	QueueSeconds    float64
	StartedAt       time.Time
}

// anomalyRequestBody é a forma JSON exigida pelo contrato do serviço de ML.
type anomalyRequestBody struct {
	JobName         string  `json:"job_name"`
	DurationSeconds float64 `json:"duration_seconds"`
	QueueSeconds    float64 `json:"queue_seconds"`
	StartedAt       string  `json:"started_at"`
}

// anomalyResponseBody é a forma JSON da resposta 200 de POST /anomaly.
type anomalyResponseBody struct {
	IsAnomaly    bool    `json:"is_anomaly"`
	Score        float64 `json:"score"`
	ModelVersion string  `json:"model_version"`
}

// AnomalyResult é o resultado de uma consulta de anomalia ao serviço de ML.
// Available reporta se o serviço respondeu com sucesso: quando false, os
// demais campos não têm significado e o caller deve simplesmente seguir
// sem o enriquecimento de ML — não é uma condição de erro.
type AnomalyResult struct {
	Available    bool
	IsAnomaly    bool
	Score        float64
	ModelVersion string
}

// Client chama o serviço de ML (Python/FastAPI) protegido por um circuit
// breaker interno. Nenhuma falha do serviço de ML (rede, timeout, 5xx,
// circuito aberto) é propagada como erro: o ML é enriquecimento, nunca
// ponto único de falha do KaizenOps.
type Client struct {
	baseURL    string
	httpClient *http.Client
	breaker    *CircuitBreaker
}

// NewClient cria um Client para o serviço de ML em baseURL. Se httpClient
// for nil, usa http.DefaultClient. O circuit breaker interno abre após
// defaultFailureThreshold falhas consecutivas e permanece aberto por
// defaultOpenDuration.
func NewClient(baseURL string, httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}

	return &Client{
		baseURL:    baseURL,
		httpClient: httpClient,
		breaker:    NewCircuitBreaker(defaultFailureThreshold, defaultOpenDuration),
	}
}

// DetectAnomaly chama POST {baseURL}/anomaly. Em qualquer falha de
// comunicação com o serviço de ML — rede, timeout, status HTTP >= 500
// (incluindo 503, "sem modelo carregado ainda"), ou circuito aberto —
// retorna AnomalyResult{Available: false} com error nil: essa é a
// degradação graciosa esperada, não uma falha do chamador.
//
// error só é não-nil quando o request em si é malformado antes de qualquer
// tentativa de rede, por exemplo um ctx já cancelado.
func (c *Client) DetectAnomaly(ctx context.Context, req AnomalyRequest) (AnomalyResult, error) {
	if err := ctx.Err(); err != nil {
		return AnomalyResult{}, fmt.Errorf("checando contexto antes de consultar ML service: %w", err)
	}

	if !c.breaker.Allow() {
		return AnomalyResult{Available: false}, nil
	}

	result, ok := c.doDetectAnomaly(ctx, req)
	if !ok {
		c.breaker.RecordFailure()
		return AnomalyResult{Available: false}, nil
	}

	c.breaker.RecordSuccess()
	return result, nil
}

// doDetectAnomaly executa a chamada HTTP propriamente dita. O segundo
// retorno indica se a chamada deve ser contabilizada como sucesso (true)
// ou falha (false) para o circuit breaker; neste último caso o Client
// sempre degrada para AnomalyResult{Available: false} sem propagar error.
func (c *Client) doDetectAnomaly(ctx context.Context, req AnomalyRequest) (AnomalyResult, bool) {
	body := anomalyRequestBody{
		JobName:         req.JobName,
		DurationSeconds: req.DurationSeconds,
		QueueSeconds:    req.QueueSeconds,
		StartedAt:       req.StartedAt.Format(time.RFC3339),
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return AnomalyResult{}, false
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/anomaly", bytes.NewReader(payload))
	if err != nil {
		return AnomalyResult{}, false
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return AnomalyResult{}, false
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, resp.Body)
		return AnomalyResult{}, false
	}

	var respBody anomalyResponseBody
	if err := json.NewDecoder(resp.Body).Decode(&respBody); err != nil {
		return AnomalyResult{}, false
	}

	return AnomalyResult{
		Available:    true,
		IsAnomaly:    respBody.IsAnomaly,
		Score:        respBody.Score,
		ModelVersion: respBody.ModelVersion,
	}, true
}
