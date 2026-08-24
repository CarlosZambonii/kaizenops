package storage

import (
	"context"
	"testing"
	"time"

	"github.com/carlosz/kaizenops/internal/ingest"
)

func TestRecentJobDurationsAndFailureCounts(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()

	repo := "carlosz/kaizenops"
	jobName := "queries-test-job"
	base := time.Now().UnixNano()

	// Duas execuções bem-sucedidas e uma falha, todas do mesmo job.
	for i, outcome := range []string{"success", "success", "failure"} {
		startedAt := time.Now().UTC().Add(time.Duration(i) * time.Minute).Truncate(time.Second)
		completedAt := startedAt.Add(time.Minute)

		job := ingest.WorkflowJob{
			Repo:        repo,
			RunID:       base,
			JobID:       base + int64(i) + 1,
			JobName:     jobName,
			Status:      "completed",
			Conclusion:  outcome,
			AuthorHash:  "deadbeef",
			StartedAt:   startedAt,
			CompletedAt: &completedAt,
		}
		if err := store.SaveWorkflowJob(ctx, job); err != nil {
			t.Fatalf("SaveWorkflowJob() error = %v", err)
		}
	}

	since := time.Now().Add(-time.Hour)

	samples, err := store.RecentJobDurations(ctx, repo, jobName, since)
	if err != nil {
		t.Fatalf("RecentJobDurations() error = %v", err)
	}

	var found int
	for _, s := range samples {
		if s.DurationSeconds == 60 {
			found++
		}
	}
	if found < 3 {
		t.Fatalf("expected at least 3 samples with duration 60s, found %d in %+v", found, samples)
	}

	counts, err := store.FailureCountsByJob(ctx, repo, since)
	if err != nil {
		t.Fatalf("FailureCountsByJob() error = %v", err)
	}

	var gotCount int
	for _, c := range counts {
		if c.JobName == jobName {
			gotCount = c.Count
		}
	}
	if gotCount != 1 {
		t.Fatalf("FailureCountsByJob() for %q = %d, want 1 (in %+v)", jobName, gotCount, counts)
	}
}

func TestRunsByWorkflow(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()

	repo := "carlosz/kaizenops"
	workflowName := "queries-test-deploy"
	runID := time.Now().UnixNano()

	startedAt := time.Now().UTC().Truncate(time.Second)
	completedAt := startedAt.Add(2 * time.Minute)

	run := ingest.WorkflowRun{
		Repo:         repo,
		WorkflowName: workflowName,
		RunID:        runID,
		RunAttempt:   1,
		TriggerEvent: "push",
		Status:       "completed",
		Conclusion:   "success",
		AuthorHash:   "deadbeef",
		StartedAt:    startedAt,
		CompletedAt:  &completedAt,
	}
	if err := store.SaveWorkflowRun(ctx, run); err != nil {
		t.Fatalf("SaveWorkflowRun() error = %v", err)
	}

	runs, err := store.RunsByWorkflow(ctx, repo, workflowName, time.Now().Add(-time.Hour))
	if err != nil {
		t.Fatalf("RunsByWorkflow() error = %v", err)
	}

	var found bool
	for _, r := range runs {
		if r.StartedAt.Equal(startedAt) && r.Conclusion == "success" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected to find run started at %v with conclusion success, got %+v", startedAt, runs)
	}
}
