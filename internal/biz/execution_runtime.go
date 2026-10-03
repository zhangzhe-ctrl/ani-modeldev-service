package biz

import (
	"context"
	"errors"
	"time"

	"github.com/zhangzhe-ctrl/ani-modeldev-service/contract/cpup01"
)

var (
	ErrRuntimeConflict = errors.New("EXECUTION_RUNTIME_CONFLICT")
	ErrTrainingUncertain = errors.New("TRAINING_CREATION_UNCERTAIN")
	ErrTrainingUnavailable = errors.New("TRAINING_UNAVAILABLE")
	ErrTrainingNotFound = errors.New("TRAINING_NOT_FOUND")
	ErrRuntimeNotReady = errors.New("EXECUTION_RUNTIME_NOT_READY")
)

// WorkspaceBinding is an observed runtime fact, separate from the immutable
// execution snapshot. Its subpaths come exclusively from that snapshot.
type WorkspaceBinding struct {
	Mode string
	NamespaceName, NamespaceUID, PVCName, PVCUID string
	InputSubpath, TrainingSubpath, ReportsSubpath, PublicationSubpath string
	PreparedManifestSHA256 string
	PreparedManifestBytes int64
}

type TrainingPlan struct {
	TenantID, OperationID, ExecutionID, SpecHash string
	Name, RequestSHA256 string
	Snapshot cpup01.Snapshot
	Workspace WorkspaceBinding
}

type TrainingHandle struct {
	NamespaceUID, TrainJobUID, PVCUID string
}

type RuntimeResource struct {
	APIVersion, Kind, Namespace, Name, UID, OwnerUID string
	APIObjectPresent bool
	Terminal bool
	ExitCode *int32
}

// WritersAbsent requires verified current controller/child facts and all known
// historical Pods. Neither a missing object nor a Complete condition suffices.
type TrainingRuntimeObservation struct {
	Handle TrainingHandle
	Resources []RuntimeResource
	Outcome string // RUNNING, SUCCEEDED, FAILED, or UNKNOWN
	WritersAbsent bool
	ObservedAt time.Time
}

type TrainingReservation struct {
	State ExecutionRuntime
	SendPermit bool
}

type PublishedRuntimeFile struct {
	ArtifactID string
	File cpup01.OutputFile
	Object cpup01.FixedObjectRef
}

type UploadCompletion struct {
	RunID, WorkflowUID, TaskID, PodUID, ContainerName string
	CompletedAt, ObservedAt time.Time
}

type RuntimePublication struct {
	ID, LogicalKey, ReceiptID string
	Files []PublishedRuntimeFile
	Manifest cpup01.FixedObjectRef
	Bundle *cpup01.FixedObjectRef
	Upload UploadCompletion
	VerifiedAt time.Time
}

// ExecutionRuntime contains committed facts; no state value reconstructs an
// outbound creation permit. Normal step advancement belongs to KFP.
type ExecutionRuntime struct {
	Workspace *WorkspaceBinding
	Training *TrainingPlan
	TrainingHandle *TrainingHandle
	Observation *TrainingRuntimeObservation
	Publication *RuntimePublication
	CloseGeneration uint64
	CloseReason string
	CloseRequestedAt time.Time
	ClosedAt *time.Time
	OwnerRevision uint64
}

type ExecutionRuntimeRepository interface {
	GetRuntime(context.Context, string, string) (ExecutionRuntime, error)
	RecordPrepared(context.Context, RunAuthorityCandidate, WorkspaceBinding) (ExecutionRuntime, bool, error)
	ReserveTraining(context.Context, RunAuthorityCandidate) (TrainingReservation, error)
	RecordTrainingHandle(context.Context, RunAuthorityCandidate, TrainingHandle) (ExecutionRuntime, error)
	RecordTrainingObservation(context.Context, RunAuthorityCandidate, TrainingRuntimeObservation) (ExecutionRuntime, error)
	RecordPublication(context.Context, RunAuthorityCandidate, RuntimePublication) (ExecutionRuntime, bool, error)
	RequestRuntimeClose(context.Context, RunAuthorityCandidate, string) (ExecutionRuntime, bool, error)
	ConfirmRuntimeClosed(context.Context, RunAuthorityCandidate, uint64, TrainingRuntimeObservation) (ExecutionRuntime, error)
}

type TrainingRuntime interface {
	CreateTraining(context.Context, TrainingPlan) (TrainingHandle, error)
	FindTraining(context.Context, TrainingPlan) (TrainingHandle, error)
	ObserveTraining(context.Context, TrainingPlan, TrainingHandle, []RuntimeResource) (TrainingRuntimeObservation, error)
	StopTraining(context.Context, TrainingPlan, TrainingHandle) error
}

type PreparedWorkspaceVerifier interface {
	VerifyPrepared(context.Context, Execution, ManagedStepAssociation, WorkspaceBinding) error
}

type PublicationVerifier interface {
	VerifyPublication(context.Context, Execution, ManagedStepAssociation, RuntimePublication) (RuntimePublication, error)
}

func FreezeTrainingPlan(execution Execution, workspace WorkspaceBinding) (TrainingPlan, error) {
	return TrainingPlan{}, ErrRuntimeNotReady
}
