package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/component"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestStepFailureDiagnosticReportsOnlyStageClassAndStatus(t *testing.T) {
	const secret = "sensitive-value https://remote.example.test/object?X-Amz-Signature=signed-token owner-json jwt-value"
	for _, test := range []struct {
		name, stage, want string
		cause             error
	}{
		{name: "configuration", stage: "bootstrap", cause: fmt.Errorf("%s: %w", secret, component.ErrConfiguration), want: "managed component failed stage=bootstrap class=configuration code=Unknown"},
		{name: "RPC denied", stage: "run", cause: status.Error(codes.PermissionDenied, secret), want: "managed component failed stage=run class=grpc code=PermissionDenied"},
		{name: "RPC unavailable", stage: "run", cause: status.Error(codes.Unavailable, secret), want: "managed component failed stage=run class=grpc code=Unavailable"},
		{name: "unknown RPC code", stage: "run", cause: status.Error(codes.Code(4096), secret), want: "managed component failed stage=run class=grpc code=Unknown"},
		{name: "canceled", stage: "run", cause: fmt.Errorf("%s: %w", secret, context.Canceled), want: "managed component failed stage=run class=canceled code=Unknown"},
		{name: "deadline", stage: "run", cause: fmt.Errorf("%s: %w", secret, context.DeadlineExceeded), want: "managed component failed stage=run class=deadline code=Unknown"},
		{name: "local failure", stage: "storage-client", cause: errors.New(secret), want: "managed component failed stage=storage-client class=local code=Unknown"},
		{name: "invalid stage", stage: secret, cause: errors.New(secret), want: "managed component failed stage=unknown class=local code=Unknown"},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := stageFailure(test.stage, test.cause)
			if !errors.Is(err, test.cause) {
				t.Fatal("diagnostic wrapper lost the original failure identity")
			}
			line := failureDiagnostic(err)
			if line != test.want {
				t.Fatalf("unsafe or ambiguous failure output: %q", line)
			}
			for _, value := range []string{"sensitive-value", "https://", "X-Amz", "signed-token", "owner-json", "jwt-value"} {
				if strings.Contains(line, value) || strings.Contains(err.Error(), value) {
					t.Fatal("diagnostic leaked remote error or configuration")
				}
			}
		})
	}
}

func TestStepExecuteClassifiesBootstrapFailure(t *testing.T) {
	err := execute(context.Background(), []string{"unknown-step"})
	if !errors.Is(err, component.ErrConfiguration) || failureDiagnostic(err) != "managed component failed stage=bootstrap class=configuration code=Unknown" {
		t.Fatal("real CLI input failure lost its safe stage")
	}
}
