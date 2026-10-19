CREATE TABLE identity_links (
    id            bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    tenant_id     bigint NOT NULL CHECK (tenant_id > 0),
    slack_user_id text NOT NULL,
    github_login  text NOT NULL,
    created_at    timestamptz NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, slack_user_id)
);

CREATE INDEX identity_links_tenant_id ON identity_links (tenant_id);
