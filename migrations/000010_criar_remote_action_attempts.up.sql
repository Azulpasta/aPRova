-- Um id_acao repetido mantém o primeiro registro, por isso não há decisão
-- 'repetida' aqui: a repetição fica só no log.
CREATE TABLE remote_action_attempts (
    action_id     text PRIMARY KEY,
    tenant_id     bigint NOT NULL CHECK (tenant_id > 0),
    task_run_id   uuid REFERENCES task_runs (id) ON DELETE SET NULL,
    action_type   text NOT NULL CHECK (action_type IN ('aprovar', 'solicitar_alteracao', 'reexecutar_analise')),
    slack_user_id text NOT NULL,
    github_login  text,
    repository    text NOT NULL,
    pr_number     integer NOT NULL CHECK (pr_number > 0),
    decision      text NOT NULL CHECK (decision IN ('aceita', 'recusada', 'falha')),
    reason        text NOT NULL,
    attempted_at  timestamptz NOT NULL,
    created_at    timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX remote_action_attempts_tenant_id ON remote_action_attempts (tenant_id);
CREATE INDEX remote_action_attempts_task_run_id ON remote_action_attempts (task_run_id);
