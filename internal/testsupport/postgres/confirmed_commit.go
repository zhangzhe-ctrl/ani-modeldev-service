package postgres

import (
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// RejectConfirmedRunCommit faults only this test's confirmed-run INSERT at
// COMMIT, after the parent state and child handle have been written. It neither
// replaces a repository nor modifies another test's schema or product data.
func RejectConfirmedRunCommit(t testing.TB, runtimePool *pgxpool.Pool) func() {
	t.Helper()
	return rejectInsertCommit(t, runtimePool, "modeldev_pipeline_confirmed_runs", "reject_confirmed_run_commit", "injected_confirmed_run_commit_failure")
}
