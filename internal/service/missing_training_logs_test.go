package service

import (
	"reflect"
	"strings"
	"testing"

	modeldevv1 "github.com/zhangzhe-ctrl/ani-modeldev-service/api/ani/modeldev/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

func TestMissingTrainingLogsReviewTypedIdentityAndExitPresence(t *testing.T) {
	code := int32(143)
	original := &modeldevv1.MissingTrainingLogsReview{ExecutionId: "11111111-1111-4111-8111-111111111111", CreateOperationId: "22222222-2222-4222-8222-222222222222", CloseOperationId: "33333333-3333-4333-8333-333333333333", AutomaticCleanupPlanSha256: strings.Repeat("a", 64), MissingPods: []*modeldevv1.MissingTrainingLogPod{{PodUid: "44444444-4444-4444-8444-444444444444", PersistedExitCode: &code}}}
	value, err := missingTrainingLogsReview(original)
	if err != nil || value == nil || !proto.Equal(original, missingTrainingLogsReviewProto(value)) {
		t.Fatal("typed review lost original identities or exit", err)
	}
	for _, change := range []func(*modeldevv1.MissingTrainingLogsReview){
		func(r *modeldevv1.MissingTrainingLogsReview) { r.MissingPods[0].PersistedExitCode = nil },
		func(r *modeldevv1.MissingTrainingLogsReview) { r.CloseOperationId = r.CreateOperationId },
		func(r *modeldevv1.MissingTrainingLogsReview) { r.MissingPods = append(r.MissingPods, r.MissingPods[0]) },
		func(r *modeldevv1.MissingTrainingLogsReview) { r.ProtoReflect().SetUnknown([]byte{0xf8, 0x07, 0x01}) },
		func(r *modeldevv1.MissingTrainingLogsReview) {
			r.MissingPods[0].ProtoReflect().SetUnknown([]byte{0xf8, 0x07, 0x01})
		},
	} {
		invalid := proto.Clone(original).(*modeldevv1.MissingTrainingLogsReview)
		change(invalid)
		if _, err := missingTrainingLogsReview(invalid); status.Code(err) != codes.InvalidArgument {
			t.Fatal("malformed or incomplete reviewed source accepted", err)
		}
	}
	if value, err := missingTrainingLogsReview(nil); value != nil || err != nil || !reflect.DeepEqual(missingTrainingLogsReviewProto(nil), (*modeldevv1.MissingTrainingLogsReview)(nil)) {
		t.Fatal("legacy no-review request changed")
	}
}
