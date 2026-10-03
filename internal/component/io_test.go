package component_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	modeldevv1 "github.com/zhangzhe-ctrl/ani-modeldev-service/api/ani/modeldev/v1"
	trainingv1 "github.com/zhangzhe-ctrl/ani-modeldev-service/api/ani/training/v1"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/component"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

func TestComponentCloseMissingCandidatePreservesFailureAfterCloseAttempt(t *testing.T) {
	directory := t.TempDir()
	tokenFile := filepath.Join(directory, "token")
	if err := os.WriteFile(tokenFile, []byte("projected-close-token"), 0600); err != nil {
		t.Fatal(err)
	}
	client := &failedCloseRPC{t: t}
	config := component.Config{TenantID: "11111111-1111-4111-8111-111111111111", TokenFile: tokenFile, CandidateFile: filepath.Join(directory, "missing-candidate.json"), Context: &modeldevv1.StepContext{
		Identity: &trainingv1.ExecutionIdentity{OperationId: "33333333-3333-4333-8333-333333333333", ExecutionId: "22222222-2222-4222-8222-222222222222", ExecutionSpecHash: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
		Association: &modeldevv1.RunAssociation{KfpRunId: "44444444-4444-4444-8444-444444444444", NamespaceName: "cpu-execution", NamespaceUid: "55555555-5555-4555-8555-555555555555", WorkflowName: "main-flow", WorkflowUid: "66666666-6666-4666-8666-666666666666", PodName: "main-close", PodUid: "77777777-7777-4777-8777-777777777777"},
		Step: modeldevv1.PipelineStep_PIPELINE_STEP_CLOSE,
	}}
	runner, err := component.New(config, client, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	err = runner.Run(context.Background(), "close")
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing publication candidate must remain the original failure, not cleanup success or error: %v", err)
	}
	if client.closeCalls != 1 {
		t.Fatalf("FAILED_CLOSE_FENCE: missing publication candidate must still request owner close: calls=%d", client.closeCalls)
	}
}

// Only the transport boundary is replaced; the component reads a real missing
// file and must retain that failure even when its close request fails too.
type failedCloseRPC struct {
	modeldevv1.ModelDevStepServiceClient
	t *testing.T
	closeCalls int
}

func (client *failedCloseRPC) RequestExecutionClose(ctx context.Context, request *modeldevv1.RequestExecutionCloseRequest, _ ...grpc.CallOption) (*modeldevv1.RequestExecutionCloseResponse, error) {
	client.closeCalls++
	deadline, ok := ctx.Deadline()
	if !ok || ctx.Err() != nil || time.Until(deadline) <= 0 || time.Until(deadline) > 5*time.Second {
		client.t.Error("failure cleanup must have an active bounded five-second context")
	}
	md, _ := metadata.FromOutgoingContext(ctx)
	if len(md.Get("authorization")) != 1 || md.Get("authorization")[0] != "Bearer projected-close-token" || request.GetContext().GetStep() != modeldevv1.PipelineStep_PIPELINE_STEP_CLOSE || request.GetReason() != modeldevv1.CloseReason_CLOSE_REASON_STEP_FAILED {
		client.t.Error("failure cleanup must retain the authenticated close identity and STEP_FAILED reason")
	}
	return nil, status.Error(codes.Unavailable, "close observation unavailable")
}
