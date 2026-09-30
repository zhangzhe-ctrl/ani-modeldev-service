package service

import (
	"context"

	modeldevv1 "github.com/zhangzhe-ctrl/ani-modeldev-service/api/ani/modeldev/v1"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/biz"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Command adapts the Governance delivery contract. Other capabilities remain
// explicitly unimplemented until their own durable receipts are available.
type Command struct {
	modeldevv1.UnimplementedModelDevCommandServiceServer
	repository biz.ExecutionRepository
}

func NewCommand(repository biz.ExecutionRepository) *Command {
	return &Command{repository: repository}
}

// ApplyCloseIntent deliberately fails until the real TLS/PostgreSQL behavior
// has a fixed-source RED. It does not acknowledge or write a command yet.
func (*Command) ApplyCloseIntent(context.Context, *modeldevv1.ApplyCloseIntentRequest) (*modeldevv1.ApplyCloseIntentResponse, error) {
	return nil, status.Error(codes.Unimplemented, "close command delivery is not implemented")
}
