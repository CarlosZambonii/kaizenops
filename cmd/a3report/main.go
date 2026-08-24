// Command a3report gera um relatório mensal de melhoria contínua em
// formato A3 (Markdown), a partir dos dados de SPC/DORA já coletados no
// TimescaleDB.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/carlosz/kaizenops/internal/dora"
	"github.com/carlosz/kaizenops/internal/spc"
	"github.com/carlosz/kaizenops/internal/storage"
)

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "a3report: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string, out io.Writer) error {
	fs := flag.NewFlagSet("a3report", flag.ContinueOnError)
	repo := fs.String("repo", "", "repositório (owner/name), obrigatório")
	job := fs.String("job", "", "nome do job cuja duração entra na carta I-MR, obrigatório")
	workflow := fs.String("workflow", "", "nome do workflow usado como proxy de deployment para as DORA metrics (opcional)")
	upperSpecSeconds := fs.String("upper-spec-seconds", "", "limite superior de especificação (segundos) para Cp/Cpk (opcional)")
	windowDays := fs.Int("window-days", 30, "janela, em dias, considerada no relatório")
	outPath := fs.String("out", "", "arquivo de saída (default: stdout)")

	if err := fs.Parse(args); err != nil {
		return err
	}

	if *repo == "" || *job == "" {
		return fmt.Errorf("--repo e --job são obrigatórios")
	}

	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		return fmt.Errorf("variável de ambiente DATABASE_URL não configurada")
	}

	var upperSpec *float64
	if *upperSpecSeconds != "" {
		v, err := strconv.ParseFloat(*upperSpecSeconds, 64)
		if err != nil {
			return fmt.Errorf("parsing --upper-spec-seconds: %w", err)
		}
		upperSpec = &v
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	store, err := storage.Open(ctx, databaseURL)
	if err != nil {
		return fmt.Errorf("opening storage: %w", err)
	}
	defer store.Close()

	window := time.Duration(*windowDays) * 24 * time.Hour
	report, err := buildReport(ctx, store, reportParams{
		repo:             *repo,
		job:              *job,
		workflow:         *workflow,
		upperSpecSeconds: upperSpec,
		window:           window,
	})
	if err != nil {
		return err
	}

	if *outPath == "" {
		_, err = out.Write([]byte(report))
		return err
	}
	return os.WriteFile(*outPath, []byte(report), 0o644)
}

type reportParams struct {
	repo             string
	job              string
	workflow         string
	upperSpecSeconds *float64
	window           time.Duration
}

func buildReport(ctx context.Context, store *storage.Store, p reportParams) (string, error) {
	now := time.Now()
	since := now.Add(-p.window)

	samples, err := store.RecentJobDurations(ctx, p.repo, p.job, since)
	if err != nil {
		return "", fmt.Errorf("querying job durations: %w", err)
	}

	failureCounts, err := store.FailureCountsByJob(ctx, p.repo, since)
	if err != nil {
		return "", fmt.Errorf("querying failure counts: %w", err)
	}
	causes := make([]spc.FailureCause, len(failureCounts))
	for i, fc := range failureCounts {
		causes[i] = spc.FailureCause{Name: fc.JobName, Count: fc.Count}
	}
	pareto := spc.Pareto(causes)

	var chart *spc.IMRChart
	var violations []spc.NelsonViolation
	var capability *spc.CapabilityResult
	if len(samples) >= 2 {
		points := make([]spc.Point, len(samples))
		values := make([]spc.ValueAt, len(samples))
		for i, s := range samples {
			points[i] = spc.Point{Timestamp: s.StartedAt, Value: s.DurationSeconds}
			values[i] = spc.ValueAt{Timestamp: s.StartedAt, Value: s.DurationSeconds}
		}

		c, err := spc.ComputeIMR(points)
		if err != nil {
			return "", fmt.Errorf("computing I-MR chart: %w", err)
		}
		chart = &c
		violations = spc.ApplyNelsonRules(c)

		if p.upperSpecSeconds != nil {
			cap, err := spc.ComputeCapability(values, p.upperSpecSeconds, nil)
			if err == nil {
				capability = &cap
			}
		}
	}

	var metrics *dora.Metrics
	if p.workflow != "" {
		runs, err := store.RunsByWorkflow(ctx, p.repo, p.workflow, since)
		if err == nil {
			deployments := make([]dora.Deployment, len(runs))
			for i, r := range runs {
				deployments[i] = dora.Deployment{
					DeployedAt: r.StartedAt,
					Success:    r.Conclusion == "success",
				}
			}
			if m, err := dora.Compute(now, deployments, nil, p.window); err == nil {
				metrics = &m
			}
		}
	}

	return renderMarkdown(p, now, chart, violations, capability, pareto, metrics), nil
}

