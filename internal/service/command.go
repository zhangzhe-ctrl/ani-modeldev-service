package service

import (
	"context"
	"errors"
	"strings"

	modeldevv1 "github.com/zhangzhe-ctrl/ani-modeldev-service/api/ani/modeldev/v1"
	trainingv1 "github.com/zhangzhe-ctrl/ani-modeldev-service/api/ani/training/v1"
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

type governanceDeliveryKey struct{}

// GovernanceDelivery is a command assertion from the current authenticated
// Governance workload, not proof of the historical actor's current permissions.
type GovernanceDelivery struct {
	TenantID, Actor, RequestID string
}

// WithVerifiedGovernanceDelivery is used only by the server's authenticated
// receiver, after certificate and metadata checks. Body fields cannot set it.
func WithVerifiedGovernanceDelivery(ctx context.Context, delivery GovernanceDelivery) context.Context {
	return context.WithValue(ctx, governanceDeliveryKey{}, delivery)
}

func (command *Command) ApplyCloseIntent(ctx context.Context, request *modeldevv1.ApplyCloseIntentRequest) (*modeldevv1.ApplyCloseIntentResponse, error) {
	delivery, ok := ctx.Value(governanceDeliveryKey{}).(GovernanceDelivery)
	if !ok {
		return nil, commandError(codes.Unauthenticated, modeldevv1.ErrorReason_ERROR_REASON_UNAUTHENTICATED, "authenticated Governance delivery required", "")
	}
	invalid := func() (*modeldevv1.ApplyCloseIntentResponse, error) {
		return nil, commandError(codes.InvalidArgument, modeldevv1.ErrorReason_ERROR_REASON_INVALID_ARGUMENT, "invalid close command", delivery.RequestID)
	}
	if request == nil || request.Identity == nil || request.RequestedAt == nil ||
		len(request.ProtoReflect().GetUnknown()) != 0 || len(request.Identity.ProtoReflect().GetUnknown()) != 0 ||
		len(request.RequestedAt.ProtoReflect().GetUnknown()) != 0 || request.RequestedAt.CheckValid() != nil ||
		request.Reason != modeldevv1.CloseReason_CLOSE_REASON_USER_STOP {
		return invalid()
	}
	intent := biz.CloseIntent{
		TenantID: request.ResourceTenantId, OperationID: request.Identity.OperationId,
		ExecutionID: request.Identity.ExecutionId, SpecHash: request.Identity.ExecutionSpecHash,
		SourceGeneration: request.IntentGeneration, Reason: biz.CloseReasonUserStop,
		RequestedAt: request.RequestedAt.AsTime(), RequestedActor: request.RequestedActorId,
	}
	if intent.Validate() != nil {
		return invalid()
	}
	if !strings.EqualFold(intent.TenantID, delivery.TenantID) || intent.RequestedActor != delivery.Actor {
		return nil, commandError(codes.PermissionDenied, modeldevv1.ErrorReason_ERROR_REASON_FORBIDDEN, "command differs from authenticated delivery scope", delivery.RequestID)
	}
	if command.repository == nil {
		return nil, commandError(codes.Unavailable, modeldevv1.ErrorReason_ERROR_REASON_UPSTREAM_UNAVAILABLE, "command persistence unavailable", delivery.RequestID)
	}
	receipt, err := command.repository.ApplyCloseIntent(ctx, intent)
	if err != nil {
		switch {
		case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
			return nil, status.FromContextError(err).Err()
		case errors.Is(err, biz.ErrInvalidAdmission):
			return invalid()
		case errors.Is(err, biz.ErrAdmissionConflict):
			return nil, commandError(codes.AlreadyExists, modeldevv1.ErrorReason_ERROR_REASON_COMMAND_CONFLICT, "close command conflicts with the original", delivery.RequestID)
		default:
			return nil, commandError(codes.Unavailable, modeldevv1.ErrorReason_ERROR_REASON_UPSTREAM_UNAVAILABLE, "command persistence unavailable", delivery.RequestID)
		}
	}
	return &modeldevv1.ApplyCloseIntentResponse{
		Identity:        &trainingv1.ExecutionIdentity{OperationId: receipt.OperationID, ExecutionId: receipt.ExecutionID, ExecutionSpecHash: receipt.SpecHash},
		CloseGeneration: receipt.Generation, CloseState: modeldevv1.CloseState_CLOSE_STATE_CLOSING,
		Replayed: receipt.Replayed, DurablyRecorded: true,
	}, nil
}

func commandError(code codes.Code, reason modeldevv1.ErrorReason, message, correlation string) error {
	failure := status.New(code, message)
	detailed, err := failure.WithDetails(&modeldevv1.ErrorDetail{Reason: reason, SafeMessage: message, CorrelationId: correlation})
	if err != nil {
		return failure.Err()
	}
	return detailed.Err()
}
