package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/carlosz/kaizenops/internal/dora"
	"github.com/carlosz/kaizenops/internal/newrelic"
	"github.com/carlosz/kaizenops/internal/spc"
	"github.com/carlosz/kaizenops/internal/storage"
)

// reportWindow é o período considerado para todo o relatório (carta I-MR,
// capacidade, Pareto e DORA). Fixo por enquanto; virar parâmetro de query
// é um ajuste natural mais adiante.
const reportWindow = 30 * 24 * time.Hour

func main() {
	if err := run(); err != nil {
		slog.Error("spc exited with error", "err", err)
		os.Exit(1)
	}
}

type config struct {
	databaseURL        string
	httpAddr           string
	upperSpecSeconds   *float64
	doraDeployWorkflow string
	newRelicAccountID  string
	newRelicInsertKey  string
}

func loadConfig() (config, error) {
	cfg := config{
		databaseURL:        os.Getenv("DATABASE_URL"),
		httpAddr:           envOrDefault("HTTP_ADDR", ":8081"),
		doraDeployWorkflow: os.Getenv("DORA_DEPLOY_WORKFLOW"),
		newRelicAccountID:  os.Getenv("NEW_RELIC_ACCOUNT_ID"),
		newRelicInsertKey:  os.Getenv("NEW_RELIC_INSERT_API_KEY"),
	}

	if cfg.databaseURL == "" {
		return config{}, fmt.Errorf("loading config: missing required env var DATABASE_URL")
	}

	if raw := os.Getenv("SPC_UPPER_SPEC_SECONDS"); raw != "" {
		v, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			return config{}, fmt.Errorf("parsing SPC_UPPER_SPEC_SECONDS: %w", err)
		}
		cfg.upperSpecSeconds = &v
	}

	return cfg, nil
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func run() error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	store, err := storage.Open(ctx, cfg.databaseURL)
	if err != nil {
		return fmt.Errorf("opening storage: %w", err)
	}
	defer store.Close()

	if err := storage.Migrate(ctx, store.Pool(), "migrations"); err != nil {
		return fmt.Errorf("running migrations: %w", err)
	}

	var nrClient *newrelic.Client
	if cfg.newRelicAccountID != "" && cfg.newRelicInsertKey != "" {
		nrClient = newrelic.NewClient(cfg.newRelicAccountID, cfg.newRelicInsertKey, nil)
		slog.Info("New Relic event export enabled")
	} else {
		slog.Info("New Relic event export disabled (NEW_RELIC_ACCOUNT_ID/NEW_RELIC_INSERT_API_KEY not set)")
	}

	handler := &reportHandler{
		store:              store,
		newRelic:           nrClient,
		upperSpecSeconds:   cfg.upperSpecSeconds,
		doraDeployWorkflow: cfg.doraDeployWorkflow,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/spc/report", handler.serveReport)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	server := &http.Server{
		Addr:              cfg.httpAddr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	serverErr := make(chan error, 1)
	go func() {
		slog.Info("spc listening", "addr", cfg.httpAddr)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
			return
		}
		serverErr <- nil
	}()

	select {
	case <-ctx.Done():
		slog.Info("shutting down spc")
	case err := <-serverErr:
		if err != nil {
			return fmt.Errorf("running HTTP server: %w", err)
		}
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutting down HTTP server: %w", err)
	}

	return nil
}

type reportHandler struct {
	store              *storage.Store
	newRelic           *newrelic.Client
	upperSpecSeconds   *float64
	doraDeployWorkflow string
}

type reportResponse struct {
	Repo             string                `json:"repo"`
	Job              string                `json:"job"`
	Chart            *spc.IMRChart         `json:"chart,omitempty"`
	NelsonViolations []spc.NelsonViolation `json:"nelson_violations,omitempty"`
	Capability       *capabilityResponse   `json:"capability,omitempty"`
	Pareto           []spc.ParetoEntry     `json:"pareto,omitempty"`
	DORA             *dora.Metrics         `json:"dora,omitempty"`
	Warnings         []string              `json:"warnings,omitempty"`
}

// capabilityResponse é a versão JSON-segura de spc.CapabilityResult: Cp é
// NaN quando só um dos limites de spec é informado, e tanto Cp quanto Cpk
// podem virar +-Inf quando sigma é 0 (todas as amostras idênticas) — ver
// internal/spc/capability.go. encoding/json não sabe serializar NaN/Inf,
// então aqui os dois viram null.
type capabilityResponse struct {
	Cp    *float64 `json:"cp,omitempty"`
	Cpk   *float64 `json:"cpk,omitempty"`
	Mean  float64  `json:"mean"`
	Sigma float64  `json:"sigma"`
}

func toCapabilityResponse(c spc.CapabilityResult) capabilityResponse {
	out := capabilityResponse{Mean: c.Mean, Sigma: c.Sigma}
	if jsonSafeFloat(c.Cp) {
		cp := c.Cp
		out.Cp = &cp
	}
	if jsonSafeFloat(c.Cpk) {
		cpk := c.Cpk
		out.Cpk = &cpk
	}
	return out
}

