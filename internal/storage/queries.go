package storage

import (
	"context"
	"fmt"
	"time"
)

// JobDurationSample é uma observação de duração de job, em ordem temporal —
// a entrada bruta para a carta I-MR e para capacidade de processo.
type JobDurationSample struct {
	StartedAt       time.Time
	DurationSeconds float64
}

// RecentJobDurations retorna as durações de um job (repo+jobName) desde
// "since", em ordem crescente de started_at. Só considera jobs já
// concluídos (duration_seconds não nulo).
func (s *Store) RecentJobDurations(ctx context.Context, repo, jobName string, since time.Time) ([]JobDurationSample, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT started_at, duration_seconds
		FROM workflow_jobs
		WHERE repo = $1 AND job_name = $2 AND started_at >= $3 AND duration_seconds IS NOT NULL
		ORDER BY started_at ASC
	`, repo, jobName, since)
	if err != nil {
		return nil, fmt.Errorf("querying recent job durations: %w", err)
	}
	defer rows.Close()

	var samples []JobDurationSample
	for rows.Next() {
		var sample JobDurationSample
		if err := rows.Scan(&sample.StartedAt, &sample.DurationSeconds); err != nil {
			return nil, fmt.Errorf("scanning job duration row: %w", err)
		}
		samples = append(samples, sample)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading job duration rows: %w", err)
	}

	return samples, nil
}

// FailureCount é a contagem de falhas de um job/teste (nunca de uma
// pessoa) num período.
type FailureCount struct {
	JobName string
	Count   int
}

// FailureCountsByJob conta, por job_name, quantas execuções concluíram com
// conclusion = 'failure' no repo desde "since". Base para o Pareto de
// causas de falha.
func (s *Store) FailureCountsByJob(ctx context.Context, repo string, since time.Time) ([]FailureCount, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT job_name, count(*)
		FROM workflow_jobs
		WHERE repo = $1 AND started_at >= $2 AND conclusion = 'failure'
		GROUP BY job_name
	`, repo, since)
	if err != nil {
		return nil, fmt.Errorf("querying failure counts: %w", err)
	}
	defer rows.Close()

	var counts []FailureCount
	for rows.Next() {
		var fc FailureCount
		if err := rows.Scan(&fc.JobName, &fc.Count); err != nil {
			return nil, fmt.Errorf("scanning failure count row: %w", err)
		}
		counts = append(counts, fc)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading failure count rows: %w", err)
	}

	return counts, nil
}

// WorkflowRunSummary é uma execução de workflow_run usada como proxy de
// deployment para as DORA metrics (ver cmd/spc: não há, hoje, um conceito
// explícito de "deployment" nos dados coletados — usamos o nome do
// workflow como heurística configurável).
type WorkflowRunSummary struct {
	StartedAt  time.Time
	Conclusion string
}

// RunsByWorkflow retorna as execuções (started_at, conclusion) de um
// workflow específico desde "since", em ordem crescente.
func (s *Store) RunsByWorkflow(ctx context.Context, repo, workflowName string, since time.Time) ([]WorkflowRunSummary, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT started_at, coalesce(conclusion, '')
		FROM workflow_runs
		WHERE repo = $1 AND workflow_name = $2 AND started_at >= $3 AND status = 'completed'
		ORDER BY started_at ASC
	`, repo, workflowName, since)
	if err != nil {
		return nil, fmt.Errorf("querying runs by workflow: %w", err)
	}
	defer rows.Close()

	var runs []WorkflowRunSummary
	for rows.Next() {
		var run WorkflowRunSummary
		if err := rows.Scan(&run.StartedAt, &run.Conclusion); err != nil {
			return nil, fmt.Errorf("scanning workflow run row: %w", err)
		}
		runs = append(runs, run)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading workflow run rows: %w", err)
	}

	return runs, nil
}
