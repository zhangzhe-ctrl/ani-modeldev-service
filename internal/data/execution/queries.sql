-- name: InsertExecution :one
INSERT INTO modeldev_executions (
    tenant_id, execution_id, operation_id, actor,
    intent_canonical, intent_hash, snapshot_canonical, spec_hash, accepted_at
) VALUES (
    sqlc.arg(tenant_id)::uuid, sqlc.arg(execution_id)::uuid,
    sqlc.arg(operation_id)::uuid, sqlc.arg(actor),
    sqlc.arg(intent_canonical), sqlc.arg(intent_hash),
    sqlc.arg(snapshot_canonical), sqlc.arg(spec_hash), sqlc.arg(accepted_at)
)
RETURNING tenant_id, execution_id, operation_id, actor,
    intent_canonical, intent_hash, snapshot_canonical, spec_hash, accepted_at;

-- name: GetExecution :one
SELECT tenant_id, execution_id, operation_id, actor,
    intent_canonical, intent_hash, snapshot_canonical, spec_hash, accepted_at
FROM modeldev_executions
WHERE tenant_id = sqlc.arg(tenant_id)::uuid
  AND execution_id = sqlc.arg(execution_id)::uuid;
