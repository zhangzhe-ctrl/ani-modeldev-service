//go:build revisionupgrade

package postgres

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const revisionSeedSource = "a827457f999646394518c640d7c21915e1321496"

// RevisionUpgrade owns only the 0011-to-0012 migration test. Its old writer is
// built from the fixed historical commit by scripts/verify-revision-upgrade.
// The parent owns schema cleanup; the old process receives no migration role.
type RevisionUpgrade struct {
	schema   *schemaFixture
	binary   string
	seeded   bool
	upgraded bool
}

func PrepareRevisionUpgrade(t *testing.T) *RevisionUpgrade {
	t.Helper()
	directory := os.Getenv("CPU_P01_REVISION_SEED_DIRECTORY")
	if !filepath.IsAbs(directory) || filepath.Clean(directory) != directory {
		t.Fatal("REVISION_UPGRADE_PREFLIGHT: use make verify or scripts/verify-revision-upgrade; seed dependency missing; behavior NOT_RUN")
	}
	source, err := os.ReadFile(filepath.Join(directory, "source"))
	if err != nil || strings.TrimSpace(string(source)) != revisionSeedSource {
		t.Fatal("REVISION_UPGRADE_PREFLIGHT: fixed old source identity missing; behavior NOT_RUN")
	}
	binary := filepath.Join(directory, "old-writer.test")
	file, err := os.Open(binary)
	if err != nil {
		t.Fatal("REVISION_UPGRADE_PREFLIGHT: fixed old binary missing; behavior NOT_RUN")
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > 64<<20 {
		t.Fatal("REVISION_UPGRADE_PREFLIGHT: old binary size/type invalid; behavior NOT_RUN")
	}
	digest := sha256.New()
	if _, err := io.Copy(digest, file); err != nil {
		t.Fatal("REVISION_UPGRADE_PREFLIGHT: old binary unreadable; behavior NOT_RUN")
	}
	wantDigest, err := os.ReadFile(filepath.Join(directory, "binary.sha256"))
	gotDigest := hex.EncodeToString(digest.Sum(nil))
	if err != nil || strings.TrimSpace(string(wantDigest)) != gotDigest {
		t.Fatal("REVISION_UPGRADE_PREFLIGHT: old binary identity mismatch; behavior NOT_RUN")
	}
	// These are the historical files from S_seed, never the current repository's
	// mutable migration directory or a caller-selected version range.
	var migrations []string
	for _, name := range []string{
		"0001_execution.up.sql", "0002_execution_identity.up.sql", "0003_execution_close_intent.up.sql",
		"0004_input_import.up.sql", "0005_input_scope_credential_reference.up.sql", "0006_input_verification.up.sql",
		"0007_pipeline_dispatch.up.sql", "0008_input_validation_failure.up.sql", "0009_submission_uncertainty.up.sql",
		"0010_pipeline_confirmed_runs.up.sql", "0011_submission_not_sent.up.sql",
	} {
		migrations = append(migrations, filepath.Join(directory, "old-source", "migrations", name))
	}
	schema := prepareSchema(t)
	schema.install(t, migrations, false)
	t.Logf("REVISION_UPGRADE_PREFLIGHT: old_source=%s binary_sha256=%s", revisionSeedSource, gotDigest)
	return &RevisionUpgrade{schema: schema, binary: binary}
}

// SeedOldWriter waits for the old process to exit before handing its committed
// facts to the test. It neither runs current writers on old tables nor grants
// the child ownership of this schema's lifecycle.
func (fixture *RevisionUpgrade) SeedOldWriter(t *testing.T) []byte {
	t.Helper()
	if fixture.seeded || fixture.upgraded {
		t.Fatal("REVISION_UPGRADE_PREFLIGHT: old seed may run only once before migration; behavior NOT_RUN")
	}
	manifest := filepath.Join(t.TempDir(), "old-facts.json")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, fixture.binary, "-test.run=^TestSeedRevisionUpgradeOldWriter$", "-test.count=1", "-test.v", "-test.timeout=25s")
	command.WaitDelay = 2 * time.Second
	command.Env = []string{
		"CPU_P01_TEST_DATABASE_URL=" + fixture.schema.runtimeConfig.ConnString(),
		"CPU_P01_REVISION_SEED_SCHEMA=" + fixture.schema.schemaName,
		"CPU_P01_REVISION_SEED_MANIFEST=" + manifest,
	}
	output, err := command.CombinedOutput()
	// The fixed seed uses finite preflight messages and never logs connection
	// references or raw driver errors. Preserve those real phase/exit messages.
	t.Logf("old writer output:\n%s", output)
	if err != nil {
		t.Fatal("REVISION_UPGRADE_PREFLIGHT: old writer process failed or timed out; behavior NOT_RUN")
	}
	info, err := os.Stat(manifest)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0600 || info.Size() <= 0 || info.Size() > 1<<20 {
		t.Fatal("REVISION_UPGRADE_PREFLIGHT: private old manifest missing or invalid; behavior NOT_RUN")
	}
	raw, err := os.ReadFile(manifest)
	if err != nil {
		t.Fatal("REVISION_UPGRADE_PREFLIGHT: old manifest unreadable; behavior NOT_RUN")
	}
	fixture.seeded = true
	return raw
}

func (fixture *RevisionUpgrade) OpenRuntimePool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	return fixture.schema.openRuntimePool(t)
}

// ApplyOwnerRevision adds 0012 and precisely its extra mutable-column grant in
// one transaction. The test must first snapshot all old columns independently.
func (fixture *RevisionUpgrade) ApplyOwnerRevision(t *testing.T) {
	t.Helper()
	if !fixture.seeded || fixture.upgraded {
		t.Fatal("REVISION_UPGRADE_PREFLIGHT: migration requires one completed old seed; behavior NOT_RUN")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	migration, err := os.ReadFile(filepath.Join(repositoryRoot(t), "migrations", "0012_execution_owner_revision.up.sql"))
	if err != nil {
		t.Fatal("REVISION_UPGRADE_PREFLIGHT: owner revision migration missing; behavior NOT_RUN")
	}
	transaction := fixture.schema.migrationTransaction(t, ctx)
	defer rollbackMigration(transaction)
	if _, err := transaction.Exec(ctx, string(migration)); err != nil {
		t.Fatal("REVISION_UPGRADE_BEHAVIOR: valid historical facts rejected by owner revision migration")
	}
	if _, err := transaction.Exec(ctx, "GRANT UPDATE (owner_revision) ON "+fixture.schema.schemaSQL+".modeldev_execution_identities TO "+fixture.schema.roleSQL); err != nil {
		t.Fatal("REVISION_UPGRADE_PREFLIGHT: new runtime column grant failed; behavior NOT_RUN")
	}
	if err := transaction.Commit(ctx); err != nil {
		t.Fatal("REVISION_UPGRADE_BEHAVIOR: owner revision migration did not commit")
	}
	fixture.upgraded = true
}
