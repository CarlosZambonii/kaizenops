package storage

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/carlosz/kaizenops/internal/ingest"
)

// Store persiste eventos de pipeline no TimescaleDB. Implementa
// ingest.Sink.
type Store struct {
	pool *pgxpool.Pool
}

var _ ingest.Sink = (*Store)(nil)

// Open conecta ao TimescaleDB e confirma a conexão com um ping.
func Open(ctx context.Context, databaseURL string) (*Store, error) {
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, fmt.Errorf("opening TimescaleDB pool: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("pinging TimescaleDB: %w", err)
	}

	return &Store{pool: pool}, nil
}

// Pool expõe o pool de conexões subjacente, usado por Migrate.
func (s *Store) Pool() *pgxpool.Pool {
	return s.pool
}

// Close encerra todas as conexões do pool.
func (s *Store) Close() {
	s.pool.Close()
}

// SaveWorkflowRun insere ou atualiza (numa redelivery de webhook) um
// evento workflow_run.
func (s *Store) SaveWorkflowRun(ctx context.Context, run ingest.WorkflowRun) error {
	var duration *float64
	if run.CompletedAt != nil {
		d := run.CompletedAt.Sub(run.StartedAt).Seconds()
		duration = &d
	}

	_, err := s.pool.Exec(ctx, `
		INSERT INTO workflow_runs (
			repo, workflow_name, run_id, run_attempt, trigger_event, branch,
			status, conclusion, files_changed, file_types, author_hash,
			started_at, completed_at, duration_seconds
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)
		ON CONFLICT (repo, run_id, run_attempt, started_at) DO UPDATE SET
			status           = EXCLUDED.status,
			conclusion       = EXCLUDED.conclusion,
			completed_at     = EXCLUDED.completed_at,
			duration_seconds = EXCLUDED.duration_seconds
	`,
		run.Repo, run.WorkflowName, run.RunID, run.RunAttempt, run.TriggerEvent, run.Branch,
		run.Status, run.Conclusion, run.FilesChanged, run.FileTypes, run.AuthorHash,
		run.StartedAt, run.CompletedAt, duration,
	)
	if err != nil {
		return fmt.Errorf("saving workflow run: %w", err)
	}

	return nil
}

// SaveWorkflowJob insere ou atualiza (numa redelivery de webhook) um
// evento workflow_job.
func (s *Store) SaveWorkflowJob(ctx context.Context, job ingest.WorkflowJob) error {
	var duration *float64
	if job.CompletedAt != nil {
		d := job.CompletedAt.Sub(job.StartedAt).Seconds()
		duration = &d
	}

	var queueSeconds *float64
	if job.QueuedAt != nil {
		d := job.StartedAt.Sub(*job.QueuedAt).Seconds()
		queueSeconds = &d
	}

	_, err := s.pool.Exec(ctx, `
		INSERT INTO workflow_jobs (
			repo, workflow_name, run_id, job_id, job_name, status, conclusion,
			author_hash, queued_at, started_at, completed_at, duration_seconds,
			queue_seconds
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
		ON CONFLICT (repo, run_id, job_id, started_at) DO UPDATE SET
			status           = EXCLUDED.status,
			conclusion       = EXCLUDED.conclusion,
			completed_at     = EXCLUDED.completed_at,
			duration_seconds = EXCLUDED.duration_seconds,
			queue_seconds    = EXCLUDED.queue_seconds
	`,
		job.Repo, job.WorkflowName, job.RunID, job.JobID, job.JobName, job.Status, job.Conclusion,
		job.AuthorHash, job.QueuedAt, job.StartedAt, job.CompletedAt, duration, queueSeconds,
	)
	if err != nil {
		return fmt.Errorf("saving workflow job: %w", err)
	}

	return nil
}