func renderMarkdown(
	p reportParams,
	generatedAt time.Time,
	chart *spc.IMRChart,
	violations []spc.NelsonViolation,
	capability *spc.CapabilityResult,
	pareto []spc.ParetoEntry,
	metrics *dora.Metrics,
) string {
	var b strings.Builder

	fmt.Fprintf(&b, "# Relatório A3 — %s / %s\n\n", p.repo, p.job)
	fmt.Fprintf(&b, "Gerado em %s. Janela analisada: últimos %d dias.\n\n", generatedAt.Format("2006-01-02 15:04"), int(p.window.Hours()/24))

	b.WriteString("## Background\n\n")
	b.WriteString("Este relatório aplica Statistical Process Control sobre a duração do job monitorado, ")
	b.WriteString("seguindo o princípio do KaizenOps: melhorar o processo de entrega, nunca vigiar pessoas — ")
	b.WriteString("toda a análise abaixo é sobre o job, nunca sobre quem o executou.\n\n")

	b.WriteString("## Current Condition (Measure)\n\n")
	if chart == nil {
		b.WriteString("Não há amostras suficientes na janela para montar a carta de controle (mínimo 2 execuções concluídas).\n\n")
	} else {
		fmt.Fprintf(&b, "- Amostras na janela: %d\n", len(chart.Points))
		fmt.Fprintf(&b, "- Linha central (média): %.2fs\n", chart.CenterLine)
		fmt.Fprintf(&b, "- Limites de controle (I-MR): LCL=%.2fs, UCL=%.2fs\n", chart.LCL, chart.UCL)
		fmt.Fprintf(&b, "- Violações de regra de Nelson detectadas: %d\n\n", len(violations))

		if capability != nil {
			if !math.IsNaN(capability.Cp) && !math.IsInf(capability.Cp, 0) {
				fmt.Fprintf(&b, "- Cp: %.3f\n", capability.Cp)
			}
			if !math.IsNaN(capability.Cpk) && !math.IsInf(capability.Cpk, 0) {
				fmt.Fprintf(&b, "- Cpk: %.3f\n", capability.Cpk)
			}
			b.WriteString("\n")
		}
	}

	if metrics != nil {
		b.WriteString("### DORA metrics\n\n")
		fmt.Fprintf(&b, "- Deployment frequency: %.2f/dia (workflow %q como proxy de deployment)\n", metrics.DeploymentFrequencyPerDay, p.workflow)
		fmt.Fprintf(&b, "- Lead time (mediana): %s (não rastreado hoje — requer correlação commit→deploy)\n", metrics.MedianLeadTime)
		fmt.Fprintf(&b, "- Change failure rate: %.1f%%\n", metrics.ChangeFailureRate*100)
		fmt.Fprintf(&b, "- MTTR: %s (não rastreado hoje — requer fonte de incidents)\n\n", metrics.MTTR)
	}

	b.WriteString("## Analyze — Pareto de causas de falha\n\n")
	if len(pareto) == 0 {
		b.WriteString("Nenhuma falha registrada na janela.\n\n")
	} else {
		b.WriteString("| Job/Teste | Ocorrências | % acumulado |\n")
		b.WriteString("|---|---|---|\n")
		for _, entry := range pareto {
			fmt.Fprintf(&b, "| %s | %d | %.1f%% |\n", entry.Name, entry.Count, entry.CumulativePercent)
		}
		b.WriteString("\n")
	}

	b.WriteString("## Violações de regra de Nelson (causa especial)\n\n")
	if len(violations) == 0 {
		b.WriteString("Nenhuma violação no período — o processo está estatisticamente estável (só variação de causa comum).\n\n")
	} else {
		b.WriteString("| Regra | Ponto | Descrição |\n")
		b.WriteString("|---|---|---|\n")
		for _, v := range violations {
			fmt.Fprintf(&b, "| %d | %d | %s |\n", v.RuleNumber, v.Index, v.Description)
		}
		b.WriteString("\n")
	}

	b.WriteString("## Root Cause Analysis\n\n")
	b.WriteString("_A completar: cruzar as violações de Nelson e o Pareto acima com o contexto de cada incidente. ")
	b.WriteString("Este relatório aponta ONDE olhar; o porquê exige investigação humana._\n\n")

	b.WriteString("## Countermeasures / Plan\n\n")
	b.WriteString("_A completar: ações propostas para as causas raiz identificadas, com responsável e prazo._\n\n")

	b.WriteString("## Follow-up\n\n")
	b.WriteString("_A completar no próximo ciclo: as contramedidas reduziram a variação de causa especial e o Cpk melhorou?_\n")

	return b.String()
}
