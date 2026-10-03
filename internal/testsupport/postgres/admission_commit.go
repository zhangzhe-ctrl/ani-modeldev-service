package postgres

import (
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// RejectAdmissionCommit faults only this test's admission INSERT at real
// COMMIT. It shares the existing isolated schema, migration role, deadline and
// cleanup guards; previously committed close facts are outside that transaction.
func RejectAdmissionCommit(t testing.TB, runtimePool *pgxpool.Pool) func() {
	t.Helper()
	return rejectInsertCommit(t, runtimePool, "modeldev_executions", "reject_admission_commit", "injected_admission_commit_failure")
}
