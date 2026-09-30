package postgres

import (
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// RejectDispatchCommit faults only this test's initial dispatch INSERT at real
// COMMIT, before reservation may grant a send permit. It shares the existing
// isolated schema, migration role, deadline and cleanup guards.
func RejectDispatchCommit(t testing.TB, runtimePool *pgxpool.Pool) func() {
	t.Helper()
	return rejectInsertCommit(t, runtimePool, "modeldev_pipeline_dispatches", "reject_dispatch_commit", "injected_dispatch_commit_failure")
}
