-- One immutable identity reservation is shared by Admission and close intents.
-- The owner generation is the sole mutable field on this identity record.
CREATE TABLE modeldev_execution_identities (
    tenant_id uuid NOT NULL,
    execution_id uuid NOT NULL,
    operation_id uuid NOT NULL,
    spec_hash text NOT NULL CHECK (spec_hash ~ '^[0-9a-f]{64}$'),
    close_generation numeric(20, 0) NOT NULL DEFAULT 0
        CHECK (close_generation >= 0 AND close_generation <= 18446744073709551615),
    PRIMARY KEY (tenant_id, execution_id),
    UNIQUE (execution_id),
    UNIQUE (operation_id),
    UNIQUE (tenant_id, execution_id, operation_id, spec_hash)
);

ALTER TABLE modeldev_execution_identities DISABLE ROW LEVEL SECURITY;

INSERT INTO modeldev_execution_identities (tenant_id, execution_id, operation_id, spec_hash)
SELECT tenant_id, execution_id, operation_id, spec_hash
FROM modeldev_executions;

ALTER TABLE modeldev_executions
    ADD CONSTRAINT modeldev_executions_identity_fk
    FOREIGN KEY (tenant_id, execution_id, operation_id, spec_hash)
    REFERENCES modeldev_execution_identities (tenant_id, execution_id, operation_id, spec_hash);

CREATE TABLE modeldev_close_intents (
    tenant_id uuid NOT NULL,
    execution_id uuid NOT NULL,
    operation_id uuid NOT NULL,
    spec_hash text NOT NULL CHECK (spec_hash ~ '^[0-9a-f]{64}$'),
    source_kind text NOT NULL CHECK (source_kind = 'GOVERNANCE'),
    source_generation numeric(20, 0) NOT NULL
        CHECK (source_generation > 0 AND source_generation <= 18446744073709551615),
    owner_generation numeric(20, 0) NOT NULL
        CHECK (owner_generation > 0 AND owner_generation <= 18446744073709551615),
    reason text NOT NULL CHECK (reason = 'USER_STOP'),
    requested_at timestamptz NOT NULL,
    requested_actor text NOT NULL CHECK (requested_actor <> ''),
    close_state text NOT NULL CHECK (close_state = 'CLOSING'),
    PRIMARY KEY (tenant_id, execution_id, source_kind, source_generation),
    UNIQUE (tenant_id, execution_id, owner_generation),
    FOREIGN KEY (tenant_id, execution_id, operation_id, spec_hash)
        REFERENCES modeldev_execution_identities (tenant_id, execution_id, operation_id, spec_hash)
);

ALTER TABLE modeldev_close_intents DISABLE ROW LEVEL SECURITY;
