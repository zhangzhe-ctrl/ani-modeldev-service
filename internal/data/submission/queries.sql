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

-- Sample this database's clock after the outbound call. The adapter compares
-- the returned immutable attempt/hash with the original permit and requires
-- observed_at >= reserved_at. Keeping the tenant-scoped row distinguishes a
-- missing reservation from a conflicting attempt without a second lookup.
-- This read neither authorizes a send nor locks across network work.
-- name: GetSubmissionObservationTime :one
SELECT attempt_id, plan_hash, reserved_at,
    clock_timestamp()::timestamptz AS observed_at
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

-- These immutable observations belong to one exact reservation, not to an
-- authoritative Run binding. Ordering gives stable output, never priority.
-- Readers combining this list with dispatch state must use one consistent
-- snapshot; writers hold the shared execution identity lock.
-- name: ListConfirmedPipelineRuns :many
SELECT tenant_id, execution_id, attempt_id, plan_hash, run_id, first_observed_at
FROM modeldev_pipeline_confirmed_runs
WHERE tenant_id = sqlc.arg(tenant_id)::uuid
  AND execution_id = sqlc.arg(execution_id)::uuid
  AND attempt_id = sqlc.arg(attempt_id)::uuid
  AND plan_hash = sqlc.arg(plan_hash)::text
ORDER BY run_id;

-- The caller has matched the original permit and complete frozen plan under
-- the identity lock. A late observation survives close/deadline. Replaying the
-- same Run never refreshes its first time; another Run is retained separately.
-- name: InsertConfirmedPipelineRun :execrows
INSERT INTO modeldev_pipeline_confirmed_runs (
    tenant_id, execution_id, attempt_id, plan_hash, run_id, first_observed_at
)
SELECT tenant_id, execution_id, attempt_id, plan_hash,
    sqlc.arg(run_id)::uuid, sqlc.arg(observed_at)::timestamptz
FROM modeldev_pipeline_dispatches
WHERE tenant_id = sqlc.arg(tenant_id)::uuid
  AND execution_id = sqlc.arg(execution_id)::uuid
  AND attempt_id = sqlc.arg(attempt_id)::uuid
  AND plan_hash = sqlc.arg(plan_hash)::text
  AND state IN ('SUBMITTING', 'SUBMISSION_UNCERTAIN', 'SUBMISSION_CONFIRMED')
  AND sqlc.arg(observed_at)::timestamptz >= reserved_at
ON CONFLICT (tenant_id, execution_id, attempt_id, run_id) DO NOTHING;

-- In the same transaction as the retained handle, mark the original attempt's
-- first confirmation without erasing an earlier uncertainty observation.
-- name: MarkSubmissionConfirmed :one
UPDATE modeldev_pipeline_dispatches AS dispatch
SET state = 'SUBMISSION_CONFIRMED'
WHERE dispatch.tenant_id = sqlc.arg(tenant_id)::uuid
  AND dispatch.execution_id = sqlc.arg(execution_id)::uuid
  AND dispatch.attempt_id = sqlc.arg(attempt_id)::uuid
  AND dispatch.plan_hash = sqlc.arg(plan_hash)::text
  AND dispatch.state IN ('SUBMITTING', 'SUBMISSION_UNCERTAIN')
  AND EXISTS (
      SELECT 1
      FROM modeldev_pipeline_confirmed_runs AS observed
      WHERE observed.tenant_id = dispatch.tenant_id
        AND observed.execution_id = dispatch.execution_id
        AND observed.attempt_id = dispatch.attempt_id
        AND observed.plan_hash = dispatch.plan_hash
  )
RETURNING dispatch.tenant_id, dispatch.execution_id, dispatch.operation_id,
    dispatch.spec_hash, dispatch.attempt_id, dispatch.plan_canonical,
    dispatch.plan_hash, dispatch.state, dispatch.reserved_at, dispatch.uncertain_at;

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
