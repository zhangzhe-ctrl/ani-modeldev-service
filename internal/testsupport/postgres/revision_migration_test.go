//go:build revisionupgrade

package postgres

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestOwnerRevisionMigrationRejectsInvalidHistory(t *testing.T) {
	for _, scenario := range []string{"orphan identity", "mismatched close fence", "confirmed without Run", "Run before reservation"} {
		t.Run(scenario, func(t *testing.T) {
			fixture := PrepareRevisionUpgrade(t)
			fixture.SeedOldWriter(t)
			pool := fixture.OpenRuntimePool(t)
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			// Corrupt only this parent's random test schema, after genuine old
			// writers have exited. These are invalid-history fixtures, not a
			// substitute for the old writer's valid Admission or observations.
			transaction := fixture.schema.migrationTransaction(t, ctx)
			defer rollbackMigration(transaction)
			var tag pgconn.CommandTag
			var err error
			switch scenario {
			case "orphan identity":
				tag, err = transaction.Exec(ctx, `INSERT INTO modeldev_execution_identities
                    (tenant_id, execution_id, operation_id, spec_hash)
                    VALUES ($1, $2, $3, repeat('a', 64))`, uuid.NewString(), uuid.NewString(), uuid.NewString())
			case "mismatched close fence":
				tag, err = transaction.Exec(ctx, `UPDATE modeldev_execution_identities
                    SET close_generation = close_generation + 1
                    WHERE execution_id = (SELECT execution_id FROM modeldev_close_intents
                        ORDER BY execution_id LIMIT 1)`)
			case "confirmed without Run":
				tag, err = transaction.Exec(ctx, `UPDATE modeldev_pipeline_dispatches
                    SET state = 'SUBMISSION_CONFIRMED' WHERE state = 'SUBMISSION_UNCERTAIN'`)
			case "Run before reservation":
				tag, err = transaction.Exec(ctx, `UPDATE modeldev_pipeline_confirmed_runs AS observed
                    SET first_observed_at = dispatch.reserved_at - interval '1 microsecond'
                    FROM modeldev_pipeline_dispatches AS dispatch
                    WHERE observed.tenant_id = dispatch.tenant_id
                      AND observed.execution_id = dispatch.execution_id
                      AND observed.run_id = (SELECT run_id FROM modeldev_pipeline_confirmed_runs
                          ORDER BY run_id LIMIT 1)`)
			}
			if err != nil || tag.RowsAffected() != 1 {
				t.Fatal("REVISION_UPGRADE_PREFLIGHT: invalid-history fixture did not alter exactly one old row; behavior NOT_RUN")
			}
			if err := transaction.Commit(ctx); err != nil {
				t.Fatal("REVISION_UPGRADE_PREFLIGHT: invalid-history fixture did not commit; behavior NOT_RUN")
			}
			before := revisionHistoryBytes(t, ctx, pool)
			migration, err := os.ReadFile(filepath.Join(repositoryRoot(t), "migrations", "0012_execution_owner_revision.up.sql"))
			if err != nil {
				t.Fatal("REVISION_UPGRADE_PREFLIGHT: owner revision migration missing; behavior NOT_RUN")
			}
			t.Log("REVISION_UPGRADE_PREFLIGHT PASS: real old writer facts and exact invalid-history row committed")
			migrationTx := fixture.schema.migrationTransaction(t, ctx)
			defer rollbackMigration(migrationTx)
			_, migrationErr := migrationTx.Exec(ctx, string(migration))
			var databaseError *pgconn.PgError
			if !errors.As(migrationErr, &databaseError) || databaseError.Code != "23514" || databaseError.Message != "CPU04_OWNER_REVISION_INVALID_HISTORY" {
				t.Fatal("REVISION_UPGRADE_BEHAVIOR: migration did not reject inconsistent old history with its exact guard")
			}
			if err := migrationTx.Rollback(ctx); err != nil {
				t.Fatal("REVISION_UPGRADE_BEHAVIOR: rejected migration failed to roll back")
			}
			var hasRevision bool
			if err := pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM information_schema.columns
                    WHERE table_schema = current_schema() AND table_name = 'modeldev_execution_identities'
                      AND column_name = 'owner_revision')`).Scan(&hasRevision); err != nil || hasRevision {
				t.Fatal("REVISION_UPGRADE_BEHAVIOR: rejected migration exposed a partial revision column")
			}
			if after := revisionHistoryBytes(t, ctx, pool); after != before {
				t.Fatal("REVISION_UPGRADE_BEHAVIOR: rejected migration rewrote existing historical facts")
			}
		})
	}
}

// Each value is a single statement snapshot of the entire exclusive schema's
// five old fact tables. JSONB serializes bytea as its actual hex bytes and fixes
// key order; no secret, raw driver error or private manifest is printed.
func revisionHistoryBytes(t *testing.T, ctx context.Context, pool *pgxpool.Pool) string {
	t.Helper()
	var snapshot string
	err := pool.QueryRow(ctx, `SELECT jsonb_build_object(
        'identities', (SELECT jsonb_agg(to_jsonb(i) ORDER BY i.tenant_id, i.execution_id) FROM modeldev_execution_identities i),
        'admissions', (SELECT jsonb_agg(to_jsonb(a) ORDER BY a.tenant_id, a.execution_id) FROM modeldev_executions a),
        'closes', (SELECT jsonb_agg(to_jsonb(c) ORDER BY c.tenant_id, c.execution_id, c.owner_generation) FROM modeldev_close_intents c),
        'dispatches', (SELECT jsonb_agg(to_jsonb(d) ORDER BY d.tenant_id, d.execution_id) FROM modeldev_pipeline_dispatches d),
        'runs', (SELECT jsonb_agg(to_jsonb(r) ORDER BY r.tenant_id, r.execution_id, r.run_id) FROM modeldev_pipeline_confirmed_runs r)
    )::text`).Scan(&snapshot)
	if err != nil || snapshot == "" {
		t.Fatal("REVISION_UPGRADE_PREFLIGHT: independent history snapshot failed; behavior NOT_RUN")
	}
	return snapshot
}
