-- A finite validation observation never replaces the frozen source request.
-- Infrastructure read failure can resume that same object; content rejection
-- is terminal. Existing READY and VALIDATING records retain their state.
ALTER TABLE modeldev_input_versions
    DROP CONSTRAINT modeldev_input_versions_verification_check,
    ADD COLUMN failure_code text,
    ADD COLUMN failure_observed_at timestamptz,
    ADD CONSTRAINT modeldev_input_versions_verification_check CHECK (
        (state IN ('VALIDATING', 'REJECTED')
            AND verified_at IS NULL AND verified_schema_version IS NULL
            AND verified_row_count IS NULL AND verified_feature_count IS NULL)
        OR
        (state = 'READY'
            AND verified_at IS NOT NULL AND isfinite(verified_at)
            AND verified_at >= requested_at
            AND verified_schema_version IS NOT NULL AND verified_schema_version = 'ani.cpu.csv.v1'
            AND verified_row_count IS NOT NULL AND verified_row_count = 1024
            AND verified_feature_count IS NOT NULL AND verified_feature_count = 16)
    ),
    ADD CONSTRAINT modeldev_input_versions_failure_check CHECK (
        (state IN ('VALIDATING', 'READY')
            AND failure_code IS NULL AND failure_observed_at IS NULL)
        OR
        (failure_code IS NOT NULL AND failure_observed_at IS NOT NULL
            AND isfinite(failure_observed_at) AND failure_observed_at >= requested_at
            AND ((state = 'VALIDATING' AND failure_code = 'SOURCE_UNAVAILABLE')
                OR (state = 'REJECTED' AND failure_code = 'CONTENT_REJECTED')))
    );
