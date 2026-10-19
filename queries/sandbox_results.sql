-- name: CreateSandboxResult :one
-- O tenant vem do próprio task_run, para o job do sandbox não precisar sabê-lo.
INSERT INTO sandbox_results (
    tenant_id, task_run_id, attempt, compiled, tests_passed, tests_failed,
    raw_output, duration_ms, failure_type
)
SELECT tr.tenant_id, tr.id, sqlc.arg(attempt), sqlc.arg(compiled), sqlc.arg(tests_passed),
       sqlc.arg(tests_failed), sqlc.arg(raw_output), sqlc.arg(duration_ms), sqlc.arg(failure_type)
FROM task_runs tr
WHERE tr.id = sqlc.arg(task_run_id)
RETURNING *;

-- name: GetLatestSandboxResult :one
SELECT * FROM sandbox_results
WHERE task_run_id = $1
ORDER BY attempt DESC
LIMIT 1;
