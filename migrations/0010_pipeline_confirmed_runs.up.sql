-- Retain creation-response observations, including conflicting Run handles.
-- These facts grant neither Run authority nor another sending permission.
ALTER TABLE modeldev_pipeline_dispatches
    ADD CONSTRAINT modeldev_pipeline_dispatches_observation_identity_key
        UNIQUE (tenant_id, execution_id, attempt_id, plan_hash),
    DROP CONSTRAINT modeldev_pipeline_dispatches_state_check,
    DROP CONSTRAINT modeldev_pipeline_dispatches_uncertainty_check,
    ADD CONSTRAINT modeldev_pipeline_dispatches_state_check
        CHECK (state IN ('SUBMITTING', 'SUBMISSION_UNCERTAIN', 'SUBMISSION_CONFIRMED')),
    ADD CONSTRAINT modeldev_pipeline_dispatches_uncertainty_check CHECK (
        (uncertain_at IS NULL OR (
            isfinite(uncertain_at)
            AND uncertain_at >= reserved_at
            AND uncertain_at >= '0001-01-01 00:00:00+00'::timestamptz
            AND uncertain_at < '10000-01-01 00:00:00+00'::timestamptz
        ))
        AND (
            (state = 'SUBMITTING' AND uncertain_at IS NULL)
            OR (state = 'SUBMISSION_UNCERTAIN' AND uncertain_at IS NOT NULL)
            OR state = 'SUBMISSION_CONFIRMED'
        )
    );

CREATE TABLE modeldev_pipeline_confirmed_runs (
    tenant_id uuid NOT NULL,
    execution_id uuid NOT NULL,
    attempt_id uuid NOT NULL CHECK (attempt_id <> '00000000-0000-0000-0000-000000000000'),
    plan_hash text NOT NULL CHECK (plan_hash ~ '^[0-9a-f]{64}$'),
    run_id uuid NOT NULL CHECK (run_id <> '00000000-0000-0000-0000-000000000000'),
    first_observed_at timestamptz NOT NULL CHECK (
        isfinite(first_observed_at)
        AND first_observed_at >= '0001-01-01 00:00:00+00'::timestamptz
        AND first_observed_at < '10000-01-01 00:00:00+00'::timestamptz
    ),
    PRIMARY KEY (tenant_id, execution_id, attempt_id, run_id),
    FOREIGN KEY (tenant_id, execution_id, attempt_id, plan_hash)
        REFERENCES modeldev_pipeline_dispatches (tenant_id, execution_id, attempt_id, plan_hash)
);

-- Run IDs are interpreted in the parent's frozen environment. No global Run
-- uniqueness or one-Run-per-attempt constraint may discard conflicting facts.
ALTER TABLE modeldev_pipeline_confirmed_runs DISABLE ROW LEVEL SECURITY;
