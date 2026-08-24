package github

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/carlosz/kaizenops/internal/ingest"
)

type fakeSink struct {
	runs []ingest.WorkflowRun
	jobs []ingest.WorkflowJob
}

func (f *fakeSink) SaveWorkflowRun(ctx context.Context, run ingest.WorkflowRun) error {
	f.runs = append(f.runs, run)
	return nil
}

func (f *fakeSink) SaveWorkflowJob(ctx context.Context, job ingest.WorkflowJob) error {
	f.jobs = append(f.jobs, job)
	return nil
}

func newTestHandler(sink *fakeSink) *Handler {
	return &Handler{
		Secret: "s3cr3t",
		Salt:   "salt",
		Sink:   sink,
		Enqueue: func(r *http.Request, task ingest.Task) error {
			return task(r.Context())
		},
	}
}

func postWebhook(t *testing.T, h *Handler, event, body string) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(http.MethodPost, "/webhooks/github", strings.NewReader(body))
	req.Header.Set("X-GitHub-Event", event)
	req.Header.Set("X-Hub-Signature-256", sign(t, h.Secret, []byte(body)))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

const workflowRunBody = `{
	"action": "completed",
	"workflow_run": {
		"id": 42,
		"run_attempt": 1,
		"name": "CI",
		"event": "push",
		"status": "completed",
		"conclusion": "success",
		"head_branch": "main",
		"run_started_at": "2026-08-24T10:00:00Z",
		"updated_at": "2026-08-24T10:05:00Z"
	},
	"repository": { "full_name": "carlosz/kaizenops" },
	"sender": { "login": "octocat" }
}`

const workflowJobBody = `{
	"action": "completed",
	"workflow_job": {
		"id": 99,
		"run_id": 42,
		"workflow_name": "CI",
		"name": "test",
		"status": "completed",
		"conclusion": "success",
		"created_at": "2026-08-24T10:00:00Z",
		"started_at": "2026-08-24T10:01:00Z",
		"completed_at": "2026-08-24T10:04:00Z"
	},
	"repository": { "full_name": "carlosz/kaizenops" },
	"sender": { "login": "octocat" }
}`

func TestHandlerWorkflowRun(t *testing.T) {
	sink := &fakeSink{}
	h := newTestHandler(sink)

	rec := postWebhook(t, h, "workflow_run", workflowRunBody)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusAccepted, rec.Body.String())
	}
	if len(sink.runs) != 1 {
		t.Fatalf("sink.runs = %d, want 1", len(sink.runs))
	}

	got := sink.runs[0]
	if got.Repo != "carlosz/kaizenops" || got.RunID != 42 || got.Branch != "main" {
		t.Fatalf("unexpected run: %+v", got)
	}
	if got.AuthorHash == "" || got.AuthorHash == "octocat" {
		t.Fatalf("AuthorHash not pseudonymized: %q", got.AuthorHash)
	}
	if got.CompletedAt == nil {
		t.Fatal("CompletedAt should be set for a completed run")
	}
}

func TestHandlerWorkflowJob(t *testing.T) {
	sink := &fakeSink{}
	h := newTestHandler(sink)

	rec := postWebhook(t, h, "workflow_job", workflowJobBody)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusAccepted, rec.Body.String())
	}
	if len(sink.jobs) != 1 {
		t.Fatalf("sink.jobs = %d, want 1", len(sink.jobs))
	}

	got := sink.jobs[0]
	if got.RunID != 42 || got.JobID != 99 || got.JobName != "test" {
		t.Fatalf("unexpected job: %+v", got)
	}
	if got.QueuedAt == nil {
		t.Fatal("QueuedAt should be set")
	}
}

func TestHandlerIgnoresUnknownEvent(t *testing.T) {
	sink := &fakeSink{}
	h := newTestHandler(sink)

	rec := postWebhook(t, h, "ping", `{}`)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusAccepted)
	}
	if len(sink.runs) != 0 || len(sink.jobs) != 0 {
		t.Fatal("unknown event should not reach the sink")
	}
}

func TestHandlerRejectsInvalidSignature(t *testing.T) {
	sink := &fakeSink{}
	h := newTestHandler(sink)

	req := httptest.NewRequest(http.MethodPost, "/webhooks/github", strings.NewReader(workflowRunBody))
	req.Header.Set("X-GitHub-Event", "workflow_run")
	req.Header.Set("X-Hub-Signature-256", "sha256=0000000000000000000000000000000000000000000000000000000000000000")

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
	if len(sink.runs) != 0 {
		t.Fatal("invalid signature should not reach the sink")
	}
}

func TestHandlerRejectsMalformedPayload(t *testing.T) {
	sink := &fakeSink{}
	h := newTestHandler(sink)

	rec := postWebhook(t, h, "workflow_run", `{not-json`)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}
