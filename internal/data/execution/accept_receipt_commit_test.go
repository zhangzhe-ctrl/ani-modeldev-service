package execution_test

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/biz"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/data/execution"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/testsupport/postgres"
)

func TestAcceptReceiptCommitFailureReturnsZeroAndRetryIsFirstAcceptance(t *testing.T) {
	for _, earlierClose := range []bool{false, true} {
		name := "without earlier close"
		if earlierClose {
			name = "with earlier close"
		}
		t.Run(name, func(t *testing.T) {
			openRuntimePool := preparePostgreSQL(t)
			fixturePool := openRuntimePool()
			command, stop := validAdmission(t), userStopIntent(t)
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			var originalClose *biz.CloseRecord
			if earlierClose {
				closed, err := execution.New(fixturePool).ApplyCloseIntent(ctx, stop)
				if err != nil {
					t.Fatalf("earlier close fixture failed; admission commit behavior NOT_RUN: %v", err)
				}
				assertInitialCloseTombstone(t, closed.CloseRecord, stop)
				originalClose = &closed.CloseRecord
			}
			trace := &admissionCommitTrace{}
			config := fixturePool.Config()
			config.ConnConfig.Tracer = trace
			writerPool, err := pgxpool.NewWithConfig(ctx, config)
			if err != nil {
				t.Fatal("CPU04_DB_PREFLIGHT: observed admission writer pool unavailable; behavior NOT_RUN")
			}
			t.Cleanup(writerPool.Close)
			if err := writerPool.Ping(ctx); err != nil {
				t.Fatal("CPU04_DB_PREFLIGHT: observed admission writer connection unavailable; behavior NOT_RUN")
			}
			removeCommitFailure := postgres.RejectAdmissionCommit(t, writerPool)
			failed, err := execution.New(writerPool).Accept(ctx, command)
			assertEmptyAcceptReceiptFailure(t, failed, err, biz.ErrPersistence)
			if !trace.failed.Load() {
				t.Fatal("CPU04_DB_PREFLIGHT: the exact deferred admission COMMIT failure was not observed; commit-boundary behavior NOT_RUN")
			}
			t.Log("CPU04_ADMISSION_COMMIT_FAULT: real COMMIT reached the isolated deferred 23514 admission marker")
			writerPool.Close()
			fixturePool.Close()

			reconnected := execution.New(openRuntimePool())
			absent, err := reconnected.Get(ctx, command.TenantID, command.ExecutionID)
			assertEmptyFailure(t, absent, err, biz.ErrExecutionNotFound)
			persistedClose, err := reconnected.GetCloseIntent(ctx, stop.TenantID, stop.ExecutionID)
			if originalClose == nil {
				assertEmptyCloseFailure(t, persistedClose, err, biz.ErrExecutionNotFound)
			} else if err != nil || !reflect.DeepEqual(persistedClose, *originalClose) {
				t.Fatalf("failed Admission commit changed the earlier durable close: %v", err)
			}

			removeCommitFailure()
			first, err := reconnected.Accept(ctx, command)
			if err != nil || first.Replayed {
				t.Fatalf("retry after failed COMMIT must remain the first Admission: %v", err)
			}
			assertOriginalAdmission(t, first.Execution, command)
			if !reflect.DeepEqual(first.Close, originalClose) {
				t.Fatal("successful Admission retry changed the original close facts")
			}
			reader := execution.New(openRuntimePool())
			stored, err := reader.Get(ctx, command.TenantID, command.ExecutionID)
			if err != nil || !reflect.DeepEqual(stored, first.Execution) {
				t.Fatalf("successful retry was not independently durable: %v", err)
			}
			replayed, err := reader.Accept(ctx, command)
			if err != nil || !replayed.Replayed || !reflect.DeepEqual(replayed.Execution, stored) {
				t.Fatalf("delivery after the first successful commit must replay its original facts: %v", err)
			}
		})
	}
}

// Trace only a boolean for the exact deferred failure at COMMIT. Do not retain
// SQL arguments or driver errors; business assertions use repository methods.
type admissionCommitTrace struct {
	failed atomic.Bool
}

type admissionCommitTraceKey struct{}

func (*admissionCommitTrace) TraceQueryStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	return context.WithValue(ctx, admissionCommitTraceKey{}, strings.EqualFold(strings.TrimSpace(data.SQL), "commit"))
}

func (trace *admissionCommitTrace) TraceQueryEnd(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryEndData) {
	commit, _ := ctx.Value(admissionCommitTraceKey{}).(bool)
	var databaseError *pgconn.PgError
	if commit && errors.As(data.Err, &databaseError) && databaseError.Code == "23514" && databaseError.Message == "injected_admission_commit_failure" {
		trace.failed.Store(true)
	}
}
