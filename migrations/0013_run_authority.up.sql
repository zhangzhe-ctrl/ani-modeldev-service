-- One immutable authority association per execution. This is not a training
-- creation permit; the identity lock and close fence still protect creators.
CREATE TABLE modeldev_run_authorities (
    tenant_id uuid NOT NULL,
    execution_id uuid NOT NULL,
    operation_id uuid NOT NULL,
    spec_hash text NOT NULL CHECK (spec_hash ~ '^[0-9a-f]{64}$'),
    attempt_id uuid NOT NULL CHECK (attempt_id <> '00000000-0000-0000-0000-000000000000'),
    plan_hash text NOT NULL CHECK (plan_hash ~ '^[0-9a-f]{64}$'),
    run_id uuid NOT NULL CHECK (run_id <> '00000000-0000-0000-0000-000000000000'),
    namespace_name text NOT NULL CHECK (length(namespace_name) BETWEEN 1 AND 63),
    namespace_uid uuid NOT NULL CHECK (namespace_uid <> '00000000-0000-0000-0000-000000000000'),
    workflow_name text NOT NULL CHECK (length(workflow_name) BETWEEN 1 AND 253),
    workflow_uid text NOT NULL CHECK (length(workflow_uid) BETWEEN 1 AND 128),
    bound_at timestamptz NOT NULL CHECK (
        isfinite(bound_at)
        AND bound_at >= '0001-01-01 00:00:00+00'::timestamptz
        AND bound_at < '10000-01-01 00:00:00+00'::timestamptz
    ),
    PRIMARY KEY (tenant_id, execution_id),
    FOREIGN KEY (tenant_id, execution_id, operation_id, spec_hash)
        REFERENCES modeldev_execution_identities (tenant_id, execution_id, operation_id, spec_hash),
    FOREIGN KEY (tenant_id, execution_id, attempt_id, plan_hash)
        REFERENCES modeldev_pipeline_dispatches (tenant_id, execution_id, attempt_id, plan_hash)
);

ALTER TABLE modeldev_run_authorities DISABLE ROW LEVEL SECURITY;
