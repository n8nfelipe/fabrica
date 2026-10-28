ALTER TABLE tasks
    ADD COLUMN specification_status TEXT NOT NULL DEFAULT 'not_started'
        CHECK (specification_status IN ('not_started', 'pending', 'approved', 'changes_requested')),
    ADD COLUMN implementation_status TEXT NOT NULL DEFAULT 'not_started'
        CHECK (implementation_status IN ('not_started', 'running', 'succeeded', 'failed')),
    ADD COLUMN qa_status TEXT NOT NULL DEFAULT 'not_started'
        CHECK (qa_status IN ('not_started', 'pending', 'approved', 'rejected')),
    ADD COLUMN pr_status TEXT NOT NULL DEFAULT 'not_created'
        CHECK (pr_status IN ('not_created', 'open', 'merged')),
    ADD COLUMN checks_status TEXT NOT NULL DEFAULT 'unknown'
        CHECK (checks_status IN ('unknown', 'pending', 'passed', 'failed')),
    ADD COLUMN blocked_from_state TEXT NOT NULL DEFAULT '';

ALTER TABLE tasks
    ADD CONSTRAINT tasks_state_check CHECK (state IN (
        'Aberta',
        'Pronta',
        'Especificando',
        'Aguardando aprovação das especificações',
        'Ajustes nas especificações',
        'Em desenvolvimento',
        'Aguardando QA',
        'Em QA',
        'Correção necessária',
        'Aguardando PR',
        'PR aberto',
        'Pronta para merge',
        'Bloqueada',
        'Concluída'
    ));
