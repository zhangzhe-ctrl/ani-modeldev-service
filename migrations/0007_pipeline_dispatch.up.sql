-- Phase A retains one immutable outbound reservation per admitted execution.
-- A close-only identity tombstone is insufficient: both foreign keys must
-- resolve, including the full tenant-preserving immutable identity tuple.
CREATE TABLE modeldev_pipeline_dispatches (
    tenant_id uuid NOT NULL,
    execution_id uuid NOT NULL,
    operation_id uuid NOT NULL,
    spec_hash text NOT NULL CHECK (spec_hash ~ '^[0-9a-f]{64}$'),
    attempt_id uuid NOT NULL CHECK (attempt_id <> '00000000-0000-0000-0000-000000000000'),
    plan_canonical bytea NOT NULL CHECK (octet_length(plan_canonical) > 0),
    plan_hash text NOT NULL CHECK (plan_hash ~ '^[0-9a-f]{64}$'),
    state text NOT NULL CHECK (state = 'SUBMITTING'),
    reserved_at timestamptz NOT NULL CHECK (isfinite(reserved_at)),
    PRIMARY KEY (tenant_id, execution_id),
    UNIQUE (attempt_id),
    FOREIGN KEY (tenant_id, execution_id)
        REFERENCES modeldev_executions (tenant_id, execution_id),
    FOREIGN KEY (tenant_id, execution_id, operation_id, spec_hash)
        REFERENCES modeldev_execution_identities (tenant_id, execution_id, operation_id, spec_hash)
);

ALTER TABLE modeldev_pipeline_dispatches DISABLE ROW LEVEL SECURITY;
