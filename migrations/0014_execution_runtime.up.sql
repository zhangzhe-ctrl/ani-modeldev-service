-- Runtime facts are separate from the immutable admission and authority. Every
-- writer locks the shared identity, including all close/creation decisions.
CREATE TABLE modeldev_execution_runtimes (
    tenant_id uuid NOT NULL,
    execution_id uuid NOT NULL,
    operation_id uuid NOT NULL,
    spec_hash text NOT NULL,
    facts jsonb NOT NULL CHECK (jsonb_typeof(facts) = 'object'),
    training_name text,
    training_request_sha256 text CHECK (training_request_sha256 ~ '^[0-9a-f]{64}$'),
    training_uid text CHECK (length(training_uid) BETWEEN 1 AND 128),
    publication_id uuid,
    close_generation numeric(20,0) NOT NULL DEFAULT 0 CHECK (close_generation >= 0 AND close_generation <= 18446744073709551615),
    close_reason text CHECK (close_reason IN ('NATURAL_TERMINAL', 'STEP_FAILED', 'DEADLINE', 'USER_STOP')),
    close_requested_at timestamptz,
    closed_at timestamptz,
    PRIMARY KEY (tenant_id, execution_id),
    FOREIGN KEY (tenant_id, execution_id, operation_id, spec_hash)
        REFERENCES modeldev_execution_identities (tenant_id, execution_id, operation_id, spec_hash),
    FOREIGN KEY (tenant_id, execution_id)
        REFERENCES modeldev_run_authorities (tenant_id, execution_id),
    CHECK ((training_name IS NULL) = (training_request_sha256 IS NULL)),
    CHECK (training_name IS NULL OR training_name = 'md-' || execution_id::text),
    CHECK (training_uid IS NULL OR training_name IS NOT NULL),
    CHECK (publication_id IS NULL OR training_uid IS NOT NULL),
    CHECK ((facts->'Training'->>'Name') IS NOT DISTINCT FROM training_name),
    CHECK ((facts->'Training'->>'RequestSHA256') IS NOT DISTINCT FROM training_request_sha256),
    CHECK ((facts->'TrainingHandle'->>'TrainJobUID') IS NOT DISTINCT FROM training_uid),
    CHECK ((facts->'Publication'->>'ID')::uuid IS NOT DISTINCT FROM publication_id),
    CHECK ((facts->>'CloseGeneration')::numeric = close_generation),
    CHECK ((close_generation = 0 AND close_reason IS NULL AND close_requested_at IS NULL AND closed_at IS NULL)
        OR (close_generation > 0 AND close_reason IS NOT NULL AND close_requested_at IS NOT NULL)),
    CHECK (closed_at IS NULL OR closed_at >= close_requested_at)
);
ALTER TABLE modeldev_execution_runtimes DISABLE ROW LEVEL SECURITY;
