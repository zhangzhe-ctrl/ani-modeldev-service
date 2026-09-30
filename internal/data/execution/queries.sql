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
ON CONFLICT DO NOTHING
RETURNING tenant_id, execution_id, operation_id, actor,
    intent_canonical, intent_hash, snapshot_canonical, spec_hash, accepted_at;

-- name: GetExecution :one
SELECT tenant_id, execution_id, operation_id, actor,
    intent_canonical, intent_hash, snapshot_canonical, spec_hash, accepted_at
FROM modeldev_executions
WHERE tenant_id = sqlc.arg(tenant_id)::uuid
  AND execution_id = sqlc.arg(execution_id)::uuid;

-- name: InsertExecutionIdentity :exec
INSERT INTO modeldev_execution_identities (tenant_id, execution_id, operation_id, spec_hash)
VALUES (
    sqlc.arg(tenant_id)::uuid, sqlc.arg(execution_id)::uuid,
    sqlc.arg(operation_id)::uuid, sqlc.arg(spec_hash)
)
ON CONFLICT DO NOTHING;

-- name: LockExecutionIdentity :one
SELECT tenant_id, execution_id, operation_id, spec_hash, close_generation
FROM modeldev_execution_identities
WHERE tenant_id = sqlc.arg(tenant_id)::uuid
  AND execution_id = sqlc.arg(execution_id)::uuid
FOR UPDATE;

-- name: AdvanceCloseGeneration :one
UPDATE modeldev_execution_identities
SET close_generation = close_generation + 1
WHERE tenant_id = sqlc.arg(tenant_id)::uuid
  AND execution_id = sqlc.arg(execution_id)::uuid
RETURNING close_generation;

-- name: InsertCloseIntent :one
INSERT INTO modeldev_close_intents (
    tenant_id, execution_id, operation_id, spec_hash, source_kind,
    source_generation, owner_generation, reason, requested_at, requested_actor, close_state
) VALUES (
    sqlc.arg(tenant_id)::uuid, sqlc.arg(execution_id)::uuid,
    sqlc.arg(operation_id)::uuid, sqlc.arg(spec_hash), 'GOVERNANCE',
    sqlc.arg(source_generation), sqlc.arg(owner_generation), 'USER_STOP',
    sqlc.arg(requested_at), sqlc.arg(requested_actor), 'CLOSING'
)
RETURNING tenant_id, execution_id, operation_id, spec_hash, source_kind,
    source_generation, owner_generation, reason, requested_at, requested_actor, close_state;

-- name: GetCloseIntent :one
SELECT tenant_id, execution_id, operation_id, spec_hash, source_kind,
    source_generation, owner_generation, reason, requested_at, requested_actor, close_state
FROM modeldev_close_intents
WHERE tenant_id = sqlc.arg(tenant_id)::uuid
  AND execution_id = sqlc.arg(execution_id)::uuid
ORDER BY owner_generation DESC
LIMIT 1;
