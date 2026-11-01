//go:build original_c99_pg

package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/testsupport/postgres"
)

func originalSevenFixture(t *testing.T) (context.Context, *pgxpool.Pool) {
	t.Helper()
	path := os.Getenv("ORIGINAL_C99_PRIVATE_INPUT")
	i, e := os.Lstat(path)
	if e != nil || !i.Mode().IsRegular() || i.Mode().Perm() != 0600 {
		t.Fatal("explicit protected original-seven fixture required")
	}
	raw, e := os.ReadFile(path)
	if e != nil {
		t.Fatal("original-seven fixture unavailable")
	}
	var input struct {
		ModelDev map[string]json.RawMessage `json:"modeldev"`
	}
	if json.Unmarshal(raw, &input) != nil {
		t.Fatal("original-seven fixture shape invalid")
	}
	// The unmodified native helper locates migrations from the working module.
	// This nested module deliberately has no copied schema. Bind the checkout's
	// byte-identical c99 baseline before asking the native helper to install it.
	t.Chdir("../../..")
	baseline, e := os.ReadFile("migrations/0001_current_baseline.up.sql")
	if e != nil || fmt.Sprintf("%x", sha256.Sum256(baseline)) != "9f5eda834b27e47c343a8eb1616d77e19d09b9cd3365a5ae579144df769dd4d5" {
		t.Fatal("native c99 baseline is unavailable or changed; behavior NOT_RUN")
	}
	open := postgres.Prepare(t)
	runtime := open()
	cfg, e := pgxpool.ParseConfig(os.Getenv("CPU_P01_TEST_DATABASE_ADMIN_URL"))
	if e != nil {
		t.Fatal("test migration reference unavailable")
	}
	cfg.ConnConfig.RuntimeParams = map[string]string{"search_path": runtime.Config().ConnConfig.RuntimeParams["search_path"]}
	ctx := context.Background()
	pool, e := pgxpool.NewWithConfig(ctx, cfg)
	if e != nil {
		t.Fatal("test migration pool unavailable")
	}
	t.Cleanup(pool.Close)
	var schema string
	if pool.QueryRow(ctx, "SELECT current_schema()").Scan(&schema) != nil {
		t.Fatal("test schema unavailable")
	}
	order := []string{"modeldev_execution_identities", "modeldev_executions", "modeldev_pipeline_dispatches", "modeldev_pipeline_confirmed_runs", "modeldev_run_authorities", "modeldev_close_intents", "modeldev_execution_runtimes", "modeldev_execution_cleanup_audits"}
	for _, name := range order {
		cols, e := pool.Query(ctx, "SELECT column_name FROM information_schema.columns WHERE table_schema=$1 AND table_name=$2 AND is_generated='NEVER' ORDER BY ordinal_position", schema, name)
		if e != nil {
			t.Fatal("fixture column contract unavailable")
		}
		names := []string{}
		selects := []string{}
		for cols.Next() {
			var col string
			if cols.Scan(&col) != nil {
				t.Fatal("fixture column invalid")
			}
			q := pgx.Identifier{col}.Sanitize()
			names = append(names, q)
			selects = append(selects, "r."+q)
		}
		cols.Close()
		var originalRows []map[string]json.RawMessage
		if json.Unmarshal(input.ModelDev[name], &originalRows) != nil {
			t.Fatal("original fixture rows invalid")
		}
		for _, r := range originalRows {
			b, e := json.Marshal(r)
			if e != nil {
				t.Fatal("fixture encoding invalid")
			}
			table := pgx.Identifier{schema, name}.Sanitize()
			sql := "INSERT INTO " + table + "(" + strings.Join(names, ",") + ") SELECT " + strings.Join(selects, ",") + " FROM json_populate_record(NULL::" + table + ",$1::json) r"
			if _, e = pool.Exec(ctx, sql, b); e != nil {
				t.Fatalf("original fixture table %s could not be restored: %s", name, e)
			}
		}
	}
	return ctx, pool
}
func TestOriginalSevenNativeC99QualificationAndMissingCloseEvidence(t *testing.T) {
	ctx, pool := originalSevenFixture(t)
	out, e := qualify(ctx, pool)
	if e != nil {
		t.Fatal(e)
	}
	if len(out.Records) != 7 {
		t.Fatal("original seven scope changed")
	}
	for _, r := range out.Records {
		if r.Predicate != "ORIGINAL_C99_NATIVE_CLOSED_CLEANUP_ACK" {
			t.Fatalf("original %s blocked at %s", r.ExecutionID, r.Guard)
		}
		t.Logf("original %s: native c99 full admission/runtime/CLOSED/cleanup/ACK PASS", r.ExecutionID)
	}
	// Canonical/spec validity alone cannot adopt a changed original actor/time,
	// a different immutable payload or an identity outside these exact seven.
	tx, e := pool.Begin(ctx)
	if e != nil {
		t.Fatal("isolated original read unavailable")
	}
	defer tx.Rollback(ctx)
	var schema string
	if tx.QueryRow(ctx, "SELECT current_schema()").Scan(&schema) != nil {
		t.Fatal("isolated schema unavailable")
	}
	rows, e := readRows(ctx, tx, schema, "modeldev_executions", originals[0].ExecutionID)
	if e != nil || len(rows) != 1 {
		t.Fatal("isolated original row unavailable")
	}
	for _, key := range []string{"actor", "accepted_at", "intent_canonical", "snapshot_canonical", "execution_id"} {
		copy := map[string]json.RawMessage{}
		for k, v := range rows[0] {
			copy[k] = append(json.RawMessage(nil), v...)
		}
		copy[key] = json.RawMessage(`"changed-original"`)
		if _, e := originalAdmission(copy, originals[0]); e == nil {
			t.Fatalf("changed original %s accepted", key)
		}
	}
	if e = tx.Rollback(ctx); e != nil {
		t.Fatal("isolated original read rollback failed")
	}
	// Remove the original close evidence only in the isolated fixture. The
	// original reader must reject it; no substitute close fact is produced.
	target := out.Records[0].ExecutionID
	_, e = pool.Exec(ctx, `UPDATE modeldev_execution_runtimes SET facts=jsonb_set(facts,'{CloseEvidence}','null'::jsonb) WHERE tenant_id=$1::uuid AND execution_id=$2::uuid`, originalTenant, target)
	if e != nil {
		t.Fatal("isolated close-evidence mutation failed")
	}
	bad, e := qualify(ctx, pool)
	if e != nil {
		t.Fatal(e)
	}
	for _, r := range bad.Records {
		if r.ExecutionID == target && r.Predicate != "" {
			t.Fatal("missing original close evidence was accepted")
		}
	}
	if out.ExpiresAt.Sub(out.ObservedAt) != 5*time.Minute {
		t.Fatal("native proof freshness changed")
	}
}
