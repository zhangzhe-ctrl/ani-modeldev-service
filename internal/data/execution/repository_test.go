package execution_test

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/zhangzhe-ctrl/ani-modeldev-service/contract/cpup01"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/contract/cpup01/conformance"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/biz"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/data/execution"
)

func TestAcceptPersistsCompleteAdmissionAcrossNewConnections(t *testing.T) {
	openRuntimePool := preparePostgreSQL(t)
	writerPool := openRuntimePool()
	command := validAdmission(t)
	expectedIntent, _, err := cpup01.CanonicalIntent(command.Intent)
	if err != nil {
		t.Fatalf("invalid intent fixture: %v", err)
	}
	expectedSnapshot, err := command.Snapshot.Canonical()
	if err != nil {
		t.Fatalf("invalid snapshot fixture: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := execution.New(writerPool).Accept(ctx, command); err != nil {
		t.Fatalf("Accept rejected valid trusted admission: %v", err)
	}
	writerPool.Close()

	// A fresh repository and pool must recover committed data, not a process map
	// or an open connection's uncommitted transaction.
	readerPool := openRuntimePool()
	readContext, readCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer readCancel()
	got, err := execution.New(readerPool).Get(readContext, command.TenantID, command.ExecutionID)
	if err != nil {
		t.Fatalf("Get after reconnect: %v", err)
	}
	if got.TenantID != command.TenantID || got.Actor != command.Actor || got.OperationID != command.OperationID || got.ExecutionID != command.ExecutionID {
		t.Fatalf("persisted admission identity changed: got tenant=%q actor=%q operation=%q execution=%q", got.TenantID, got.Actor, got.OperationID, got.ExecutionID)
	}
	if got.IntentHash != command.IntentHash || got.SpecHash != command.SpecHash || !got.AcceptedAt.Equal(command.AcceptedAt) {
		t.Fatalf("persisted admission hashes or acceptance time changed")
	}
	actualIntent, _, err := cpup01.CanonicalIntent(got.Intent)
	if err != nil || string(actualIntent) != string(expectedIntent) {
		t.Fatalf("persisted intent is incomplete or changed: %v", err)
	}
	actualSnapshot, err := got.Snapshot.Canonical()
	if err != nil || string(actualSnapshot) != string(expectedSnapshot) {
		t.Fatalf("persisted frozen snapshot is incomplete or changed: %v", err)
	}
}

func TestAcceptDuplicateAfterReconnectReturnsOriginalAdmission(t *testing.T) {
	openRuntimePool := preparePostgreSQL(t)
	writerPool := openRuntimePool()
	command := validAdmission(t)
	// Governance owns actor identity. Its authenticated user IDs need not be
	// UUIDs, and the receiver must retain this immutable audit identity.
	command.Actor = "governance:user:42"
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := execution.New(writerPool).Accept(ctx, command); err != nil {
		t.Fatalf("initial Accept rejected valid trusted admission: %v", err)
	}
	writerPool.Close()

	retryPool := openRuntimePool()
	retryRepository := execution.New(retryPool)
	retryContext, retryCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer retryCancel()
	replayed, err := retryRepository.Accept(retryContext, command)
	if err != nil {
		t.Fatalf("duplicate Accept after reconnect must return the original admission: %v", err)
	}
	assertOriginalAdmission(t, replayed, command)
	persisted, err := retryRepository.Get(retryContext, command.TenantID, command.ExecutionID)
	if err != nil {
		t.Fatalf("Get after duplicate Accept: %v", err)
	}
	assertOriginalAdmission(t, persisted, command)
}

func assertOriginalAdmission(t *testing.T, got biz.Execution, want biz.Admission) {
	t.Helper()
	if got.TenantID != want.TenantID || got.Actor != want.Actor || got.OperationID != want.OperationID || got.ExecutionID != want.ExecutionID {
		t.Fatal("duplicate delivery changed the original admission identity")
	}
	if got.IntentHash != want.IntentHash || got.SpecHash != want.SpecHash || !got.AcceptedAt.Equal(want.AcceptedAt) {
		t.Fatal("duplicate delivery changed the original hashes or accepted_at")
	}
	expectedIntent, _, err := cpup01.CanonicalIntent(want.Intent)
	if err != nil {
		t.Fatalf("invalid expected intent: %v", err)
	}
	actualIntent, _, err := cpup01.CanonicalIntent(got.Intent)
	if err != nil || string(actualIntent) != string(expectedIntent) {
		t.Fatalf("duplicate delivery changed the complete original intent: %v", err)
	}
	expectedSnapshot, err := want.Snapshot.Canonical()
	if err != nil {
		t.Fatalf("invalid expected snapshot: %v", err)
	}
	actualSnapshot, err := got.Snapshot.Canonical()
	if err != nil || string(actualSnapshot) != string(expectedSnapshot) {
		t.Fatalf("duplicate delivery changed the complete original snapshot: %v", err)
	}
}

func validAdmission(t *testing.T) biz.Admission {
	t.Helper()
	snapshot := conformance.SnapshotV1()
	intent := cpup01.Intent{
		Name:             "durable-cpu-execution",
		Kind:             "GENERAL_TRAINING",
		PresetID:         snapshot.Release.PresetID,
		DatasetVersionID: snapshot.Input.InputVersionID,
	}
	_, intentHash, err := cpup01.CanonicalIntent(intent)
	if err != nil {
		t.Fatalf("intent fixture: %v", err)
	}
	specHash, err := snapshot.Digest()
	if err != nil {
		t.Fatalf("snapshot fixture: %v", err)
	}
	return biz.Admission{
		TenantID:    "11111111-2222-4333-8444-555555555555",
		Actor:       "governance:user:66666666-7777-4888-8999-aaaaaaaaaaaa",
		OperationID: "bbbbbbbb-cccc-4ddd-8eee-ffffffffffff",
		ExecutionID: "aaaaaaaa-bbbb-4ccc-8ddd-eeeeeeeeeeee",
		Intent:      intent,
		IntentHash:  intentHash,
		Snapshot:    snapshot,
		SpecHash:    specHash,
		AcceptedAt:  snapshot.DeadlineAt.Add(-time.Hour).UTC(),
	}
}

// This preflight and migration setup are deliberately distinct from the product
// assertion. Any failure here is a missing/broken real dependency, not a valid
// TDD RED. No connection URL or raw connection error is included in test output.
func preparePostgreSQL(t *testing.T) func() *pgxpool.Pool {
	t.Helper()
	runtimeURL := os.Getenv("CPU_P01_TEST_DATABASE_URL")
	adminURL := os.Getenv("CPU_P01_TEST_DATABASE_ADMIN_URL")
	if runtimeURL == "" || adminURL == "" {
		t.Fatal("CPU04_DB_PREFLIGHT: both protected test database references are required; behavior NOT_RUN")
	}
	runtimeConfig, err := pgxpool.ParseConfig(runtimeURL)
	if err != nil {
		t.Fatal("CPU04_DB_PREFLIGHT: runtime connection configuration invalid; behavior NOT_RUN")
	}
	adminConfig, err := pgxpool.ParseConfig(adminURL)
	if err != nil {
		t.Fatal("CPU04_DB_PREFLIGHT: migration connection configuration invalid; behavior NOT_RUN")
	}
	if runtimeConfig.ConnConfig.Host != adminConfig.ConnConfig.Host || runtimeConfig.ConnConfig.Port != adminConfig.ConnConfig.Port || runtimeConfig.ConnConfig.Database != adminConfig.ConnConfig.Database || runtimeConfig.ConnConfig.User == adminConfig.ConnConfig.User {
		t.Fatal("CPU04_DB_PREFLIGHT: separate runtime/migration roles on the same test database are required; behavior NOT_RUN")
	}
	runtimeConfig.MaxConns = 2
	adminConfig.MaxConns = 1
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	adminPool, err := pgxpool.NewWithConfig(ctx, adminConfig)
	if err != nil {
		t.Fatal("CPU04_DB_PREFLIGHT: migration pool creation failed; behavior NOT_RUN")
	}
	t.Cleanup(adminPool.Close)
	if err := adminPool.Ping(ctx); err != nil {
		t.Fatal("CPU04_DB_PREFLIGHT: migration database connection failed; behavior NOT_RUN")
	}
	preflightPool, err := pgxpool.NewWithConfig(ctx, runtimeConfig.Copy())
	if err != nil {
		t.Fatal("CPU04_DB_PREFLIGHT: runtime pool creation failed; behavior NOT_RUN")
	}
	defer preflightPool.Close()
	var superuser, bypassRLS bool
	if err := preflightPool.QueryRow(ctx, "SELECT rolsuper, rolbypassrls FROM pg_roles WHERE rolname = current_user").Scan(&superuser, &bypassRLS); err != nil {
		t.Fatal("CPU04_DB_PREFLIGHT: runtime role inspection failed; behavior NOT_RUN")
	}
	if superuser || bypassRLS {
		t.Fatal("CPU04_DB_PREFLIGHT: runtime role must be NOSUPERUSER NOBYPASSRLS; behavior NOT_RUN")
	}
	var random [12]byte
	if _, err := rand.Read(random[:]); err != nil {
		t.Fatal("CPU04_DB_PREFLIGHT: schema identifier allocation failed; behavior NOT_RUN")
	}
	schemaName := "cpu04_" + hex.EncodeToString(random[:])
	schemaSQL := pgx.Identifier{schemaName}.Sanitize()
	roleSQL := pgx.Identifier{runtimeConfig.ConnConfig.User}.Sanitize()
	if _, err := adminPool.Exec(ctx, "CREATE SCHEMA "+schemaSQL); err != nil {
		t.Fatal("CPU04_DB_PREFLIGHT: isolated schema creation failed; behavior NOT_RUN")
	}
	t.Cleanup(func() {
		cleanupContext, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cleanupCancel()
		if _, err := adminPool.Exec(cleanupContext, "DROP SCHEMA "+schemaSQL+" CASCADE"); err != nil {
			t.Errorf("CPU04_DB_CLEANUP: isolated schema cleanup failed")
		}
	})
	migrations, err := filepath.Glob(filepath.Join("..", "..", "..", "migrations", "*.up.sql"))
	if err != nil || len(migrations) == 0 {
		t.Fatal("CPU04_DB_PREFLIGHT: versioned execution migrations missing; behavior NOT_RUN")
	}
	sort.Strings(migrations)
	transaction, err := adminPool.Begin(ctx)
	if err != nil {
		t.Fatal("CPU04_DB_PREFLIGHT: migration transaction failed; behavior NOT_RUN")
	}
	defer transaction.Rollback(context.Background())
	if _, err := transaction.Exec(ctx, "SET LOCAL search_path TO "+schemaSQL); err != nil {
		t.Fatal("CPU04_DB_PREFLIGHT: migration schema binding failed; behavior NOT_RUN")
	}
	for _, migrationPath := range migrations {
		migration, err := os.ReadFile(migrationPath)
		if err != nil {
			t.Fatalf("CPU04_DB_PREFLIGHT: cannot read migration %s; behavior NOT_RUN", filepath.Base(migrationPath))
		}
		if _, err := transaction.Exec(ctx, string(migration)); err != nil {
			t.Fatalf("CPU04_DB_PREFLIGHT: migration %s failed; behavior NOT_RUN", filepath.Base(migrationPath))
		}
	}
	if err := transaction.Commit(ctx); err != nil {
		t.Fatal("CPU04_DB_PREFLIGHT: migration commit failed; behavior NOT_RUN")
	}
	if _, err := adminPool.Exec(ctx, "GRANT USAGE ON SCHEMA "+schemaSQL+" TO "+roleSQL); err != nil {
		t.Fatal("CPU04_DB_PREFLIGHT: runtime schema grant failed; behavior NOT_RUN")
	}
	if _, err := adminPool.Exec(ctx, "GRANT INSERT, SELECT ON "+schemaSQL+".modeldev_executions TO "+roleSQL); err != nil {
		t.Fatal("CPU04_DB_PREFLIGHT: runtime table grant failed; behavior NOT_RUN")
	}
	runtimeConfig.ConnConfig.RuntimeParams["search_path"] = schemaName
	t.Log("CPU04_DB_PREFLIGHT PASS: real PostgreSQL, restricted runtime role, isolated versioned schema")
	return func() *pgxpool.Pool {
		t.Helper()
		connectionContext, connectionCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer connectionCancel()
		pool, err := pgxpool.NewWithConfig(connectionContext, runtimeConfig.Copy())
		if err != nil {
			t.Fatal("CPU04_DB_PREFLIGHT: isolated runtime pool creation failed; behavior NOT_RUN")
		}
		t.Cleanup(pool.Close)
		if err := pool.Ping(connectionContext); err != nil {
			t.Fatal("CPU04_DB_PREFLIGHT: isolated runtime connection failed; behavior NOT_RUN")
		}
		return pool
	}
}
