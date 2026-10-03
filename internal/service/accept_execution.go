package service

import (
	"context"
	"errors"
	"strings"

	modeldevv1 "github.com/zhangzhe-ctrl/ani-modeldev-service/api/ani/modeldev/v1"
	trainingv1 "github.com/zhangzhe-ctrl/ani-modeldev-service/api/ani/training/v1"
	contractpb "github.com/zhangzhe-ctrl/ani-modeldev-service/contract/cpup01/protobuf"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/biz"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// AcceptExecution delivers Governance's already frozen command. It neither
// resolves a current Release nor authorizes a new external resource creation.
func (command *Command) AcceptExecution(ctx context.Context, request *modeldevv1.AcceptExecutionRequest) (*modeldevv1.AcceptExecutionResponse, error) {
	delivery, ok := ctx.Value(governanceDeliveryKey{}).(GovernanceDelivery)
	if !ok {
		return nil, commandError(codes.Unauthenticated, modeldevv1.ErrorReason_ERROR_REASON_UNAUTHENTICATED, "authenticated Governance delivery required", "")
	}
	if err := ctx.Err(); err != nil {
		return nil, status.FromContextError(err).Err()
	}
	invalid := func() (*modeldevv1.AcceptExecutionResponse, error) {
		return nil, commandError(codes.InvalidArgument, modeldevv1.ErrorReason_ERROR_REASON_INVALID_ARGUMENT, "invalid admission command", delivery.RequestID)
	}
	unavailable := func() (*modeldevv1.AcceptExecutionResponse, error) {
		return nil, commandError(codes.Unavailable, modeldevv1.ErrorReason_ERROR_REASON_UPSTREAM_UNAVAILABLE, "command persistence unavailable", delivery.RequestID)
	}
	if request == nil || request.Identity == nil || request.AcceptedAt == nil ||
		len(request.ProtoReflect().GetUnknown()) != 0 || len(request.Identity.ProtoReflect().GetUnknown()) != 0 ||
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
	snapshot, err := contractpb.DecodeSnapshot(request.Snapshot)
	if err != nil {
		return invalid()
	}
	admission := biz.Admission{
		TenantID: request.ResourceTenantId, Actor: request.AdmittedActorId,
		OperationID: request.Identity.OperationId, ExecutionID: request.Identity.ExecutionId,
		Intent: intent, IntentHash: request.IntentHash, Snapshot: snapshot,
		SpecHash: request.Identity.ExecutionSpecHash, AcceptedAt: acceptedAt,
	}
	if _, _, err := admission.CanonicalPayloads(); err != nil {
		return invalid()
	}
	if !strings.EqualFold(admission.TenantID, delivery.TenantID) || admission.Actor != delivery.Actor {
		return nil, commandError(codes.PermissionDenied, modeldevv1.ErrorReason_ERROR_REASON_FORBIDDEN, "command differs from authenticated delivery scope", delivery.RequestID)
	}
	// The validated body contains assertions only. Persist the authenticated
	// workload's scope after matching those assertions, never a body override.
	admission.TenantID, admission.Actor = delivery.TenantID, delivery.Actor
	if err := ctx.Err(); err != nil {
		return nil, status.FromContextError(err).Err()
	}
	if command == nil || command.repository == nil {
		return unavailable()
	}
	receipt, err := command.repository.Accept(ctx, admission)
	if err != nil {
		// Storage may expose a finite persistence error after its driver noticed
		// cancellation. Preserve the actual call context before mapping it.
		if contextErr := ctx.Err(); contextErr != nil {
			return nil, status.FromContextError(contextErr).Err()
		}
		switch {
		case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
			return nil, status.FromContextError(err).Err()
		case errors.Is(err, biz.ErrInvalidAdmission):
			return invalid()
		case errors.Is(err, biz.ErrAdmissionConflict):
			return nil, commandError(codes.AlreadyExists, modeldevv1.ErrorReason_ERROR_REASON_COMMAND_CONFLICT, "admission command conflicts with the original", delivery.RequestID)
		default:
			return unavailable()
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, status.FromContextError(err).Err()
	}
	states, valid := acceptExecutionStates(receipt.States)
	if receipt.OwnerRevision == 0 || !valid {
		return unavailable()
	}
	return &modeldevv1.AcceptExecutionResponse{
		Identity: &trainingv1.ExecutionIdentity{OperationId: receipt.OperationID, ExecutionId: receipt.ExecutionID, ExecutionSpecHash: receipt.SpecHash},
		Replayed: receipt.Replayed, States: states, Revision: receipt.OwnerRevision,
	}, nil
}

// This is only the transport mapping of the owner's same-transaction projection.
// Unknown or absent axes cannot be repaired into a successful default receipt.
func acceptExecutionStates(states biz.ExecutionStates) (*modeldevv1.ExecutionStates, bool) {
	return encodeRuntimeStates(states)
}
