-- name: CreateCodeMemory :one
-- Com task_run, ele precisa ser do mesmo tenant; senão nenhuma linha entra.
INSERT INTO code_memory (tenant_id, task_run_id, repository, summary, embedding, level, outcome)
SELECT sqlc.arg(tenant_id), sqlc.narg(task_run_id), sqlc.arg(repository), sqlc.arg(summary),
       sqlc.arg(embedding), sqlc.arg(level), sqlc.arg(outcome)
WHERE sqlc.narg(task_run_id)::uuid IS NULL OR EXISTS (
    SELECT 1 FROM task_runs tr
    WHERE tr.id = sqlc.narg(task_run_id)::uuid AND tr.tenant_id = sqlc.arg(tenant_id))
RETURNING id;

-- name: FindNearestCodeMemory :many
-- Distância de cosseno, a mesma do índice HNSW. Só olha o próprio tenant; o
-- repositório liga hnsw.iterative_scan antes, para o filtro não esvaziar o
-- resultado de um tenant pequeno.
SELECT id, tenant_id, task_run_id, repository, summary, level, outcome, created_at,
       (embedding <=> sqlc.arg(embedding)::vector)::float8 AS distance
FROM code_memory
WHERE tenant_id = sqlc.arg(tenant_id)
ORDER BY embedding <=> sqlc.arg(embedding)::vector
LIMIT sqlc.arg(max_results);
