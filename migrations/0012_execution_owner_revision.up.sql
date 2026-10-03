-- Apply in one migration transaction after stopping all old execution writers.
-- The baseline identifies the existing aggregate, not a reconstructed event
-- count. Old and new writers must not run together after this migration.
LOCK TABLE modeldev_execution_identities, modeldev_executions,
    modeldev_close_intents, modeldev_pipeline_dispatches,
    modeldev_pipeline_confirmed_runs IN ACCESS EXCLUSIVE MODE;

-- Refuse inconsistent history instead of hiding it behind a new baseline.
-- Preserve every original byte, identifier, fence, and observation timestamp.
DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM modeldev_execution_identities AS identity
        WHERE NOT EXISTS (
            SELECT 1 FROM modeldev_executions AS admission
            WHERE admission.tenant_id = identity.tenant_id
              AND admission.execution_id = identity.execution_id
        ) AND NOT EXISTS (
            SELECT 1 FROM modeldev_close_intents AS closing
            WHERE closing.tenant_id = identity.tenant_id
              AND closing.execution_id = identity.execution_id
        )
    ) OR EXISTS (
        SELECT 1 FROM modeldev_executions AS admission
        WHERE NOT EXISTS (
            SELECT 1 FROM modeldev_execution_identities AS identity
            WHERE identity.tenant_id = admission.tenant_id
              AND identity.execution_id = admission.execution_id
              AND identity.operation_id = admission.operation_id
              AND identity.spec_hash = admission.spec_hash
        )
    ) OR EXISTS (
        SELECT 1 FROM modeldev_close_intents AS closing
        WHERE NOT EXISTS (
            SELECT 1 FROM modeldev_execution_identities AS identity
            WHERE identity.tenant_id = closing.tenant_id
              AND identity.execution_id = closing.execution_id
              AND identity.operation_id = closing.operation_id
              AND identity.spec_hash = closing.spec_hash
        )
    ) OR EXISTS (
        SELECT 1 FROM modeldev_execution_identities AS identity
        WHERE identity.close_generation <> COALESCE((
            SELECT MAX(closing.owner_generation) FROM modeldev_close_intents AS closing
            WHERE closing.tenant_id = identity.tenant_id
              AND closing.execution_id = identity.execution_id
        ), 0)
    ) OR EXISTS (
        SELECT 1 FROM modeldev_pipeline_dispatches AS dispatch
        WHERE NOT EXISTS (
            SELECT 1 FROM modeldev_executions AS admission
            JOIN modeldev_execution_identities AS identity
              ON identity.tenant_id = admission.tenant_id
             AND identity.execution_id = admission.execution_id
             AND identity.operation_id = admission.operation_id
             AND identity.spec_hash = admission.spec_hash
            WHERE admission.tenant_id = dispatch.tenant_id
              AND admission.execution_id = dispatch.execution_id
              AND admission.operation_id = dispatch.operation_id
              AND admission.spec_hash = dispatch.spec_hash
        ) OR (dispatch.state = 'SUBMISSION_CONFIRMED') <> EXISTS (
            SELECT 1 FROM modeldev_pipeline_confirmed_runs AS observed
            WHERE observed.tenant_id = dispatch.tenant_id
              AND observed.execution_id = dispatch.execution_id
        )
    ) OR EXISTS (
        SELECT 1 FROM modeldev_pipeline_confirmed_runs AS observed
        WHERE NOT EXISTS (
            SELECT 1 FROM modeldev_pipeline_dispatches AS dispatch
            WHERE dispatch.tenant_id = observed.tenant_id
              AND dispatch.execution_id = observed.execution_id
              AND dispatch.attempt_id = observed.attempt_id
              AND dispatch.plan_hash = observed.plan_hash
              AND observed.first_observed_at >= dispatch.reserved_at
        )
    ) THEN
        RAISE EXCEPTION 'CPU04_OWNER_REVISION_INVALID_HISTORY' USING ERRCODE = '23514';
    END IF;
END
$$;

-- Keep arbitrary numeric scale so fractional writes fail the explicit check
-- instead of being rounded by a numeric(20,0) column before it is evaluated.
ALTER TABLE modeldev_execution_identities
    ADD COLUMN owner_revision numeric NOT NULL DEFAULT 0,
    ADD CONSTRAINT modeldev_execution_owner_revision_check CHECK (
        owner_revision >= 0 AND owner_revision <= 18446744073709551615
        AND owner_revision = trunc(owner_revision)
    );

UPDATE modeldev_execution_identities SET owner_revision = 1;

-- New identity rows start at zero only inside their first writer transaction.
-- Their first Admission or Close and revision=1 must commit together.
