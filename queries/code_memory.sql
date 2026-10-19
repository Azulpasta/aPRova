-- name: CreateCodeMemory :one
INSERT INTO code_memory (tenant_id, task_run_id, repository, summary, embedding, level, outcome)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: FindNearestCodeMemory :many
-- Distância de cosseno, a mesma do índice HNSW. Só olha o próprio tenant.
SELECT id, tenant_id, task_run_id, repository, summary, level, outcome, created_at,
       (embedding <=> sqlc.arg(embedding)::vector)::float8 AS distance
FROM code_memory
WHERE tenant_id = sqlc.arg(tenant_id)
ORDER BY embedding <=> sqlc.arg(embedding)::vector
LIMIT sqlc.arg(max_results);