func jsonSafeFloat(v float64) bool {
	return !math.IsNaN(v) && !math.IsInf(v, 0)
}

func (h *reportHandler) serveReport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	repo := r.URL.Query().Get("repo")
	job := r.URL.Query().Get("job")
	if repo == "" || job == "" {
		http.Error(w, "repo and job query params are required", http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	since := time.Now().Add(-reportWindow)

	resp := reportResponse{Repo: repo, Job: job}

	samples, err := h.store.RecentJobDurations(ctx, repo, job, since)
	if err != nil {
		http.Error(w, fmt.Sprintf("querying job durations: %v", err), http.StatusInternalServerError)
		return
	}

	if len(samples) < 2 {
		resp.Warnings = append(resp.Warnings, "not enough completed runs in the window to compute a control chart (need >= 2)")
	} else {
		points := make([]spc.Point, len(samples))
		values := make([]spc.ValueAt, len(samples))
		for i, s := range samples {
			points[i] = spc.Point{Timestamp: s.StartedAt, Value: s.DurationSeconds}
			values[i] = spc.ValueAt{Timestamp: s.StartedAt, Value: s.DurationSeconds}
		}

		chart, err := spc.ComputeIMR(points)
		if err != nil {
			http.Error(w, fmt.Sprintf("computing I-MR chart: %v", err), http.StatusInternalServerError)
			return
		}
		resp.Chart = &chart
		resp.NelsonViolations = spc.ApplyNelsonRules(chart)

		if h.upperSpecSeconds != nil {
			capability, err := spc.ComputeCapability(values, h.upperSpecSeconds, nil)
			if err != nil {
				resp.Warnings = append(resp.Warnings, fmt.Sprintf("computing capability: %v", err))
			} else {
				out := toCapabilityResponse(capability)
				resp.Capability = &out
			}
		}

		if h.newRelic != nil {
			if err := h.sendChartEvents(ctx, repo, job, chart, resp.NelsonViolations); err != nil {
				slog.Error("sending SPC events to New Relic", "err", err)
			}
		}
	}

	failureCounts, err := h.store.FailureCountsByJob(ctx, repo, since)
	if err != nil {
		http.Error(w, fmt.Sprintf("querying failure counts: %v", err), http.StatusInternalServerError)
		return
	}
	causes := make([]spc.FailureCause, len(failureCounts))
	for i, fc := range failureCounts {
		causes[i] = spc.FailureCause{Name: fc.JobName, Count: fc.Count}
	}
	resp.Pareto = spc.Pareto(causes)

	if h.doraDeployWorkflow != "" {
		metrics, err := h.computeDORA(ctx, repo, since)
		if err != nil {
			resp.Warnings = append(resp.Warnings, fmt.Sprintf("computing DORA metrics: %v", err))
		} else {
			resp.DORA = &metrics
		}
	} else {
		resp.Warnings = append(resp.Warnings, "DORA_DEPLOY_WORKFLOW not configured; DORA metrics skipped")
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		slog.Error("encoding report response", "err", err)
	}
}

func (h *reportHandler) computeDORA(ctx context.Context, repo string, since time.Time) (dora.Metrics, error) {
	runs, err := h.store.RunsByWorkflow(ctx, repo, h.doraDeployWorkflow, since)
	if err != nil {
		return dora.Metrics{}, fmt.Errorf("querying deploy workflow runs: %w", err)
	}

	deployments := make([]dora.Deployment, len(runs))
	for i, run := range runs {
		deployments[i] = dora.Deployment{
			DeployedAt: run.StartedAt,
			// Lead time (commit -> deploy) não é rastreado nos dados
			// coletados hoje; fica 0 até existir essa correlação.
			LeadTime: 0,
			Success:  run.Conclusion == "success",
		}
	}

	// Não há, hoje, uma fonte de incidents (isso normalmente vem de uma
	// ferramenta de gestão de incidentes, não do GitHub Actions). MTTR
	// fica 0 até essa integração existir.
	return dora.Compute(time.Now(), deployments, nil, reportWindow)
}

func (h *reportHandler) sendChartEvents(ctx context.Context, repo, job string, chart spc.IMRChart, violations []spc.NelsonViolation) error {
	violatedIndex := make(map[int]int) // index -> rule number
	for _, v := range violations {
		if existing, ok := violatedIndex[v.Index]; !ok || v.RuleNumber < existing {
			violatedIndex[v.Index] = v.RuleNumber
		}
	}

	events := make([]newrelic.Event, len(chart.Points))
	for i, p := range chart.Points {
		attrs := map[string]any{
			"repo":             repo,
			"job_name":         job,
			"value":            p.Value,
			"center_line":      chart.CenterLine,
			"ucl":              chart.UCL,
			"lcl":              chart.LCL,
			"nelson_violation": false,
		}
		if rule, ok := violatedIndex[i]; ok {
			attrs["nelson_violation"] = true
			attrs["nelson_rule"] = rule
		}

		events[i] = newrelic.Event{
			EventType:  "KaizenOpsSPC",
			Timestamp:  p.Timestamp,
			Attributes: attrs,
		}
	}

	return h.newRelic.SendEvents(ctx, events)
}
