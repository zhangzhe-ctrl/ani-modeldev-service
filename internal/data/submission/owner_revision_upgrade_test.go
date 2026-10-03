//go:build revisionupgrade

package submission_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/biz"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/data/execution"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/data/submission"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/testsupport/postgres"
)

// Only the fixed historical process produces this private test protocol. Its
// old receipts are expectations, not inputs to a replacement repository.
type oldRevisionSeedManifest struct {
	WriterBase string                `json:"writer_base"`
	Cases      []oldRevisionSeedCase `json:"cases"`
}

type oldRevisionSeedCase struct {
	Name      string                      `json:"name"`
	Request   biz.PipelineDispatchRequest `json:"request"`
	Closes    []biz.CloseRecord            `json:"closes"`
	Permit    *biz.PipelineSendPermit      `json:"permit,omitempty"`
	Execution *biz.Execution               `json:"execution,omitempty"`
	Dispatch  *biz.PipelineDispatch        `json:"dispatch,omitempty"`
}

func TestOwnerRevisionUpgradeFromOldWriter(t *testing.T) {
	fixture := postgres.PrepareRevisionUpgrade(t)
	old := decodeOldRevisionSeeds(t, fixture.SeedOldWriter(t))
	beforePool := fixture.OpenRuntimePool(t)
	before := snapshotRevisionOldColumns(t, beforePool)
	beforePool.Close()
	// These independent row counts reject an empty/partial seed before any
	// migration behavior is assessed. The manifest alone is not persistence.
	for index, count := range []int{5, 4, 3, 2, 2} {
		if len(before[index].rows) != count {
			t.Fatal("REVISION_UPGRADE_PREFLIGHT: independent old rows do not contain all five cases; behavior NOT_RUN")
		}
	}
	t.Log("REVISION_UPGRADE_PREFLIGHT PASS: exited historical writer, five qualified cases, independent complete old-column byte snapshot")
	fixture.ApplyOwnerRevision(t)
	afterPool := fixture.OpenRuntimePool(t)
	after := snapshotRevisionOldColumns(t, afterPool)
	afterPool.Close()
	if !reflect.DeepEqual(after, before) {
		t.Fatal("REVISION_UPGRADE_BEHAVIOR: migration changed old column identity, bytes, timestamps, generations or related rows")
	}
	for _, seed := range old.Cases {
		t.Run(seed.Name, func(t *testing.T) {
			pool := fixture.OpenRuntimePool(t)
			writer, submissions := execution.New(pool), submission.New(pool)
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if seed.Execution == nil {
				got, err := writer.Get(ctx, seed.Request.Admission.TenantID, seed.Request.Admission.ExecutionID)
				if !errors.Is(err, biz.ErrExecutionNotFound) || !reflect.DeepEqual(got, biz.Execution{}) {
					t.Fatal("REVISION_UPGRADE_BEHAVIOR: migration invented a close-only admission")
				}
			} else {
				got, err := writer.Get(ctx, seed.Request.Admission.TenantID, seed.Request.Admission.ExecutionID)
				if err != nil {
					t.Fatal("REVISION_UPGRADE_BEHAVIOR: upgraded admission unreadable")
				}
				assertUpgradedExecution(t, got, *seed.Execution, 1)
				replay, err := writer.Accept(ctx, seed.Request.Admission)
				if err != nil || !replay.Replayed {
					t.Fatal("REVISION_UPGRADE_BEHAVIOR: original admission was not replayed")
				}
				assertUpgradedExecution(t, replay.Execution, *seed.Execution, 1)
			}
			for _, close := range seed.Closes {
				replay, err := writer.ApplyCloseIntent(ctx, close.CloseIntent)
				if err != nil || !replay.Replayed || replay.OwnerRevision != 1 || !reflect.DeepEqual(replay.CloseRecord, close) {
					t.Fatal("REVISION_UPGRADE_BEHAVIOR: old close changed original source/fence or incremented baseline")
				}
			}
			if seed.Dispatch != nil {
				got, err := submissions.Get(ctx, seed.Request.Admission.TenantID, seed.Request.Admission.ExecutionID)
				if err != nil {
					t.Fatal("REVISION_UPGRADE_BEHAVIOR: upgraded dispatch unreadable")
				}
				assertUpgradedDispatch(t, got, *seed.Dispatch, 1)
				replay, err := submissions.Reserve(ctx, seed.Request)
				if err != nil || replay.SendPermit != nil {
					t.Fatal("REVISION_UPGRADE_BEHAVIOR: old reservation failed or granted another send")
				}
				assertUpgradedDispatch(t, replay.Dispatch, *seed.Dispatch, 1)
				if seed.Dispatch.UncertainAt != nil {
					got, err := submissions.MarkSubmissionUncertain(ctx, *seed.Permit, *seed.Dispatch.UncertainAt)
					if err != nil {
						t.Fatal("REVISION_UPGRADE_BEHAVIOR: old uncertainty replay failed")
					}
					assertUpgradedDispatch(t, got, *seed.Dispatch, 1)
				}
				if seed.Dispatch.NotSentAt != nil {
					got, err := submissions.MarkSubmissionNotSent(ctx, *seed.Permit, *seed.Dispatch.NotSentAt)
					if err != nil {
						t.Fatal("REVISION_UPGRADE_BEHAVIOR: old no-send replay failed")
					}
					assertUpgradedDispatch(t, got, *seed.Dispatch, 1)
				}
				for _, run := range seed.Dispatch.ConfirmedRuns {
					got, err := submissions.RecordSubmissionConfirmed(ctx, *seed.Permit, confirmedObservation(run.RunID), run.FirstObservedAt)
					if err != nil || !got.ConflictingRuns {
						t.Fatal("REVISION_UPGRADE_BEHAVIOR: old conflicting Run replay failed")
					}
					assertUpgradedDispatch(t, got.Dispatch, *seed.Dispatch, 1)
				}
			}
			advanceUpgradedCase(t, ctx, writer, submissions, seed)
			pool.Close()
			// A fresh pool must observe revision 2, not a transaction-local receipt.
			reconnected := fixture.OpenRuntimePool(t)
			got, err := execution.New(reconnected).Get(ctx, seed.Request.Admission.TenantID, seed.Request.Admission.ExecutionID)
			if err != nil || got.OwnerRevision != 2 {
				t.Fatal("REVISION_UPGRADE_BEHAVIOR: next fact/revision did not survive reconnect")
			}
			if seed.Dispatch != nil || seed.Name == "admission-only" {
				dispatch, err := submission.New(reconnected).Get(ctx, seed.Request.Admission.TenantID, seed.Request.Admission.ExecutionID)
				if err != nil || dispatch.OwnerRevision != 2 {
					t.Fatal("REVISION_UPGRADE_BEHAVIOR: next dispatch revision did not survive reconnect")
				}
			}
		})
	}
}

