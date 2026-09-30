package input_test

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/biz"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/data/input"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/testsupport/postgres"
)

func TestInputValidationFailureSurvivesNewConnectionWithOriginalRequest(t *testing.T) {
	for _, code := range []biz.InputFailureCode{biz.InputFailureContentRejected, biz.InputFailureSourceUnavailable} {
		t.Run(string(code), func(t *testing.T) {
			openPool := postgres.Prepare(t)
			writer := openPool()
			request := importFixture()
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			repo := input.New(writer)
			if _, err := repo.FreezeImport(ctx, request); err != nil {
				t.Fatalf("freeze original request before failure observation: %v", err)
			}
			failure := biz.InputValidationFailure{Code: code, ObservedAt: request.RequestedAt.Add(time.Second)}
			failed, err := repo.RecordValidationFailure(ctx, request, failure)
			if err != nil {
				t.Fatalf("record finite verification failure after committed freeze: %v", err)
			}
			wantState := biz.InputStateRejected
			if code == biz.InputFailureSourceUnavailable {
				wantState = biz.InputStateValidating
			}
			if failed.State != wantState || failed.Verification != nil || failed.Failure == nil || *failed.Failure != failure || !reflect.DeepEqual(failed.Import, request) {
				t.Fatalf("failure lost fixed facts or retry classification: %+v", failed)
			}
			writer.Close()
			reader := input.New(openPool())
			recovered, err := reader.Get(ctx, request.TenantID, request.InputVersionID)
			if err != nil || !reflect.DeepEqual(recovered, failed) {
				t.Fatalf("new connection lost persisted failure: %+v, %v", recovered, err)
			}
			replayed, err := reader.FreezeImport(ctx, request)
			if err != nil || !reflect.DeepEqual(replayed, failed) {
				t.Fatalf("replay reset a failure or changed original object: %+v, %v", replayed, err)
			}
		})
	}
}
