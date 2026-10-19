CREATE TABLE sandbox_results (
    id           bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    tenant_id    bigint NOT NULL CHECK (tenant_id > 0),
    task_run_id  uuid NOT NULL REFERENCES task_runs (id) ON DELETE CASCADE,
    attempt      integer NOT NULL CHECK (attempt > 0),
    compiled     boolean NOT NULL,
    tests_passed integer NOT NULL CHECK (tests_passed >= 0),
    tests_failed integer NOT NULL CHECK (tests_failed >= 0),
    raw_output   text NOT NULL,
    duration_ms  bigint NOT NULL CHECK (duration_ms >= 0),
    failure_type text NOT NULL CHECK (failure_type IN ('nenhuma', 'teste', 'compilacao', 'infraestrutura')),
    created_at   timestamptz NOT NULL DEFAULT now(),
    UNIQUE (task_run_id, attempt)
);

CREATE INDEX sandbox_results_tenant_id ON sandbox_results (tenant_id);
