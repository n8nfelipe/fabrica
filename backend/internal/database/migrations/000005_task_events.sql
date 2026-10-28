CREATE TABLE task_events (
    id BIGSERIAL PRIMARY KEY,
    task_id BIGINT NOT NULL REFERENCES tasks(id) ON DELETE RESTRICT,
    actor_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
    actor_role TEXT NOT NULL,
    event_type TEXT NOT NULL CHECK (event_type IN ('transition', 'approval', 'comment', 'failure', 'attempt')),
    action TEXT NOT NULL,
    from_state TEXT NOT NULL,
    to_state TEXT NOT NULL,
    comment TEXT NOT NULL DEFAULT '',
    reason TEXT NOT NULL DEFAULT '',
    occurred_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX task_events_task_id_id_idx ON task_events (task_id, id);

CREATE FUNCTION reject_task_event_mutation() RETURNS trigger AS $$
BEGIN
    RAISE EXCEPTION 'task events are append-only';
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER task_events_append_only
BEFORE UPDATE OR DELETE ON task_events
FOR EACH ROW EXECUTE FUNCTION reject_task_event_mutation();
