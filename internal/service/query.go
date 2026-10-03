package service

import (
	"context"
	"errors"
	"sort"

	"github.com/google/uuid"
	modeldevv1 "github.com/zhangzhe-ctrl/ani-modeldev-service/api/ani/modeldev/v1"
	trainingv1 "github.com/zhangzhe-ctrl/ani-modeldev-service/api/ani/training/v1"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/biz"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type Query struct {
	modeldevv1.UnimplementedModelDevQueryServiceServer
	repository biz.ArtifactQueryRepository
	signer     biz.ArtifactSigner
}

func NewQuery(repository biz.ArtifactQueryRepository, signer biz.ArtifactSigner) *Query {
	return &Query{repository: repository, signer: signer}
}

type governanceQueryKey struct{}
type governanceQuery struct {
	delivery GovernanceDelivery
	method   string
}

// WithVerifiedGovernanceQuery is called by the mTLS receiver only after checking
// the BFF's per-request current authorization delegation for the exact method.
func WithVerifiedGovernanceQuery(ctx context.Context, delivery GovernanceDelivery, method string) context.Context {
	return context.WithValue(ctx, governanceQueryKey{}, governanceQuery{delivery: delivery, method: method})
}

func queryScope(ctx context.Context, method string) (GovernanceDelivery, error) {
	access, ok := ctx.Value(governanceQueryKey{}).(governanceQuery)
	if !ok || access.method != method {
		return GovernanceDelivery{}, status.Error(codes.Unauthenticated, "current query authorization required")
	}
	if err := ctx.Err(); err != nil {
		return GovernanceDelivery{}, status.FromContextError(err).Err()
	}
	return access.delivery, nil
}

func queryID(value string) bool {
	id, err := uuid.Parse(value)
	return err == nil && id != uuid.Nil && id.String() == value
}

func queryError(err error, requestID string) error {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return status.FromContextError(err).Err()
	}
	if errors.Is(err, biz.ErrExecutionNotFound) || errors.Is(err, biz.ErrArtifactNotFound) {
		return commandError(codes.NotFound, modeldevv1.ErrorReason_ERROR_REASON_RESOURCE_NOT_FOUND, "resource unavailable", requestID)
	}
	return commandError(codes.Unavailable, modeldevv1.ErrorReason_ERROR_REASON_UPSTREAM_UNAVAILABLE, "query unavailable", requestID)
}

func (query *Query) GetExecution(ctx context.Context, request *modeldevv1.GetExecutionRequest) (*modeldevv1.GetExecutionResponse, error) {
	scope, err := queryScope(ctx, modeldevv1.ModelDevQueryService_GetExecution_FullMethodName)
	if err != nil {
		return nil, err
	}
	if request == nil || !queryID(request.ExecutionId) || len(request.ProtoReflect().GetUnknown()) != 0 {
		return nil, status.Error(codes.InvalidArgument, "invalid execution query")
	}
	if query == nil || query.repository == nil {
		return nil, queryError(biz.ErrDownloadUnavailable, scope.RequestID)
	}
	record, err := query.repository.GetQueryRecord(ctx, scope.TenantID, request.ExecutionId)
	if err != nil {
		return nil, queryError(err, scope.RequestID)
	}
	value := record.Execution
	states, valid := encodeRuntimeStates(value.States)
	if !valid {
		return nil, queryError(biz.ErrPersistence, scope.RequestID)
	}
	view := &modeldevv1.ExecutionView{Identity: &trainingv1.ExecutionIdentity{OperationId: value.OperationID, ExecutionId: value.ExecutionID, ExecutionSpecHash: value.SpecHash}, Name: value.Intent.Name, Kind: trainingv1.ExecutionKind_EXECUTION_KIND_GENERAL_TRAINING, PresetId: value.Snapshot.Release.PresetID, ReleaseId: value.Snapshot.Release.ReleaseID, InputVersionId: value.Snapshot.Input.InputVersionID, ImageVersionId: value.Snapshot.Program.ImageVersionID, States: states, CloseGeneration: record.Runtime.CloseGeneration, AcceptedAt: timestamppb.New(value.AcceptedAt), DeadlineAt: timestamppb.New(value.Snapshot.DeadlineAt), ObservedAt: timestamppb.New(value.AcceptedAt)}
	if value.Close != nil {
		view.StopRequested = value.Close.Reason == biz.CloseReasonUserStop
	}
	if record.Runtime.Observation != nil {
		view.ObservedAt = timestamppb.New(record.Runtime.Observation.ObservedAt)
	}
	if record.Runtime.Workspace != nil {
		view.CurrentStep = modeldevv1.PipelineStep_PIPELINE_STEP_PREPARE
	}
	if record.Runtime.Training != nil {
		view.CurrentStep = modeldevv1.PipelineStep_PIPELINE_STEP_TRAIN_WAIT
	}
	if record.Runtime.Publication != nil {
		view.CurrentStep = modeldevv1.PipelineStep_PIPELINE_STEP_PUBLISH
		view.ObservedAt = timestamppb.New(record.Runtime.Publication.VerifiedAt)
	}
	if record.Runtime.ClosedAt != nil {
		view.CurrentStep = modeldevv1.PipelineStep_PIPELINE_STEP_CLOSE
		view.ObservedAt = timestamppb.New(*record.Runtime.ClosedAt)
	}
	return &modeldevv1.GetExecutionResponse{Execution: view}, nil
}

