package mlclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestClient_DetectAnomaly_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/anomaly" {
			t.Errorf("path inesperado: %s", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Errorf("método inesperado: %s", r.Method)
		}

		var body anomalyRequestBody
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decodificando request body: %v", err)
		}
		if body.JobName != "build" {
			t.Errorf("job_name inesperado: %q", body.JobName)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(anomalyResponseBody{
			IsAnomaly:    true,
			Score:        0.87,
			ModelVersion: "isoforest-2026.01",
		})
	}))
	defer srv.Close()

	c := NewClient(srv.URL, srv.Client())

	result, err := c.DetectAnomaly(context.Background(), AnomalyRequest{
		JobName:         "build",
		DurationSeconds: 120,
		QueueSeconds:    5,
		StartedAt:       time.Now(),
	})

	if err != nil {
		t.Fatalf("esperava err nil, recebeu: %v", err)
	}
	if !result.Available {
		t.Fatal("esperava Available==true")
	}
	if !result.IsAnomaly {
		t.Error("esperava IsAnomaly==true")
	}
	if result.Score != 0.87 {
		t.Errorf("score inesperado: %v", result.Score)
	}
	if result.ModelVersion != "isoforest-2026.01" {
		t.Errorf("model_version inesperado: %q", result.ModelVersion)
	}
}

func TestClient_DetectAnomaly_ServiceUnavailable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	c := NewClient(srv.URL, srv.Client())

	result, err := c.DetectAnomaly(context.Background(), AnomalyRequest{JobName: "build"})

	if err != nil {
		t.Fatalf("esperava err nil em degradação graciosa, recebeu: %v", err)
	}
	if result.Available {
		t.Fatal("esperava Available==false quando o serviço responde 503")
	}
}

func TestClient_DetectAnomaly_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := NewClient(srv.URL, srv.Client())

	result, err := c.DetectAnomaly(context.Background(), AnomalyRequest{JobName: "build"})

	if err != nil {
		t.Fatalf("esperava err nil, recebeu: %v", err)
	}
	if result.Available {
		t.Fatal("esperava Available==false em resposta 500")
	}
}

func TestClient_DetectAnomaly_NetworkError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	baseURL := srv.URL
	srv.Close() // servidor já fechado: qualquer chamada falha por erro de rede

	c := NewClient(baseURL, http.DefaultClient)

	result, err := c.DetectAnomaly(context.Background(), AnomalyRequest{JobName: "build"})

	if err != nil {
		t.Fatalf("esperava err nil em degradação graciosa, recebeu: %v", err)
	}
	if result.Available {
		t.Fatal("esperava Available==false quando o servidor está inacessível")
	}
}

func TestClient_DetectAnomaly_CanceledContextReturnsError(t *testing.T) {
	c := NewClient("http://example.invalid", http.DefaultClient)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := c.DetectAnomaly(ctx, AnomalyRequest{JobName: "build"})

	if err == nil {
		t.Fatal("esperava err != nil com contexto já cancelado")
	}
}

func TestClient_DetectAnomaly_CircuitOpensAfterConsecutiveFailuresAndRecoversAfterOpenDuration(t *testing.T) {
	var failNext bool = true
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if failNext {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(anomalyResponseBody{
			IsAnomaly:    false,
			Score:        0.1,
			ModelVersion: "isoforest-2026.01",
		})
	}))
	defer srv.Close()

	c := NewClient(srv.URL, srv.Client())

	// Troca o breaker por um com threshold baixo e clock injetável, para
	// não depender de sleep real nem de 3 falhas com o default.
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	current := start
	c.breaker = NewCircuitBreaker(2, 10*time.Second)
	c.breaker.now = func() time.Time { return current }

	// 2 falhas consecutivas -> abre o circuito.
	for i := 0; i < 2; i++ {
		result, err := c.DetectAnomaly(context.Background(), AnomalyRequest{JobName: "build"})
		if err != nil {
			t.Fatalf("iteração %d: esperava err nil, recebeu %v", i, err)
		}
		if result.Available {
			t.Fatalf("iteração %d: esperava Available==false (servidor retorna 503)", i)
		}
	}

	// Circuito aberto: nem tenta a chamada HTTP. Trocamos o servidor para
	// "sucesso" só para provar que, mesmo assim, o resultado é
	// Available==false — ou seja, a chamada HTTP não foi feita.
	failNext = false

	result, err := c.DetectAnomaly(context.Background(), AnomalyRequest{JobName: "build"})
	if err != nil {
		t.Fatalf("esperava err nil, recebeu: %v", err)
	}
	if result.Available {
		t.Fatal("esperava Available==false: circuito deveria estar aberto e não tentar a chamada")
	}

	// Avança o clock além de openDuration: o circuito deve ir para
	// half-open, tentar de novo e, como o servidor agora responde com
	// sucesso, fechar e devolver Available==true.
	current = start.Add(10 * time.Second)

	result, err = c.DetectAnomaly(context.Background(), AnomalyRequest{JobName: "build"})
	if err != nil {
		t.Fatalf("esperava err nil, recebeu: %v", err)
	}
	if !result.Available {
		t.Fatal("esperava Available==true: openDuration expirou e o serviço voltou a responder com sucesso")
	}
}
