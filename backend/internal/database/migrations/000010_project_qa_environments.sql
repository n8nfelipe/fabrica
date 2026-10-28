CREATE TABLE project_qa_environments (
    project_id BIGINT PRIMARY KEY REFERENCES projects(id) ON DELETE CASCADE,
    task_id BIGINT NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    status TEXT NOT NULL CHECK (status IN ('starting', 'healthy', 'unhealthy', 'stopped')),
    url TEXT,
    port INTEGER CHECK (port BETWEEN 1 AND 65535),
    health_checked_at TIMESTAMPTZ,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
