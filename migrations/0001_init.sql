CREATE EXTENSION IF NOT EXISTS timescaledb;

CREATE TABLE workflow_runs (
    repo             TEXT NOT NULL,
    workflow_name    TEXT NOT NULL,
    run_id           BIGINT NOT NULL,
    run_attempt      INT NOT NULL,
    trigger_event    TEXT NOT NULL,
    branch           TEXT,
    status           TEXT NOT NULL,
    conclusion       TEXT,
    files_changed    INT,
    file_types       TEXT[],
    author_hash      TEXT NOT NULL,
    started_at       TIMESTAMPTZ NOT NULL,
    completed_at     TIMESTAMPTZ,
    duration_seconds DOUBLE PRECISION,
    ingested_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

SELECT create_hypertable('workflow_runs', by_range('started_at'));

-- Idempotência: um webhook redelivered ou uma reconciliação via REST não
-- deve duplicar a linha. started_at entra na constraint porque o índice
-- único de uma hypertable precisa incluir a coluna de particionamento.
CREATE UNIQUE INDEX workflow_runs_run_attempt_idx
    ON workflow_runs (repo, run_id, run_attempt, started_at);

CREATE INDEX workflow_runs_repo_workflow_idx
    ON workflow_runs (repo, workflow_name, started_at DESC);

CREATE TABLE workflow_jobs (
    repo             TEXT NOT NULL,
    workflow_name    TEXT,
    run_id           BIGINT NOT NULL,
    job_id           BIGINT NOT NULL,
    job_name         TEXT NOT NULL,
    status           TEXT NOT NULL,
    conclusion       TEXT,
    author_hash      TEXT NOT NULL,
    queued_at        TIMESTAMPTZ,
    started_at       TIMESTAMPTZ NOT NULL,
    completed_at     TIMESTAMPTZ,
    duration_seconds DOUBLE PRECISION,
    queue_seconds    DOUBLE PRECISION,
    ingested_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

SELECT create_hypertable('workflow_jobs', by_range('started_at'));

CREATE UNIQUE INDEX workflow_jobs_job_idx
    ON workflow_jobs (repo, run_id, job_id, started_at);

CREATE INDEX workflow_jobs_repo_job_name_idx
    ON workflow_jobs (repo, job_name, started_at DESC);
