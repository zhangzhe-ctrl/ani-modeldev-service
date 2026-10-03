-- Close may precede the first Run: keep execution identity mandatory, and
-- require a normal Run authority whenever runtime facts can contain compute.
-- A fenced, resource-free close record never grants a managed-step authority.
ALTER TABLE modeldev_execution_runtimes
    ADD COLUMN authority_execution_id uuid GENERATED ALWAYS AS (
        CASE WHEN close_generation = 0
            OR facts->'Workspace' IS DISTINCT FROM 'null'::jsonb
            OR facts->'Training' IS DISTINCT FROM 'null'::jsonb
            OR facts->'TrainingHandle' IS DISTINCT FROM 'null'::jsonb
            OR facts->'Observation' IS DISTINCT FROM 'null'::jsonb
            OR facts->'Publication' IS DISTINCT FROM 'null'::jsonb
        THEN execution_id ELSE NULL END
    ) STORED;
ALTER TABLE modeldev_execution_runtimes
    ADD CONSTRAINT modeldev_runtime_compute_authority_fkey
    FOREIGN KEY (tenant_id, authority_execution_id)
    REFERENCES modeldev_run_authorities (tenant_id, execution_id);
ALTER TABLE modeldev_execution_runtimes
    DROP CONSTRAINT modeldev_execution_runtimes_tenant_id_execution_id_fkey;
