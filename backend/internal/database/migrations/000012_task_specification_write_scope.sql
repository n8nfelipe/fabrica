ALTER TABLE task_specifications
    ADD COLUMN write_paths JSONB NOT NULL DEFAULT '[]'::jsonb;
