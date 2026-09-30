-- The first execution row is also the durable command inbox entry. No command
-- can be acknowledged separately from the immutable admission it creates.
CREATE TABLE modeldev_executions (
    tenant_id uuid NOT NULL,
    execution_id uuid NOT NULL,
    operation_id uuid NOT NULL,
    actor text NOT NULL CHECK (actor <> ''),
    intent_canonical bytea NOT NULL CHECK (octet_length(intent_canonical) > 0),
    intent_hash text NOT NULL CHECK (intent_hash ~ '^[0-9a-f]{64}$'),
    snapshot_canonical bytea NOT NULL CHECK (octet_length(snapshot_canonical) > 0),
    spec_hash text NOT NULL CHECK (spec_hash ~ '^[0-9a-f]{64}$'),
    accepted_at timestamptz NOT NULL,
    PRIMARY KEY (tenant_id, execution_id),
    UNIQUE (tenant_id, operation_id)
);

ALTER TABLE modeldev_executions DISABLE ROW LEVEL SECURITY;
