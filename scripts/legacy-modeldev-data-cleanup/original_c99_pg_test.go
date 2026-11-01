package main

import (
	"context"
	"encoding/json"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

// Restore only a protected capture of the original seven in an isolated real
// test schema. The native production qualifier remains a separate binary/module.
func originalC99ConsumerFixture(t *testing.T) (*cleanupFixture, originalC99Qualification, SourceProof) {
	t.Helper()
	private, binary, buildPath := os.Getenv("ORIGINAL_C99_PRIVATE_INPUT"), os.Getenv("ORIGINAL_C99_QUALIFIER_BINARY"), os.Getenv("ORIGINAL_C99_BUILD_RECEIPT")
	if private == "" || binary == "" || buildPath == "" {
		t.Skip("explicit protected original-seven/native qualifier dependencies not enabled")
	}
	raw, e := readBounded(private, 16<<20, true)
	if e != nil {
		t.Fatal("protected original-seven fixture unavailable")
	}
	var input struct {
		ModelDev map[string]json.RawMessage `json:"modeldev"`
	}
	if json.Unmarshal(raw, &input) != nil {
		t.Fatal("original-seven fixture invalid")
	}
	f := newCleanupFixture(t)
	f.command.Original = OriginalScope{TenantID: legacyTenant}
	for _, id := range originalC99Identities {
		f.command.Original.Executions = append(f.command.Original.Executions, id)
	}
	sort.Slice(f.command.Original.Executions, func(i, j int) bool {
		return f.command.Original.Executions[i].ExecutionID < f.command.Original.Executions[j].ExecutionID
	})
	ctx := context.Background()
	order := []string{"modeldev_execution_identities", "modeldev_executions", "modeldev_pipeline_dispatches", "modeldev_pipeline_confirmed_runs", "modeldev_run_authorities", "modeldev_close_intents", "modeldev_execution_runtimes", "modeldev_execution_cleanup_audits"}
	for _, name := range order {
		cols, e := f.md.Query(ctx, "SELECT column_name FROM information_schema.columns WHERE table_schema=$1 AND table_name=$2 AND is_generated='NEVER' ORDER BY ordinal_position", f.mdSchema, name)
		if e != nil {
			t.Fatal("original fixture column contract unavailable")
		}
		names, selects := []string{}, []string{}
		for cols.Next() {
			var col string
			if cols.Scan(&col) != nil {
				t.Fatal("original fixture column invalid")
			}
			q := pgx.Identifier{col}.Sanitize()
			names = append(names, q)
			selects = append(selects, "r."+q)
		}
		if cols.Err() != nil {
			t.Fatal("original fixture column scan failed")
		}
		cols.Close()
		var rows []row
		if json.Unmarshal(input.ModelDev[name], &rows) != nil {
			t.Fatal("original fixture rows invalid")
		}
		for _, r := range rows {
			if _, ok := originalC99Identities[text(r, "execution_id")]; !ok || text(r, "tenant_id") != legacyTenant {
				t.Fatal("original fixture outside exact seven")
			}
			b, _ := json.Marshal(r)
			table := quoted(f.mdSchema, name)
			if _, e = f.md.Exec(ctx, "INSERT INTO "+table+"("+strings.Join(names, ",")+") SELECT "+strings.Join(selects, ",")+" FROM json_populate_record(NULL::"+table+",$1::json) r", b); e != nil {
				t.Fatalf("isolated original table %s restore failed", name)
			}
		}
		switch name {
		case "modeldev_pipeline_dispatches":
			f.command.Original.OriginalDispatches = rows
		case "modeldev_pipeline_confirmed_runs":
			f.command.Original.OriginalRuns = rows
		case "modeldev_run_authorities":
			f.command.Original.OriginalAuthorities = rows
		}
	}
	u, e := url.Parse(os.Getenv("CPU_P01_TEST_DATABASE_ADMIN_URL"))
	if e != nil || u.Scheme == "" {
		t.Fatal("private test owner reference unavailable")
	}
	q := u.Query()
	q.Set("search_path", f.mdSchema)
	u.RawQuery = q.Encode()
	dsn := filepath.Join(t.TempDir(), "isolated-original-md.url")
	if os.WriteFile(dsn, []byte(u.String()), 0600) != nil {
		t.Fatal("private isolated owner reference unavailable")
	}
	command := exec.CommandContext(ctx, binary, "-dsn-file", dsn)
	out, e := command.Output()
	if e != nil {
		t.Fatal("native original-c99 production qualifier failed")
	}
	var qualification originalC99Qualification
	if strictJSON(out, &qualification) != nil {
		t.Fatal("native qualifier output invalid")
	}
	if len(qualification.Records) != 7 {
		t.Fatal("native qualifier scope differs")
	}
	for _, r := range qualification.Records {
		if r.Predicate != "ORIGINAL_C99_NATIVE_CLOSED_CLEANUP_ACK" || r.Guard != "" {
			t.Fatalf("native original %s blocked at %s", r.ExecutionID, r.Guard)
		}
	}
	buildRaw, e := readBounded(buildPath, 8<<20, false)
	if e != nil {
		t.Fatal("actual native build receipt unavailable")
	}
	return f, qualification, SourceProof{Kind: "ORIGINAL_C99_BUILD_RECEIPT", Path: buildPath, SHA256: bytesSHA(buildRaw)}
}

func TestOriginalC99NativeProofConsumerUsesExactLockedRows(t *testing.T) {
	f, q, build := originalC99ConsumerFixture(t)
	ctx := context.Background()
	proofPath := filepath.Join(t.TempDir(), "native-qualification.json")
	setProof := func(q originalC99Qualification) {
		t.Helper()
		raw, _ := json.Marshal(q)
		if os.WriteFile(proofPath, raw, 0600) != nil {
			t.Fatal("native proof file unavailable")
		}
		f.command.Root = &RootEvidence{OriginalC99Qualification: &SourceProof{Kind: "ORIGINAL_C99_NATIVE_QUALIFICATION", Path: proofPath, SHA256: bytesSHA(raw)}, OriginalC99Build: &build}
	}
	view := func() databaseView {
		t.Helper()
		tx, e := beginCleanup(ctx, f.md, false)
		if e != nil {
			t.Fatal(e)
		}
		defer rollbackCleanup(tx)
		v, e := snapshotDatabase(ctx, tx, f.command.Original, true)
		if e != nil {
			t.Fatal(e)
		}
		return v
	}
	setProof(q)
	v := view()
	for _, w := range f.command.Original.Executions {
		if reason := f.command.qualifyOriginalC99(w, v); reason != "" {
			t.Fatalf("actual native proof rejected %s: %s", w.ExecutionID, reason)
		}
	}
	tx, e := beginCleanup(ctx, f.md, false)
	if e != nil {
		t.Fatal(e)
	}
	if blockers := f.command.qualifyMD(ctx, tx, v); len(blockers) != 0 {
		t.Fatal("actual original seven qualification blocked", blockers)
	}
	rollbackCleanup(tx)
	target := f.command.Original.Executions[0]
	for _, kind := range []string{"missing", "wronghash", "outside-seven", "expired", "wrong-database", "blocked-native", "extra-table", "changed-row-proof", "wrong-build"} {
		t.Run(kind, func(t *testing.T) {
			raw, _ := json.Marshal(q)
			var bad originalC99Qualification
			_ = json.Unmarshal(raw, &bad)
			setProof(bad)
			switch kind {
			case "missing":
				f.command.Root.OriginalC99Qualification = nil
			case "wronghash":
				f.command.Root.OriginalC99Qualification.SHA256 = strings.Repeat("0", 64)
			case "outside-seven":
				bad.Records[0].ExecutionID = "11111111-1111-4111-8111-111111111111"
				setProof(bad)
			case "expired":
				bad.ObservedAt = time.Now().Add(-10 * time.Minute)
				bad.ExpiresAt = bad.ObservedAt.Add(5 * time.Minute)
				setProof(bad)
			case "wrong-database":
				bad.Database.DatabaseOID++
				setProof(bad)
			case "blocked-native":
				bad.Records[0].Predicate = ""
				bad.Records[0].Guard = "ORIGINAL_NOT_CLOSED"
				setProof(bad)
			case "extra-table":
				bad.Records[0].Rows["unapproved_table"] = []string{}
				setProof(bad)
			case "changed-row-proof":
				bad.Records[0].Rows["modeldev_executions"] = []string{strings.Repeat("1", 64)}
				setProof(bad)
			case "wrong-build":
				bad.QualifierBinarySHA256 = strings.Repeat("2", 64)
				setProof(bad)
			}
			if reason := f.command.qualifyOriginalC99(target, v); reason == "" {
				t.Fatal("invalid native qualification accepted", kind)
			}
		})
	}
	setProof(q)
	outside := target
	outside.ExecutionID = "11111111-1111-4111-8111-111111111111"
	if f.command.qualifyOriginalC99(outside, v) == "" {
		t.Fatal("non-original identity accepted")
	}
	// Use the same production locks/read-committed snapshot as Apply. An in-flight
	// real writer must drain before the retained native proof is rechecked.
	writer, e := f.md.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer rollbackCleanup(writer)
	if _, e = writer.Exec(ctx, `UPDATE modeldev_execution_identities SET owner_revision=owner_revision+1 WHERE tenant_id=$1::uuid AND execution_id=$2::uuid`, target.TenantID, target.ExecutionID); e != nil {
		t.Fatal("isolated owner revision update failed")
	}
	done := make(chan string, 1)
	go func() {
		locked, e := beginCleanup(ctx, f.md, true)
		if e != nil {
			done <- "TEST_LOCK_BEGIN_FAILED"
			return
		}
		defer rollbackCleanup(locked)
		if lockTables(ctx, locked, true) != nil {
			done <- "TEST_LOCK_FAILED"
			return
		}
		current, e := snapshotDatabase(ctx, locked, f.command.Original, true)
		if e != nil {
			done <- "TEST_SNAPSHOT_FAILED"
			return
		}
		done <- f.command.qualifyOriginalC99(target, current)
	}()
	waitFor(t, func() bool {
		var waiting bool
		_ = f.md.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_locks WHERE relation=to_regclass($1) AND mode='ShareRowExclusiveLock' AND NOT granted)`, quoted(f.mdSchema, "modeldev_execution_identities")).Scan(&waiting)
		return waiting
	})
	if e = writer.Commit(ctx); e != nil {
		t.Fatal("isolated owner revision commit failed")
	}
	if reason := <-done; reason != "MD_ORIGINAL_C99_AGGREGATE_ROW_DRIFT" {
		t.Fatal("original row drift accepted after lock drain", reason)
	}
}
