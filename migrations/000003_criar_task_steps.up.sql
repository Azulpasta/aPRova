CREATE TABLE task_steps (
    id          bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    tenant_id   bigint NOT NULL CHECK (tenant_id > 0),
    task_run_id uuid NOT NULL REFERENCES task_runs (id) ON DELETE CASCADE,
    from_state  text NOT NULL CHECK (from_state IN (
        'pendente', 'mapeando_contexto', 'gerando', 'testando_sandbox', 'avaliando_risco',
        'aguardando_aprovacao', 'aplicando', 'concluido', 'rejeitado', 'falhou'
    )),
    to_state    text NOT NULL CHECK (to_state IN (
        'pendente', 'mapeando_contexto', 'gerando', 'testando_sandbox', 'avaliando_risco',
        'aguardando_aprovacao', 'aplicando', 'concluido', 'rejeitado', 'falhou'
    )),
    detail      text NOT NULL DEFAULT '',
    created_at  timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX task_steps_tenant_id ON task_steps (tenant_id);
CREATE INDEX task_steps_por_task_run ON task_steps (task_run_id, id);
