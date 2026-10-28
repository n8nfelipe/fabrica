CREATE TABLE qa_port_leases (
    id BIGSERIAL PRIMARY KEY,
    project_id BIGINT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    host TEXT NOT NULL DEFAULT '127.0.0.1' CHECK (host = '127.0.0.1'),
    port INTEGER NOT NULL CHECK (port BETWEEN 1 AND 65535),
    status TEXT NOT NULL CHECK (status IN ('active', 'released')),
    leased_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    released_at TIMESTAMPTZ
);

CREATE UNIQUE INDEX qa_port_leases_active_port_idx ON qa_port_leases(host, port) WHERE status='active';
CREATE UNIQUE INDEX qa_port_leases_active_project_idx ON qa_port_leases(project_id) WHERE status='active';
