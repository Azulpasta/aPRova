-- name: UpsertIdentityLink :one
INSERT INTO identity_links (tenant_id, slack_user_id, github_login)
VALUES ($1, $2, $3)
ON CONFLICT (tenant_id, slack_user_id) DO UPDATE SET github_login = EXCLUDED.github_login
RETURNING *;

-- name: ResolveIdentityLink :one
SELECT github_login FROM identity_links
WHERE tenant_id = $1 AND slack_user_id = $2;
