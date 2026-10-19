-- name: CreateApproval :execrows
-- Uma reentrega do mesmo delivery_id não duplica a aprovação.
INSERT INTO approvals (tenant_id, task_run_id, delivery_id, decision, reviewer, feedback_reason, justification, decided_at)
SELECT tr.tenant_id, tr.id, sqlc.arg(delivery_id), sqlc.arg(decision), sqlc.arg(reviewer),
       sqlc.narg(feedback_reason), sqlc.narg(justification), sqlc.arg(decided_at)
FROM task_runs tr
WHERE tr.id = sqlc.arg(task_run_id)
ON CONFLICT (delivery_id) DO NOTHING;

-- name: ListApprovalsByTaskRun :many
SELECT * FROM approvals WHERE task_run_id = $1 ORDER BY decided_at, id;
