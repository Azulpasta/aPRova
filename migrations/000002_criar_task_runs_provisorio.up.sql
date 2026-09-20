CREATE TABLE task_runs (
    id          uuid PRIMARY KEY DEFAULT uuid_generate_v4(),
    delivery_id text NOT NULL UNIQUE,
    repositorio text NOT NULL,
    numero_pr   integer NOT NULL,
    sha_head    text NOT NULL,
    autor       text NOT NULL,
    acao        text NOT NULL,
    estado      text NOT NULL DEFAULT 'pendente',
    recebido_em timestamptz NOT NULL,
    criado_em   timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX task_runs_fila_por_estado ON task_runs (estado, criado_em);
CREATE INDEX task_runs_por_pull_request ON task_runs (repositorio, numero_pr);

CREATE TABLE decisoes_de_review (
    id          uuid PRIMARY KEY DEFAULT uuid_generate_v4(),
    delivery_id text NOT NULL UNIQUE,
    task_run_id uuid REFERENCES task_runs (id),
    repositorio text NOT NULL,
    numero_pr   integer NOT NULL,
    estado      text NOT NULL,
    revisor     text NOT NULL,
    recebido_em timestamptz NOT NULL,
    criado_em   timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX decisoes_de_review_por_task_run ON decisoes_de_review (task_run_id);
