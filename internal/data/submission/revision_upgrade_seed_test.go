//go:build revisionseed

// Compile this file at the fixed S_seed checkpoint while the actual writer,
// SQL, and migrations are still the bbf0ecb versions. No fixture schema is
// created, migrated, or dropped by this child process.
package submission_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/biz"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/data/execution"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/data/submission"
)

// This identifies the old writer contract, not the future harness commit.
// The runner must separately record the exact S_seed SHA and binary SHA256.
const revisionSeedWriterBase = "bbf0ecbda123229a588c29445d132d4ee5c370a6"

type revisionSeedManifest struct {
	WriterBase string             `json:"writer_base"`
	Cases      []revisionSeedCase `json:"cases"`
}

// This is private cross-process test material, never a service API or credential
// file. The request preserves the originally authored input; the other fields
// preserve facts actually returned by old repositories after successful commit.
type revisionSeedCase struct {
	Name       string                      `json:"name"`
	Request    biz.PipelineDispatchRequest `json:"request"`
	Closes     []biz.CloseRecord            `json:"closes"`
	Permit     *biz.PipelineSendPermit      `json:"permit,omitempty"`
	Execution  *biz.Execution               `json:"execution,omitempty"`
	Dispatch   *biz.PipelineDispatch        `json:"dispatch,omitempty"`
}

func TestSeedRevisionUpgradeOldWriter(t *testing.T) {
	// Missing inputs are an explicit dependency failure, never a skipped lane.
	pool := revisionSeedRuntimePool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	writer, submissions := execution.New(pool), submission.New(pool)
	manifest := revisionSeedManifest{WriterBase: revisionSeedWriterBase}
	for _, name := range []string{"admission-only", "close-only", "multiple-closes", "uncertain-and-not-sent", "confirmed-multiple-runs"} {
		request := validDispatchRequest(t)
		request.Admission.OperationID = uuid.NewString()
		request.Admission.ExecutionID = uuid.NewString()
		seed := revisionSeedCase{Name: name, Request: request}
		if name != "close-only" {
			receipt, err := writer.Accept(ctx, request.Admission)
			if err != nil || receipt.Replayed {
				t.Fatal("REVISION_SEED_PREFLIGHT: real old Accept failed; upgrade behavior NOT_RUN")
			}
		}
		if name == "close-only" || name == "multiple-closes" {
			intent := dispatchCloseIntent(request)
			// Deliberately differ from owner fence 1/2. Migration must not
			// turn source generation or close fence into owner revision.
			intent.SourceGeneration = 41
			count := 1
			if name == "multiple-closes" {
				count = 2
			}
			for index := 0; index < count; index++ {
				intent.SourceGeneration = uint64(41 + index)
				intent.RequestedAt = request.Admission.AcceptedAt.Add(time.Duration(index) * time.Microsecond)
				receipt, err := writer.ApplyCloseIntent(ctx, intent)
				if err != nil || receipt.Replayed || receipt.Generation != uint64(index+1) {
					t.Fatal("REVISION_SEED_PREFLIGHT: real old close failed; upgrade behavior NOT_RUN")
				}
				seed.Closes = append(seed.Closes, receipt.CloseRecord)
			}
		}
		if name == "uncertain-and-not-sent" || name == "confirmed-multiple-runs" {
			reservation, err := submissions.Reserve(ctx, request)
			if err != nil || reservation.SendPermit == nil || reservation.Dispatch.State != biz.PipelineDispatchSubmitting {
				t.Fatal("REVISION_SEED_PREFLIGHT: real old reservation failed; upgrade behavior NOT_RUN")
			}
			permit := *reservation.SendPermit
			seed.Permit = &permit
			observedAt := reservation.Dispatch.ReservedAt.Add(time.Microsecond)
			if name == "uncertain-and-not-sent" {
				uncertain, err := submissions.MarkSubmissionUncertain(ctx, permit, observedAt)
				if err != nil || uncertain.State != biz.PipelineDispatchUncertain || uncertain.UncertainAt == nil || !uncertain.UncertainAt.Equal(observedAt) {
					t.Fatal("REVISION_SEED_PREFLIGHT: real old uncertain observation failed; upgrade behavior NOT_RUN")
				}
				notSentAt := observedAt.Add(time.Microsecond)
				notSent, err := submissions.MarkSubmissionNotSent(ctx, permit, notSentAt)
				if err != nil || notSent.State != biz.PipelineDispatchUncertain || notSent.NotSentAt == nil || !notSent.NotSentAt.Equal(notSentAt) || notSent.UncertainAt == nil || !notSent.UncertainAt.Equal(observedAt) {
					t.Fatal("REVISION_SEED_PREFLIGHT: real old not-sent fact failed; upgrade behavior NOT_RUN")
				}
			} else {
				for index, runID := range []string{"10000000-0000-4000-8000-000000000001", "10000000-0000-4000-8000-000000000002"} {
					receipt, err := submissions.RecordSubmissionConfirmed(ctx, permit, confirmedObservation(runID), observedAt.Add(time.Duration(index)*time.Microsecond))
					if err != nil || receipt.Dispatch.State != biz.PipelineDispatchConfirmed || len(receipt.Dispatch.ConfirmedRuns) != index+1 || receipt.ConflictingRuns != (index == 1) {
						t.Fatal("REVISION_SEED_PREFLIGHT: real old Run observations failed; upgrade behavior NOT_RUN")
					}
				}
			}
			dispatch, err := submissions.Get(ctx, request.Admission.TenantID, request.Admission.ExecutionID)
			if err != nil {
				t.Fatal("REVISION_SEED_PREFLIGHT: real old dispatch read failed; upgrade behavior NOT_RUN")
			}
			seed.Dispatch = &dispatch
		}
		admitted, err := writer.Get(ctx, request.Admission.TenantID, request.Admission.ExecutionID)
		if name == "close-only" {
			if !errors.Is(err, biz.ErrExecutionNotFound) {
				t.Fatal("REVISION_SEED_PREFLIGHT: close-only seed has unexpected admission; upgrade behavior NOT_RUN")
			}
		} else {
			if err != nil {
				t.Fatal("REVISION_SEED_PREFLIGHT: real old admission read failed; upgrade behavior NOT_RUN")
			}
			seed.Execution = &admitted
		}
		manifest.Cases = append(manifest.Cases, seed)
	}
	// Closing the old pool before the manifest is returned makes migration
	// ordering explicit. Parent still waits for this entire process to exit.
	pool.Close()
	path := os.Getenv("CPU_P01_REVISION_SEED_MANIFEST")
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		t.Fatal("REVISION_SEED_PREFLIGHT: private manifest path required; upgrade behavior NOT_RUN")
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		t.Fatal("REVISION_SEED_PREFLIGHT: private manifest creation failed; upgrade behavior NOT_RUN")
	}
	encodeErr := json.NewEncoder(file).Encode(manifest)
	closeErr := file.Close()
	if encodeErr != nil || closeErr != nil {
		t.Fatal("REVISION_SEED_PREFLIGHT: manifest write failed; upgrade behavior NOT_RUN")
	}
	t.Log("REVISION_SEED_PREFLIGHT PASS: five old-writer cases committed; pool closed; parent owns schema")
}

