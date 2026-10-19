-- name: CreateTaskStep :one
INSERT INTO task_steps (tenant_id, task_run_id, from_state, to_state, detail)
SELECT tr.tenant_id, tr.id, sqlc.arg(from_state), sqlc.arg(to_state), sqlc.arg(detail)
FROM task_runs tr
WHERE tr.id = sqlc.arg(task_run_id)
RETURNING *;