func advanceUpgradedCase(t *testing.T, ctx context.Context, writer *execution.Repository, submissions *submission.Repository, seed oldRevisionSeedCase) {
	t.Helper()
	switch seed.Name {
	case "admission-only":
		first, err := submissions.Reserve(ctx, seed.Request)
		if err != nil || first.SendPermit == nil || first.Dispatch.OwnerRevision != 2 || first.Dispatch.State != biz.PipelineDispatchSubmitting {
			t.Fatal("REVISION_UPGRADE_BEHAVIOR: first reservation did not advance baseline once")
		}
		replay, err := submissions.Reserve(ctx, seed.Request)
		if err != nil || replay.SendPermit != nil || !reflect.DeepEqual(replay.Dispatch, first.Dispatch) {
			t.Fatal("REVISION_UPGRADE_BEHAVIOR: first post-upgrade reservation replay changed facts or send permission")
		}
	case "close-only":
		first, err := writer.Accept(ctx, seed.Request.Admission)
		if err != nil || first.Replayed || first.OwnerRevision != 2 || first.Close == nil || !reflect.DeepEqual(*first.Close, seed.Closes[0]) {
			t.Fatal("REVISION_UPGRADE_BEHAVIOR: delayed first admission lost old close or failed one revision advance")
		}
		replay, err := writer.Accept(ctx, seed.Request.Admission)
		if err != nil || !replay.Replayed || !reflect.DeepEqual(replay.Execution, first.Execution) {
			t.Fatal("REVISION_UPGRADE_BEHAVIOR: delayed admission replay changed facts or revision")
		}
	case "multiple-closes":
		intent := seed.Closes[1].CloseIntent
		intent.SourceGeneration++
		intent.RequestedAt = intent.RequestedAt.Add(time.Microsecond)
		first, err := writer.ApplyCloseIntent(ctx, intent)
		if err != nil || first.Replayed || first.OwnerRevision != 2 || first.Generation != 3 || !reflect.DeepEqual(first.CloseIntent, intent) {
			t.Fatal("REVISION_UPGRADE_BEHAVIOR: new source close confused old fence with revision baseline")
		}
		replay, err := writer.ApplyCloseIntent(ctx, intent)
		if err != nil || !replay.Replayed || replay.OwnerRevision != 2 || !reflect.DeepEqual(replay.CloseRecord, first.CloseRecord) {
			t.Fatal("REVISION_UPGRADE_BEHAVIOR: new close replay changed original facts or revision")
		}
	case "uncertain-and-not-sent", "confirmed-multiple-runs":
		observation := confirmedObservation("10000000-0000-4000-8000-000000000003")
		observedAt := seed.Dispatch.ReservedAt.Add(3 * time.Microsecond)
		first, err := submissions.RecordSubmissionConfirmed(ctx, *seed.Permit, observation, observedAt)
		want := *seed.Dispatch
		want.OwnerRevision, want.State = 2, biz.PipelineDispatchConfirmed
		want.ConfirmedRuns = append(append([]biz.PipelineConfirmedRun(nil), want.ConfirmedRuns...), biz.PipelineConfirmedRun{RunID: observation.RunID, FirstObservedAt: observedAt})
		if err != nil || first.ConflictingRuns != (len(want.ConfirmedRuns) > 1) || !reflect.DeepEqual(first.Dispatch, want) {
			t.Fatal("REVISION_UPGRADE_BEHAVIOR: new Run lost old attempt/observations or advanced baseline incorrectly")
		}
		replay, err := submissions.RecordSubmissionConfirmed(ctx, *seed.Permit, observation, observedAt)
		if err != nil || !reflect.DeepEqual(replay, first) {
			t.Fatal("REVISION_UPGRADE_BEHAVIOR: new Run replay changed committed facts or revision")
		}
	default:
		t.Fatal("REVISION_UPGRADE_PREFLIGHT: unknown old case; behavior NOT_RUN")
	}
}

