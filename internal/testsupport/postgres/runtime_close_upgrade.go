package postgres

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PrepareRuntimeCloseUpgrade installs the actual pre-0015 schema. The caller
// seeds it through ordinary repositories before applying the single migration.
func PrepareRuntimeCloseUpgrade(t *testing.T) (func() *pgxpool.Pool, func()) {
	t.Helper()
	fixture := prepareSchema(t)
	paths, err := filepath.Glob(filepath.Join(repositoryRoot(t), "migrations", "*.up.sql"))
	if err != nil {
		t.Fatal("RUNTIME_UPGRADE_PREFLIGHT: migrations unavailable")
	}
	sort.Strings(paths)
	var old []string
	for _, path := range paths {
		if filepath.Base(path) <= "0014_execution_runtime.up.sql" {
			old = append(old, path)
		}
	}
	fixture.install(t, old, true)
	apply := func() {
		t.Helper()
		body, err := os.ReadFile(filepath.Join(repositoryRoot(t), "migrations", "0015_fenced_unbound_runtime.up.sql"))
		if err != nil {
			t.Fatal("RUNTIME_UPGRADE_PREFLIGHT: close migration unavailable")
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		tx, err := fixture.adminPool.BeginTx(ctx, pgx.TxOptions{})
		if err != nil {
			t.Fatal("RUNTIME_UPGRADE_PREFLIGHT: migration transaction unavailable")
		}
		defer tx.Rollback(ctx)
		if _, err := tx.Exec(ctx, "SET LOCAL search_path TO "+fixture.schemaSQL); err != nil {
			t.Fatal("RUNTIME_UPGRADE_PREFLIGHT: schema selection failed")
		}
		if _, err := tx.Exec(ctx, string(body)); err != nil {
			t.Fatal("RUNTIME_UPGRADE: populated schema migration failed")
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatal("RUNTIME_UPGRADE: commit failed")
		}
	}
	return func() *pgxpool.Pool { return fixture.openRuntimePool(t) }, apply
}
