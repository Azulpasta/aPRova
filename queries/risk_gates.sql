-- name: CreateRiskGate :one
INSERT INTO risk_gates (tenant_id, task_run_id, deterministic_level, level, reason)
SELECT tr.tenant_id, tr.id, sqlc.arg(deterministic_level), sqlc.arg(level), sqlc.arg(reason)
FROM task_runs tr
WHERE tr.id = sqlc.arg(task_run_id)
RETURNING *;

-- name: GetRiskGateByTaskRun :one
SELECT * FROM risk_gates WHERE task_run_id = $1;