func revisionSeedRuntimePool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	schema := os.Getenv("CPU_P01_REVISION_SEED_SCHEMA")
	if !regexp.MustCompile(`^cpu04_[0-9a-f]{24}$`).MatchString(schema) || os.Getenv("CPU_P01_TEST_DATABASE_ADMIN_URL") != "" {
		t.Fatal("REVISION_SEED_PREFLIGHT: parent-owned schema and runtime-only child required; upgrade behavior NOT_RUN")
	}
	runtimeURL := os.Getenv("CPU_P01_TEST_DATABASE_URL")
	if runtimeURL == "" {
		t.Fatal("REVISION_SEED_PREFLIGHT: protected runtime reference required; upgrade behavior NOT_RUN")
	}
	config, err := pgxpool.ParseConfig(runtimeURL)
	if err != nil {
		t.Fatal("REVISION_SEED_PREFLIGHT: runtime reference invalid; upgrade behavior NOT_RUN")
	}
	config.MaxConns = 1
	config.ConnConfig.RuntimeParams["search_path"] = schema
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal("REVISION_SEED_PREFLIGHT: runtime pool unavailable; upgrade behavior NOT_RUN")
	}
	t.Cleanup(pool.Close)
	var schemaMatches, ownsSchema, superuser, bypassRLS, canCreate bool
	err = pool.QueryRow(ctx, `SELECT current_schema() = $1, n.nspowner = r.oid,
        r.rolsuper, r.rolbypassrls, has_schema_privilege(current_user, n.oid, 'CREATE')
        FROM pg_namespace n JOIN pg_roles r ON r.rolname = current_user WHERE n.nspname = $1`, schema).
		Scan(&schemaMatches, &ownsSchema, &superuser, &bypassRLS, &canCreate)
	if err != nil || !schemaMatches || ownsSchema || superuser || bypassRLS || canCreate {
		t.Fatal("REVISION_SEED_PREFLIGHT: restricted runtime/schema checks failed; upgrade behavior NOT_RUN")
	}
	var hasRevision bool
	err = pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM information_schema.columns
        WHERE table_schema = $1 AND table_name = 'modeldev_execution_identities' AND column_name = 'owner_revision')`, schema).Scan(&hasRevision)
	if err != nil || hasRevision {
		t.Fatal("REVISION_SEED_PREFLIGHT: expected pre-0012 schema; upgrade behavior NOT_RUN")
	}
	var empty bool
	err = pool.QueryRow(ctx, `SELECT
        NOT EXISTS (SELECT 1 FROM modeldev_execution_identities)
        AND NOT EXISTS (SELECT 1 FROM modeldev_executions)
        AND NOT EXISTS (SELECT 1 FROM modeldev_close_intents)
        AND NOT EXISTS (SELECT 1 FROM modeldev_pipeline_dispatches)
        AND NOT EXISTS (SELECT 1 FROM modeldev_pipeline_confirmed_runs)`).Scan(&empty)
	if err != nil || !empty {
		t.Fatal("REVISION_SEED_PREFLIGHT: old schema must be complete and empty; upgrade behavior NOT_RUN")
	}
	return pool
}
