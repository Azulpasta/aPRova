CREATE TABLE approvals (
    id              bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    tenant_id       bigint NOT NULL CHECK (tenant_id > 0),
    task_run_id     uuid NOT NULL REFERENCES task_runs (id) ON DELETE RESTRICT,
    delivery_id     text NOT NULL UNIQUE,
    decision        text NOT NULL CHECK (decision IN ('aprovada', 'alteracoes_solicitadas')),
    reviewer        text NOT NULL,
    feedback_reason text CHECK (feedback_reason IN ('regra_conservadora', 'risco_aceito', 'falso_positivo')),
    justification   text,
    decided_at      timestamptz NOT NULL,
    created_at      timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX approvals_tenant_id ON approvals (tenant_id);
CREATE INDEX approvals_por_task_run ON approvals (task_run_id);
