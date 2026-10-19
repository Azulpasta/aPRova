CREATE TABLE risk_gates (
    id                  bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    tenant_id           bigint NOT NULL CHECK (tenant_id > 0),
    task_run_id         uuid NOT NULL UNIQUE REFERENCES task_runs (id) ON DELETE RESTRICT,
    deterministic_level text NOT NULL CHECK (deterministic_level IN ('baixo', 'medio', 'alto')),
    level               text NOT NULL CHECK (level IN ('baixo', 'medio', 'alto')),
    reason              text NOT NULL CHECK (reason <> ''),
    created_at          timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX risk_gates_tenant_id ON risk_gates (tenant_id);
