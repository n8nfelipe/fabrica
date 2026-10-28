ALTER TABLE tasks DROP CONSTRAINT tasks_specification_status_check;
ALTER TABLE tasks ADD CONSTRAINT tasks_specification_status_check
    CHECK (specification_status IN ('not_started', 'pending', 'approved', 'changes_requested', 'failed'));

CREATE TABLE task_specifications (
    id BIGSERIAL PRIMARY KEY,
    task_id BIGINT NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    round INTEGER NOT NULL CHECK (round > 0),
    status TEXT NOT NULL CHECK (status IN ('generated', 'approved', 'changes_requested', 'failed')),
    artifact_paths JSONB NOT NULL DEFAULT '[]'::jsonb,
    summary TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (task_id, round)
);

CREATE INDEX task_specifications_task_round_idx ON task_specifications(task_id, round DESC);
