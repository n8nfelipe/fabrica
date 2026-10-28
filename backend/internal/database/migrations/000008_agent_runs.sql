CREATE TABLE agent_runs (
    id BIGSERIAL PRIMARY KEY,
    task_id BIGINT NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    phase TEXT NOT NULL CHECK (phase IN ('specification', 'implementation')),
    attempt INTEGER NOT NULL CHECK (attempt > 0),
    status TEXT NOT NULL CHECK (status IN ('running', 'succeeded', 'failed', 'cancelled')),
    process_pid BIGINT NOT NULL DEFAULT 0,
    process_start_token TEXT NOT NULL DEFAULT '',
    summary TEXT NOT NULL DEFAULT '',
    stdout_redacted TEXT NOT NULL DEFAULT '',
    stderr_redacted TEXT NOT NULL DEFAULT '',
    result_refs JSONB NOT NULL DEFAULT '[]'::jsonb,
    started_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    finished_at TIMESTAMPTZ,
    UNIQUE (task_id, phase, attempt)
);

CREATE INDEX agent_runs_reconcile_idx ON agent_runs(status, task_id) WHERE status='running';
ALTER TABLE task_specifications ADD COLUMN run_id BIGINT REFERENCES agent_runs(id);
ALTER TABLE task_implementations ADD COLUMN run_id BIGINT REFERENCES agent_runs(id);
