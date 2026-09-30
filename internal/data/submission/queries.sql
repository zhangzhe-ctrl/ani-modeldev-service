-- name: LockExecutionIdentity :one
SELECT tenant_id, execution_id, operation_id, spec_hash,
    close_generation = 0 AS creation_open
FROM modeldev_execution_identities
WHERE tenant_id = sqlc.arg(tenant_id)::uuid
  AND execution_id = sqlc.arg(execution_id)::uuid
FOR UPDATE;

-- name: GetAdmission :one
SELECT tenant_id, execution_id, operation_id, actor,
    intent_canonical, intent_hash, snapshot_canonical, spec_hash, accepted_at
FROM modeldev_executions
WHERE tenant_id = sqlc.arg(tenant_id)::uuid
  AND execution_id = sqlc.arg(execution_id)::uuid;

-- name: GetPipelineDispatch :one
SELECT tenant_id, execution_id, operation_id, spec_hash, attempt_id,
    plan_canonical, plan_hash, state, reserved_at, uncertain_at
FROM modeldev_pipeline_dispatches
WHERE tenant_id = sqlc.arg(tenant_id)::uuid
  AND execution_id = sqlc.arg(execution_id)::uuid;

-- The caller holds this identity's row lock and has compared the complete
-- admitted command. Check the current database clock after acquiring the lock;
-- transaction-start time could predate a long wait behind a close transaction.
-- name: InsertPipelineDispatch :one
INSERT INTO modeldev_pipeline_dispatches (
    tenant_id, execution_id, operation_id, spec_hash, attempt_id,
    plan_canonical, plan_hash, state, reserved_at
)
SELECT tenant_id, execution_id, operation_id, spec_hash,
    sqlc.arg(attempt_id)::uuid, sqlc.arg(plan_canonical)::bytea,
    sqlc.arg(plan_hash)::text, 'SUBMITTING', clock_timestamp()
FROM modeldev_execution_identities
WHERE tenant_id = sqlc.arg(tenant_id)::uuid
  AND execution_id = sqlc.arg(execution_id)::uuid
  AND operation_id = sqlc.arg(operation_id)::uuid
  AND spec_hash = sqlc.arg(spec_hash)::text
  AND close_generation = 0
  AND clock_timestamp() < sqlc.arg(deadline_at)::timestamptz
RETURNING tenant_id, execution_id, operation_id, spec_hash, attempt_id,
    plan_canonical, plan_hash, state, reserved_at, uncertain_at;

-- The caller holds the shared identity lock and has matched the original
-- attempt and frozen plan. Close/deadline do not discard late observations.
-- An already uncertain row is read back unchanged rather than updated again.
-- name: MarkSubmissionUncertain :one
UPDATE modeldev_pipeline_dispatches
SET state = 'SUBMISSION_UNCERTAIN', uncertain_at = sqlc.arg(observed_at)::timestamptz
WHERE tenant_id = sqlc.arg(tenant_id)::uuid
  AND execution_id = sqlc.arg(execution_id)::uuid
  AND attempt_id = sqlc.arg(attempt_id)::uuid
  AND plan_hash = sqlc.arg(plan_hash)::text
  AND state = 'SUBMITTING'
  AND uncertain_at IS NULL
  AND sqlc.arg(observed_at)::timestamptz >= reserved_at
RETURNING tenant_id, execution_id, operation_id, spec_hash, attempt_id,
    plan_canonical, plan_hash, state, reserved_at, uncertain_at;
