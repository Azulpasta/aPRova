CREATE TABLE task_runs (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id    bigint NOT NULL CHECK (tenant_id > 0),
    delivery_id  text NOT NULL UNIQUE,
    repository   text NOT NULL,
    pr_number    integer NOT NULL CHECK (pr_number > 0),
    head_sha     text NOT NULL,
    author       text NOT NULL,
    event_action text NOT NULL,
    state        text NOT NULL DEFAULT 'pendente' CHECK (state IN (
        'pendente', 'mapeando_contexto', 'gerando', 'testando_sandbox', 'avaliando_risco',
        'aguardando_aprovacao', 'aplicando', 'concluido', 'rejeitado', 'falhou'
    )),
    received_at  timestamptz NOT NULL,
    created_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX task_runs_tenant_id ON task_runs (tenant_id);
CREATE INDEX task_runs_fila_por_estado ON task_runs (state, created_at);
CREATE INDEX task_runs_por_pull_request ON task_runs (tenant_id, repository, pr_number, created_at);