func assertUpgradedExecution(t *testing.T, got, old biz.Execution, revision uint64) {
	t.Helper()
	old.OwnerRevision = revision
	if !reflect.DeepEqual(got, old) {
		t.Fatal("REVISION_UPGRADE_BEHAVIOR: complete original admission/close or expected owner revision changed")
	}
}

func assertUpgradedDispatch(t *testing.T, got, old biz.PipelineDispatch, revision uint64) {
	t.Helper()
	old.OwnerRevision = revision
	if !reflect.DeepEqual(got, old) {
		t.Fatal("REVISION_UPGRADE_BEHAVIOR: complete original dispatch/observations or expected owner revision changed")
	}
}

// These are copies of actual PostgreSQL wire values, including original bytea
// bytes. No JSON round trip, generated model, or repository decoder can hide a
// migration change. Metadata captures the exact old column order/types/formats.
type revisionOldTable struct {
	fields []pgconn.FieldDescription
	rows   [][][]byte
}

func snapshotRevisionOldColumns(t *testing.T, pool *pgxpool.Pool) []revisionOldTable {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	transaction, err := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		t.Fatal("REVISION_UPGRADE_READ: independent raw snapshot transaction unavailable")
	}
	defer func() {
		cleanup, cleanupCancel := context.WithTimeout(context.Background(), time.Second)
		defer cleanupCancel()
		_ = transaction.Rollback(cleanup)
	}()
	if _, err := transaction.Exec(ctx, "SET LOCAL TIME ZONE 'UTC'"); err != nil {
		t.Fatal("REVISION_UPGRADE_READ: independent timestamp representation unavailable")
	}
	var tables []revisionOldTable
	for _, query := range []string{
		`SELECT tenant_id, execution_id, operation_id, spec_hash, close_generation FROM modeldev_execution_identities ORDER BY tenant_id, execution_id`,
		`SELECT tenant_id, execution_id, operation_id, actor, intent_canonical, intent_hash, snapshot_canonical, spec_hash, accepted_at FROM modeldev_executions ORDER BY tenant_id, execution_id`,
		`SELECT tenant_id, execution_id, operation_id, spec_hash, source_kind, source_generation, owner_generation, reason, requested_at, requested_actor, close_state FROM modeldev_close_intents ORDER BY tenant_id, execution_id, source_kind, source_generation`,
		`SELECT tenant_id, execution_id, operation_id, spec_hash, attempt_id, plan_canonical, plan_hash, state, reserved_at, uncertain_at, not_sent_at FROM modeldev_pipeline_dispatches ORDER BY tenant_id, execution_id`,
		`SELECT tenant_id, execution_id, attempt_id, plan_hash, run_id, first_observed_at FROM modeldev_pipeline_confirmed_runs ORDER BY tenant_id, execution_id, attempt_id, run_id`,
	} {
		rows, err := transaction.Query(ctx, query)
		if err != nil {
			t.Fatal("REVISION_UPGRADE_READ: complete old-column query failed")
		}
		table := revisionOldTable{fields: append([]pgconn.FieldDescription(nil), rows.FieldDescriptions()...)}
		for rows.Next() {
			var row [][]byte
			for _, value := range rows.RawValues() {
				row = append(row, bytes.Clone(value))
			}
			table.rows = append(table.rows, row)
		}
		rows.Close()
		if rows.Err() != nil {
			t.Fatal("REVISION_UPGRADE_READ: complete old-column rows unavailable")
		}
		tables = append(tables, table)
	}
	if err := transaction.Commit(ctx); err != nil {
		t.Fatal("REVISION_UPGRADE_READ: independent raw snapshot did not complete")
	}
	return tables
}

