package github

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/carlosz/kaizenops/internal/ingest"
)

// maxPayloadBytes limita o corpo aceito de um webhook. Eventos do GitHub
// Actions não passam disso; qualquer coisa maior é sinal de payload
// malformado ou abusivo.
const maxPayloadBytes = 5 << 20 // 5 MiB

// Handler recebe webhooks workflow_run e workflow_job, valida a assinatura
// HMAC, pseudonimiza o autor e enfileira o evento no worker pool de
// ingestão.
type Handler struct {
	Secret string
	Salt   string
	Sink   ingest.Sink
	// Enqueue normalmente é ingest.Pool.Submit. Existe como campo para
	// facilitar testes com execução síncrona.
	Enqueue func(r *http.Request, task ingest.Task) error
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, maxPayloadBytes+1))
	if err != nil {
		http.Error(w, "reading body", http.StatusBadRequest)
		return
	}
	if len(body) > maxPayloadBytes {
		http.Error(w, "payload too large", http.StatusRequestEntityTooLarge)
		return
	}

	if err := ValidateSignature(h.Secret, body, r.Header.Get("X-Hub-Signature-256")); err != nil {
		http.Error(w, "invalid signature", http.StatusUnauthorized)
		return
	}

	var task ingest.Task
	switch r.Header.Get("X-GitHub-Event") {
	case "workflow_run":
		task, err = h.workflowRunTask(body)
	case "workflow_job":
		task, err = h.workflowJobTask(body)
	default:
		// Evento que não nos interessa (ex: installation, ping). Aceito e
		// ignorado, não é erro.
		w.WriteHeader(http.StatusAccepted)
		return
	}

	if err != nil {
		http.Error(w, fmt.Sprintf("parsing payload: %v", err), http.StatusBadRequest)
		return
	}

	if err := h.Enqueue(r, task); err != nil {
		http.Error(w, "enqueueing event", http.StatusServiceUnavailable)
		return
	}

	w.WriteHeader(http.StatusAccepted)
}

func (h *Handler) workflowRunTask(body []byte) (ingest.Task, error) {
	var payload workflowRunPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("decoding workflow_run payload: %w", err)
	}

	run := ingest.WorkflowRun{
		Repo:         payload.Repository.FullName,
		WorkflowName: payload.WorkflowRun.Name,
		RunID:        payload.WorkflowRun.ID,
		RunAttempt:   payload.WorkflowRun.RunAttempt,
		TriggerEvent: payload.WorkflowRun.Event,
		Branch:       payload.WorkflowRun.HeadBranch,
		Status:       payload.WorkflowRun.Status,
		Conclusion:   payload.WorkflowRun.Conclusion,
		AuthorHash:   ingest.PseudonymizeAuthor(h.Salt, payload.Sender.Login),
		StartedAt:    payload.WorkflowRun.RunStartedAt,
	}
	if run.Status == "completed" {
		completedAt := payload.WorkflowRun.UpdatedAt
		run.CompletedAt = &completedAt
	}

	return func(ctx context.Context) error {
		return h.Sink.SaveWorkflowRun(ctx, run)
	}, nil
}

func (h *Handler) workflowJobTask(body []byte) (ingest.Task, error) {
	var payload workflowJobPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("decoding workflow_job payload: %w", err)
	}

	job := ingest.WorkflowJob{
		Repo:         payload.Repository.FullName,
		WorkflowName: payload.WorkflowJob.WorkflowName,
		RunID:        payload.WorkflowJob.RunID,
		JobID:        payload.WorkflowJob.ID,
		JobName:      payload.WorkflowJob.Name,
		Status:       payload.WorkflowJob.Status,
		Conclusion:   payload.WorkflowJob.Conclusion,
		AuthorHash:   ingest.PseudonymizeAuthor(h.Salt, payload.Sender.Login),
		StartedAt:    payload.WorkflowJob.StartedAt,
		CompletedAt:  payload.WorkflowJob.CompletedAt,
	}
	if !payload.WorkflowJob.CreatedAt.IsZero() {
		queuedAt := payload.WorkflowJob.CreatedAt
		job.QueuedAt = &queuedAt
	}

	return func(ctx context.Context) error {
		return h.Sink.SaveWorkflowJob(ctx, job)
	}, nil
}
