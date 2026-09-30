-- Retain the first uncertainty observation for the original outbound attempt.
-- No new attempt, sending permission, handle or execution-close state is added.
ALTER TABLE modeldev_pipeline_dispatches
    ADD COLUMN uncertain_at timestamptz,
    DROP CONSTRAINT modeldev_pipeline_dispatches_state_check,
    ADD CONSTRAINT modeldev_pipeline_dispatches_state_check
        CHECK (state IN ('SUBMITTING', 'SUBMISSION_UNCERTAIN')),
    ADD CONSTRAINT modeldev_pipeline_dispatches_uncertainty_check CHECK (
        (state = 'SUBMITTING' AND uncertain_at IS NULL)
        OR
        (state = 'SUBMISSION_UNCERTAIN' AND uncertain_at IS NOT NULL
            AND isfinite(uncertain_at)
            AND uncertain_at >= reserved_at
            AND uncertain_at >= '0001-01-01 00:00:00+00'::timestamptz
            AND uncertain_at < '10000-01-01 00:00:00+00'::timestamptz)
    );
