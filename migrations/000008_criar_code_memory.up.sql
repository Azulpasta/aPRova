CREATE TABLE code_memory (
    id          bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    tenant_id   bigint NOT NULL CHECK (tenant_id > 0),
    task_run_id uuid REFERENCES task_runs (id) ON DELETE SET NULL,
    repository  text NOT NULL,
    summary     text NOT NULL,
    embedding   vector(1536) NOT NULL,
    level       text NOT NULL CHECK (level IN ('baixo', 'medio', 'alto')),
    outcome     text NOT NULL CHECK (outcome IN (
        'revertida_apos_merge', 'alto_risco_aprovada', 'fechada_sem_merge',
        'ajustada_e_aprovada', 'sem_reversao'
    )),
    created_at  timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX code_memory_tenant_id ON code_memory (tenant_id);
CREATE INDEX code_memory_task_run_id ON code_memory (task_run_id);
CREATE INDEX code_memory_embedding ON code_memory USING hnsw (embedding vector_cosine_ops);
