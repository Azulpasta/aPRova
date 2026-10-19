-- receipts é append-only: não existe query de UPDATE nem de DELETE, e o
-- trigger da migration recusa as duas operações.

-- name: AppendReceipt :one
INSERT INTO receipts (tenant_id, task_run_id, previous_receipt_id, payload, content_hash, signature, key_id, signed_at)
SELECT tr.tenant_id, tr.id, sqlc.narg(previous_receipt_id), sqlc.arg(payload), sqlc.arg(content_hash),
       sqlc.arg(signature), sqlc.arg(key_id), sqlc.arg(signed_at)
FROM task_runs tr
WHERE tr.id = sqlc.arg(task_run_id)
RETURNING *;

-- name: GetReceipt :one
SELECT * FROM receipts WHERE id = $1;

-- name: GetReceiptByTaskRun :one
SELECT * FROM receipts WHERE task_run_id = $1;
