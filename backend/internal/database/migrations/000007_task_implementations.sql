CREATE TABLE task_implementations (
    id BIGSERIAL PRIMARY KEY,
    task_id BIGINT NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    attempt INTEGER NOT NULL CHECK (attempt > 0),
    status TEXT NOT NULL CHECK (status IN ('succeeded', 'failed')),
    changed_files JSONB NOT NULL DEFAULT '[]'::jsonb,
    summary TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (task_id, attempt)
);

CREATE INDEX task_implementations_task_attempt_idx ON task_implementations(task_id, attempt DESC);
