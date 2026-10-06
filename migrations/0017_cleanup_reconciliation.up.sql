-- Preserve the first apply's timestamps and outcome while allowing a later
-- read-only reconciliation of the original exact-UID plan.
ALTER TABLE modeldev_execution_cleanup_audits
  DROP CONSTRAINT modeldev_execution_cleanup_audits_phase_check,
  DROP CONSTRAINT modeldev_execution_cleanup_audits_check;

ALTER TABLE modeldev_execution_cleanup_audits
  ADD CONSTRAINT modeldev_execution_cleanup_audits_phase_check
    CHECK (phase IN ('STARTED', 'APPLIED', 'NEEDS_REVIEW', 'RECONCILED')),
  ADD CONSTRAINT modeldev_execution_cleanup_audits_check CHECK (
    (phase = 'STARTED' AND completed_at IS NULL)
    OR (phase IN ('APPLIED', 'NEEDS_REVIEW') AND completed_at IS NOT NULL)
    OR (phase = 'RECONCILED'
      AND receipt->>'reconciled_from_phase' IS NOT NULL
      AND receipt->>'reconciled_at' IS NOT NULL
      AND (receipt->>'reconciled_at')::timestamptz >= started_at
      AND ((receipt->>'reconciled_from_phase' = 'STARTED' AND completed_at IS NULL)
        OR (receipt->>'reconciled_from_phase' = 'NEEDS_REVIEW' AND completed_at IS NOT NULL)))
  );
