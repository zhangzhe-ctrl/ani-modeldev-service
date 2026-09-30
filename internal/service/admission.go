package service

import (
	"context"
	"errors"

	modeldevv1 "github.com/zhangzhe-ctrl/ani-modeldev-service/api/ani/modeldev/v1"
	contractpb "github.com/zhangzhe-ctrl/ani-modeldev-service/contract/cpup01/protobuf"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/biz"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Admission adapts authenticated Governance requests to read-only candidate
// resolution. It does not accept executions or emit durable command receipts.
type Admission struct {
	modeldevv1.UnimplementedModelDevAdmissionServiceServer
	resolver *biz.AdmissionResolver
}

func NewAdmission(resolver *biz.AdmissionResolver) *Admission {
	return &Admission{resolver: resolver}
}

func (admission *Admission) ResolveAdmission(ctx context.Context, request *modeldevv1.ResolveAdmissionRequest) (*modeldevv1.ResolveAdmissionResponse, error) {
	delivery, ok := ctx.Value(governanceDeliveryKey{}).(GovernanceDelivery)
	if !ok {
		return nil, commandError(codes.Unauthenticated, modeldevv1.ErrorReason_ERROR_REASON_UNAUTHENTICATED, "authenticated Governance delivery required", "")
	}
	if err := ctx.Err(); err != nil {
		return nil, status.FromContextError(err).Err()
	}
	invalid := func() (*modeldevv1.ResolveAdmissionResponse, error) {
		return nil, commandError(codes.InvalidArgument, modeldevv1.ErrorReason_ERROR_REASON_INVALID_ARGUMENT, "invalid admission resolution request", delivery.RequestID)
	}
	if request == nil || request.Release == nil || request.AcceptedAt == nil ||
		len(request.ProtoReflect().GetUnknown()) != 0 || len(request.Release.ProtoReflect().GetUnknown()) != 0 ||
		len(request.AcceptedAt.ProtoReflect().GetUnknown()) != 0 || request.AcceptedAt.CheckValid() != nil {
		return invalid()
	}
	acceptedAt := request.AcceptedAt.AsTime()
	if acceptedAt.IsZero() || acceptedAt.Nanosecond()%1000 != 0 {
		return invalid()
	}
	intent, err := contractpb.DecodeIntent(request.Intent)
	if err != nil {
		return invalid()
	}
	resolved, err := admission.resolver.ResolveManaged(ctx, biz.AdmissionResolutionRequest{
		TenantID: delivery.TenantID, Intent: intent, AcceptedAt: acceptedAt,
		Release: biz.AdmissionReleaseSelection{
			ReleaseID: request.Release.ReleaseId, ReleaseDigest: request.Release.ReleaseDigest,
			BindingGeneration: request.Release.BindingGeneration,
		},
	})
	if err != nil {
		switch {
		case errors.Is(err, context.Canceled):
			return nil, status.FromContextError(context.Canceled).Err()
		case errors.Is(err, context.DeadlineExceeded):
			return nil, status.FromContextError(context.DeadlineExceeded).Err()
		case errors.Is(err, biz.ErrInvalidAdmission), errors.Is(err, biz.ErrInvalidInput):
			return invalid()
		case errors.Is(err, biz.ErrNoCompatibleRelease):
			return nil, commandError(codes.FailedPrecondition, modeldevv1.ErrorReason_ERROR_REASON_NO_COMPATIBLE_RELEASE, "selected Release is unavailable or incompatible", delivery.RequestID)
		case errors.Is(err, biz.ErrInputNotFound):
			return nil, commandError(codes.NotFound, modeldevv1.ErrorReason_ERROR_REASON_RESOURCE_NOT_FOUND, "input version unavailable", delivery.RequestID)
		case errors.Is(err, biz.ErrAdmissionInputNotReady):
			return nil, commandError(codes.FailedPrecondition, modeldevv1.ErrorReason_ERROR_REASON_INPUT_NOT_READY, "input version is not ready", delivery.RequestID)
		case errors.Is(err, biz.ErrAdmissionEnvironmentNotReady):
			return nil, commandError(codes.Unavailable, modeldevv1.ErrorReason_ERROR_REASON_ENVIRONMENT_NOT_READY, "managed admission environment is not ready", delivery.RequestID)
		default:
			return nil, commandError(codes.Unavailable, modeldevv1.ErrorReason_ERROR_REASON_UPSTREAM_UNAVAILABLE, "admission resolution unavailable", delivery.RequestID)
		}
	}
	snapshot, err := contractpb.EncodeSnapshot(resolved.Snapshot)
	if err != nil {
		return nil, commandError(codes.Unavailable, modeldevv1.ErrorReason_ERROR_REASON_UPSTREAM_UNAVAILABLE, "admission resolution unavailable", delivery.RequestID)
	}
	if err := ctx.Err(); err != nil {
		return nil, status.FromContextError(err).Err()
	}
	return &modeldevv1.ResolveAdmissionResponse{Snapshot: snapshot, ExecutionSpecHash: resolved.SpecHash}, nil
}
