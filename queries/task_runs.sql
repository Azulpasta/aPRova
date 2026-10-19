-- name: CreateTaskRun :one
-- Uma reentrega do mesmo delivery_id não cria linha nova: devolve zero linhas
-- e quem chama busca a existente por delivery_id.
INSERT INTO task_runs (tenant_id, delivery_id, repository, pr_number, head_sha, author, event_action, received_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
ON CONFLICT (delivery_id) DO NOTHING
RETURNING *;

-- name: GetTaskRun :one
SELECT * FROM task_runs WHERE id = $1;

-- name: GetTaskRunByDeliveryID :one
SELECT * FROM task_runs WHERE delivery_id = $1;

-- name: GetLatestTaskRunForPullRequest :one
SELECT * FROM task_runs
WHERE tenant_id = $1 AND repository = $2 AND pr_number = $3
ORDER BY created_at DESC, id DESC
LIMIT 1;

-- name: TransitionTaskRun :one
-- Só transiciona quem ainda está no estado esperado; zero linhas indica que
-- outro processo chegou antes.
UPDATE task_runs
SET state = sqlc.arg(to_state), updated_at = now()
WHERE id = sqlc.arg(id) AND state = sqlc.arg(from_state)
RETURNING *;
