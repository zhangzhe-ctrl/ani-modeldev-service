-- Preserve the first local no-send observation without losing stronger facts.
-- It grants no replacement attempt/permit and is not an execution-close state.
ALTER TABLE modeldev_pipeline_dispatches
    ADD COLUMN not_sent_at timestamptz,
    DROP CONSTRAINT modeldev_pipeline_dispatches_state_check,
    DROP CONSTRAINT modeldev_pipeline_dispatches_uncertainty_check,
    ADD CONSTRAINT modeldev_pipeline_dispatches_state_check
        CHECK (state IN ('SUBMITTING', 'SUBMISSION_NOT_SENT', 'SUBMISSION_UNCERTAIN', 'SUBMISSION_CONFIRMED')),
    ADD CONSTRAINT modeldev_pipeline_dispatches_uncertainty_check CHECK (
        (uncertain_at IS NULL OR (
            isfinite(uncertain_at)
            AND uncertain_at >= reserved_at
            AND uncertain_at >= '0001-01-01 00:00:00+00'::timestamptz
            AND uncertain_at < '10000-01-01 00:00:00+00'::timestamptz
        ))
        AND (
            (state IN ('SUBMITTING', 'SUBMISSION_NOT_SENT') AND uncertain_at IS NULL)
            OR (state = 'SUBMISSION_UNCERTAIN' AND uncertain_at IS NOT NULL)
            OR state = 'SUBMISSION_CONFIRMED'
        )
    ),
    ADD CONSTRAINT modeldev_pipeline_dispatches_not_sent_check CHECK (
        (not_sent_at IS NULL OR (
            isfinite(not_sent_at)
            AND not_sent_at >= reserved_at
            AND not_sent_at >= '0001-01-01 00:00:00+00'::timestamptz
            AND not_sent_at < '10000-01-01 00:00:00+00'::timestamptz
        ))
        AND (
            (state = 'SUBMITTING' AND not_sent_at IS NULL)
            OR (state = 'SUBMISSION_NOT_SENT' AND not_sent_at IS NOT NULL)
            OR state IN ('SUBMISSION_UNCERTAIN', 'SUBMISSION_CONFIRMED')
        )
    );

-- No old observation is backfilled or rewritten. Existing tenant-preserving
-- keys, immutable plan/attempt identity and disabled RLS remain unchanged.
