package storage

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/carlosz/kaizenops/internal/ingest"
)

// Testes de integração: precisam de um TimescaleDB real em TEST_DATABASE_URL
// (ex: o docker-compose.yml da raiz do repo, com as migrations aplicadas).
func testStore(t *testing.T) *Store {
	t.Helper()

	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set, skipping storage integration test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	store, err := Open(ctx, url)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}

	if err := Migrate(ctx, store.pool, "../../migrations"); err != nil {
		store.Close()
		t.Fatalf("Migrate() error = %v", err)
	}

	t.Cleanup(store.Close)
	return store
}

func TestStoreSaveWorkflowRun(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()

	completedAt := time.Now().UTC().Truncate(time.Second)
	startedAt := completedAt.Add(-5 * time.Minute)

	run := ingest.WorkflowRun{
		Repo:         "carlosz/kaizenops",
		WorkflowName: "CI",
		RunID:        time.Now().UnixNano(), // único por execução do teste
		RunAttempt:   1,
		TriggerEvent: "push",
		Branch:       "main",
		Status:       "completed",
		Conclusion:   "success",
		AuthorHash:   "deadbeef",
		StartedAt:    startedAt,
		CompletedAt:  &completedAt,
	}

	if err := store.SaveWorkflowRun(ctx, run); err != nil {
		t.Fatalf("SaveWorkflowRun() error = %v", err)
	}

	// Redelivery do mesmo webhook: não deve falhar por violação de
	// unicidade, deve fazer upsert.
	if err := store.SaveWorkflowRun(ctx, run); err != nil {
		t.Fatalf("SaveWorkflowRun() (redelivery) error = %v", err)
	}

	var count int
	err := store.pool.QueryRow(ctx,
		`SELECT count(*) FROM workflow_runs WHERE run_id = $1`, run.RunID,
	).Scan(&count)
	if err != nil {
		t.Fatalf("querying workflow_runs: %v", err)
	}
	if count != 1 {
		t.Fatalf("count = %d, want 1 (upsert should not duplicate)", count)
	}

	var duration float64
	err = store.pool.QueryRow(ctx,
		`SELECT duration_seconds FROM workflow_runs WHERE run_id = $1`, run.RunID,
	).Scan(&duration)
	if err != nil {
		t.Fatalf("querying duration_seconds: %v", err)
	}
	if duration != 300 {
		t.Fatalf("duration_seconds = %v, want 300", duration)
	}
}

func TestStoreSaveWorkflowJob(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()

	queuedAt := time.Now().UTC().Truncate(time.Second)
	startedAt := queuedAt.Add(30 * time.Second)
	completedAt := startedAt.Add(2 * time.Minute)

	job := ingest.WorkflowJob{
		Repo:         "carlosz/kaizenops",
		WorkflowName: "CI",
		RunID:        time.Now().UnixNano(),
		JobID:        time.Now().UnixNano() + 1,
		JobName:      "test",
		Status:       "completed",
		Conclusion:   "success",
		AuthorHash:   "deadbeef",
		QueuedAt:     &queuedAt,
		StartedAt:    startedAt,
		CompletedAt:  &completedAt,
	}

	if err := store.SaveWorkflowJob(ctx, job); err != nil {
		t.Fatalf("SaveWorkflowJob() error = %v", err)
	}

	var duration, queueSeconds float64
	err := store.pool.QueryRow(ctx,
		`SELECT duration_seconds, queue_seconds FROM workflow_jobs WHERE job_id = $1`, job.JobID,
	).Scan(&duration, &queueSeconds)
	if err != nil {
		t.Fatalf("querying workflow_jobs: %v", err)
	}
	if duration != 120 {
		t.Fatalf("duration_seconds = %v, want 120", duration)
	}
	if queueSeconds != 30 {
		t.Fatalf("queue_seconds = %v, want 30", queueSeconds)
	}
}
