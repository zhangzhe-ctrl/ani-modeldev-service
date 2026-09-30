-- Governance assigns each operation and execution once globally. Tenant-scoped
-- keys remain for filtered access and tenant-preserving foreign references.
-- Existing conflicting rows must make this migration fail; never rewrite or
-- discard an immutable admission to force the constraint into place.
ALTER TABLE modeldev_executions
    ADD CONSTRAINT modeldev_executions_operation_id_unique UNIQUE (operation_id),
    ADD CONSTRAINT modeldev_executions_execution_id_unique UNIQUE (execution_id);