func decodeOldRevisionSeeds(t *testing.T, raw []byte) oldRevisionSeedManifest {
	t.Helper()
	var manifest oldRevisionSeedManifest
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&manifest); err != nil || decoder.Decode(new(any)) != io.EOF || manifest.WriterBase != "bbf0ecbda123229a588c29445d132d4ee5c370a6" || len(manifest.Cases) != 5 {
		t.Fatal("REVISION_UPGRADE_PREFLIGHT: historical manifest is not the fixed complete seed; behavior NOT_RUN")
	}
	names := []string{"admission-only", "close-only", "multiple-closes", "uncertain-and-not-sent", "confirmed-multiple-runs"}
	identities := make(map[string]bool)
	for index, seed := range manifest.Cases {
		if seed.Name != names[index] || identities[seed.Request.Admission.ExecutionID] || identities[seed.Request.Admission.OperationID] {
			t.Fatal("REVISION_UPGRADE_PREFLIGHT: old case order/identity incomplete; behavior NOT_RUN")
		}
		identities[seed.Request.Admission.ExecutionID] = true
		identities[seed.Request.Admission.OperationID] = true
		if _, _, err := seed.Request.Admission.CanonicalPayloads(); err != nil {
			t.Fatal("REVISION_UPGRADE_PREFLIGHT: old original admission invalid; behavior NOT_RUN")
		}
		wantCloses := 0
		if seed.Name == "close-only" {
			wantCloses = 1
		} else if seed.Name == "multiple-closes" {
			wantCloses = 2
		}
		if len(seed.Closes) != wantCloses || (seed.Execution == nil) != (seed.Name == "close-only") {
			t.Fatal("REVISION_UPGRADE_PREFLIGHT: old close/admission cases incomplete; behavior NOT_RUN")
		}
		for closeIndex, close := range seed.Closes {
			want := dispatchCloseIntent(seed.Request)
			want.SourceGeneration = uint64(41 + closeIndex)
			want.RequestedAt = seed.Request.Admission.AcceptedAt.Add(time.Duration(closeIndex) * time.Microsecond)
			if !reflect.DeepEqual(close.CloseIntent, want) || close.Generation != uint64(closeIndex+1) || close.State != biz.CloseStateClosing {
				t.Fatal("REVISION_UPGRADE_PREFLIGHT: old source/owner close originals invalid; behavior NOT_RUN")
			}
		}
		if seed.Execution != nil && seed.Execution.OwnerRevision != 0 {
			t.Fatal("REVISION_UPGRADE_PREFLIGHT: seed unexpectedly used a revision writer; behavior NOT_RUN")
		}
		wantDispatch := index >= 3
		if (seed.Dispatch != nil) != wantDispatch || (seed.Permit != nil) != wantDispatch {
			t.Fatal("REVISION_UPGRADE_PREFLIGHT: old dispatch cases incomplete; behavior NOT_RUN")
		}
		if !wantDispatch {
			continue
		}
		if seed.Dispatch.OwnerRevision != 0 || seed.Dispatch.ReservedAt.IsZero() || (biz.PipelineCreateRequest{Admission: seed.Request.Admission, Plan: seed.Dispatch.Plan, Permit: *seed.Permit}).Validate() != nil {
			t.Fatal("REVISION_UPGRADE_PREFLIGHT: old reservation/permit binding invalid; behavior NOT_RUN")
		}
		firstTime := seed.Dispatch.ReservedAt.Add(time.Microsecond)
		secondTime := seed.Dispatch.ReservedAt.Add(2 * time.Microsecond)
		if seed.Name == "uncertain-and-not-sent" {
			if seed.Dispatch.State != biz.PipelineDispatchUncertain || seed.Dispatch.UncertainAt == nil || !seed.Dispatch.UncertainAt.Equal(firstTime) || seed.Dispatch.NotSentAt == nil || !seed.Dispatch.NotSentAt.Equal(secondTime) || len(seed.Dispatch.ConfirmedRuns) != 0 {
				t.Fatal("REVISION_UPGRADE_PREFLIGHT: old strong state/first observations incomplete; behavior NOT_RUN")
			}
		} else {
			wantRuns := []biz.PipelineConfirmedRun{{RunID: "10000000-0000-4000-8000-000000000001", FirstObservedAt: firstTime}, {RunID: "10000000-0000-4000-8000-000000000002", FirstObservedAt: secondTime}}
			if seed.Dispatch.State != biz.PipelineDispatchConfirmed || seed.Dispatch.UncertainAt != nil || seed.Dispatch.NotSentAt != nil || !reflect.DeepEqual(seed.Dispatch.ConfirmedRuns, wantRuns) {
				t.Fatal("REVISION_UPGRADE_PREFLIGHT: old conflicting Run originals incomplete; behavior NOT_RUN")
			}
		}
	}
	return manifest
}
