CREATE TABLE modeldev_execution_cleanup_audits (
    tenant_id uuid NOT NULL,
    execution_id uuid NOT NULL,
    plan_sha256 text NOT NULL CHECK (plan_sha256 ~ '^[0-9a-f]{64}$'),
    actor text NOT NULL,
    phase text NOT NULL CHECK (phase IN ('STARTED', 'APPLIED', 'NEEDS_REVIEW')),
    plan jsonb NOT NULL,
    receipt jsonb NOT NULL,
    started_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    completed_at timestamptz,
    PRIMARY KEY (tenant_id, execution_id, plan_sha256),
    FOREIGN KEY (tenant_id, execution_id)
      REFERENCES modeldev_execution_identities (tenant_id, execution_id),
    CHECK ((phase = 'STARTED' AND completed_at IS NULL)
      OR (phase <> 'STARTED' AND completed_at IS NOT NULL))
);
