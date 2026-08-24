// Package ingest processa eventos de pipeline recebidos do GitHub e os
// entrega para persistência através de um worker pool.
package ingest

import (
	"context"
	"time"
)

// WorkflowRun é o dado extraído de um evento workflow_run.
type WorkflowRun struct {
	Repo         string
	WorkflowName string
	RunID        int64
	RunAttempt   int
	TriggerEvent string
	Branch       string
	Status       string
	Conclusion   string
	FilesChanged int
	FileTypes    []string
	AuthorHash   string
	StartedAt    time.Time
	CompletedAt  *time.Time
}

// WorkflowJob é o dado extraído de um evento workflow_job.
type WorkflowJob struct {
	Repo         string
	WorkflowName string
	RunID        int64
	JobID        int64
	JobName      string
	Status       string
	Conclusion   string
	AuthorHash   string
	QueuedAt     *time.Time
	StartedAt    time.Time
	CompletedAt  *time.Time
}

// Sink persiste os eventos de pipeline já processados. Definida aqui, no
// pacote consumidor, e implementada por internal/storage.
type Sink interface {
	SaveWorkflowRun(ctx context.Context, run WorkflowRun) error
	SaveWorkflowJob(ctx context.Context, job WorkflowJob) error
}