func artifactView(record biz.QueryRecord, file biz.PublishedRuntimeFile) *modeldevv1.ArtifactView {
	return &modeldevv1.ArtifactView{ArtifactId: file.ArtifactID, ExecutionId: record.Execution.ExecutionID, Filename: file.File.RelativePath, Role: trainingv1.FileRole(trainingv1.FileRole_value["FILE_ROLE_"+file.File.Role]), SizeBytes: file.File.SizeBytes, Sha256: file.File.SHA256, DeliveryState: modeldevv1.DeliveryState_DELIVERY_STATE_PUBLISHED, VerifiedAt: timestamppb.New(record.Runtime.Publication.VerifiedAt)}
}

func (query *Query) ListExecutionArtifacts(ctx context.Context, request *modeldevv1.ListExecutionArtifactsRequest) (*modeldevv1.ListExecutionArtifactsResponse, error) {
	scope, err := queryScope(ctx, modeldevv1.ModelDevQueryService_ListExecutionArtifacts_FullMethodName)
	if err != nil {
		return nil, err
	}
	if request == nil || !queryID(request.ExecutionId) || len(request.ProtoReflect().GetUnknown()) != 0 {
		return nil, status.Error(codes.InvalidArgument, "invalid artifact query")
	}
	size := uint32(50)
	cursor := ""
	if page := request.Page; page != nil {
		if len(page.ProtoReflect().GetUnknown()) != 0 || page.PageSize > 100 || (page.PageToken != "" && !queryID(page.PageToken)) {
			return nil, status.Error(codes.InvalidArgument, "invalid artifact page")
		}
		if page.PageSize > 0 {
			size = page.PageSize
		}
		cursor = page.PageToken
	}
	if query == nil || query.repository == nil {
		return nil, queryError(biz.ErrDownloadUnavailable, scope.RequestID)
	}
	record, err := query.repository.GetQueryRecord(ctx, scope.TenantID, request.ExecutionId)
	if err != nil {
		return nil, queryError(err, scope.RequestID)
	}
	response := &modeldevv1.ListExecutionArtifactsResponse{}
	if record.Runtime.Publication == nil {
		return response, nil
	}
	files := append([]biz.PublishedRuntimeFile(nil), record.Runtime.Publication.Files...)
	sort.Slice(files, func(i, j int) bool { return files[i].ArtifactID < files[j].ArtifactID })
	for _, file := range files {
		if file.ArtifactID <= cursor {
			continue
		}
		if len(response.Artifacts) == int(size) {
			response.NextPageToken = response.Artifacts[len(response.Artifacts)-1].ArtifactId
			break
		}
		response.Artifacts = append(response.Artifacts, artifactView(record, file))
	}
	return response, nil
}

func (query *Query) AuthorizeArtifactDownload(ctx context.Context, request *modeldevv1.AuthorizeArtifactDownloadRequest) (*modeldevv1.AuthorizeArtifactDownloadResponse, error) {
	scope, err := queryScope(ctx, modeldevv1.ModelDevQueryService_AuthorizeArtifactDownload_FullMethodName)
	if err != nil {
		return nil, err
	}
	if request == nil || !queryID(request.ArtifactId) || len(request.ProtoReflect().GetUnknown()) != 0 {
		return nil, status.Error(codes.InvalidArgument, "invalid artifact query")
	}
	if query == nil || query.repository == nil || query.signer == nil {
		return nil, queryError(biz.ErrDownloadUnavailable, scope.RequestID)
	}
	record, file, err := query.repository.FindPublishedArtifact(ctx, scope.TenantID, request.ArtifactId)
	if err != nil {
		return nil, queryError(err, scope.RequestID)
	}
	grant, err := query.signer.SignDownload(ctx, record.Execution.Snapshot.PublicationScope, file.Object)
	if err != nil {
		return nil, queryError(err, scope.RequestID)
	}
	return &modeldevv1.AuthorizeArtifactDownloadResponse{Artifact: artifactView(record, file), DownloadUrl: grant.URL, ExpiresAt: timestamppb.New(grant.ExpiresAt)}, nil
}
