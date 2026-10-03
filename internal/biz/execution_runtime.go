package biz

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/zhangzhe-ctrl/ani-modeldev-service/contract/cpup01"
)

var (
	ErrRuntimeConflict     = errors.New("EXECUTION_RUNTIME_CONFLICT")
	ErrTrainingUncertain   = errors.New("TRAINING_CREATION_UNCERTAIN")
	ErrTrainingUnavailable = errors.New("TRAINING_UNAVAILABLE")
	ErrTrainingNotFound    = errors.New("TRAINING_NOT_FOUND")
	ErrRuntimeNotReady     = errors.New("EXECUTION_RUNTIME_NOT_READY")
)

// WorkspaceBinding is an observed runtime fact, separate from the immutable
// execution snapshot. Its subpaths come exclusively from that snapshot.
type WorkspaceBinding struct {
	Mode                                                              string
	NamespaceName, NamespaceUID, PVCName, PVCUID                      string
	InputSubpath, TrainingSubpath, ReportsSubpath, PublicationSubpath string
	PreparedManifestSHA256                                            string
	PreparedManifestBytes                                             int64
}

type TrainingPlan struct {
	TenantID, OperationID, ExecutionID, SpecHash string
	Name, RequestSHA256                          string
	Snapshot                                     cpup01.Snapshot
	Workspace                                    WorkspaceBinding
}

type TrainingHandle struct {
	NamespaceUID, TrainJobUID, PVCUID string
}

type RuntimeResource struct {
	APIVersion, Kind, Namespace, Name, UID, OwnerUID string
	APIObjectPresent                                 bool
	CreationDisabled                                 bool
	Terminal                                         bool
	ExitCode                                         *int32
}

// WritersAbsent requires verified current controller/child facts and all known
// historical Pods. Neither a missing object nor a Complete condition suffices.
type TrainingRuntimeObservation struct {
	Handle        TrainingHandle
	Resources     []RuntimeResource
	Outcome       string // RUNNING, SUCCEEDED, FAILED, or UNKNOWN
	WritersAbsent bool
	ObservedAt    time.Time
}

type TrainingReservation struct {
	State      ExecutionRuntime
	SendPermit bool
}

type PublishedRuntimeFile struct {
	ArtifactID string
	File       cpup01.OutputFile
	Object     cpup01.FixedObjectRef
}

type UploadCompletion struct {
	RunID, WorkflowUID, TaskID, PodUID, ContainerName string
	CompletedAt, ObservedAt                           time.Time
}

type RuntimePublication struct {
	ID, LogicalKey, ReceiptID string
	Files                     []PublishedRuntimeFile
	Manifest                  cpup01.FixedObjectRef
	Bundle                    *cpup01.FixedObjectRef
	Upload                    UploadCompletion
	VerifiedAt                time.Time
}

// ExecutionRuntime contains committed facts; no state value reconstructs an
// outbound creation permit. Normal step advancement belongs to KFP.
type ExecutionRuntime struct {
	Workspace        *WorkspaceBinding
	Training         *TrainingPlan
	TrainingHandle   *TrainingHandle
	Observation      *TrainingRuntimeObservation
	Publication      *RuntimePublication
	CloseGeneration  uint64
	CloseReason      string
	CloseRequestedAt time.Time
	ClosedAt         *time.Time
	CloseEvidence    *ManagedCloseEvidence
	OwnerRevision    uint64
}

type ExecutionRuntimeRepository interface {
	GetRuntime(context.Context, string, string) (ExecutionRuntime, error)
	RecordPrepared(context.Context, RunAuthorityCandidate, WorkspaceBinding) (ExecutionRuntime, bool, error)
	ReserveTraining(context.Context, RunAuthorityCandidate) (TrainingReservation, error)
	RecordTrainingHandle(context.Context, RunAuthorityCandidate, TrainingHandle) (ExecutionRuntime, error)
	RecordTrainingObservation(context.Context, RunAuthorityCandidate, TrainingRuntimeObservation) (ExecutionRuntime, error)
	RecordPublication(context.Context, RunAuthorityCandidate, RuntimePublication) (ExecutionRuntime, bool, error)
	RequestRuntimeClose(context.Context, RunAuthorityCandidate, string) (ExecutionRuntime, bool, error)
	ConfirmRuntimeClosed(context.Context, RunAuthorityCandidate, uint64, TrainingRuntimeObservation, ManagedCloseEvidence) (ExecutionRuntime, error)
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
	VerifyWritersAbsent(context.Context, Execution, ManagedStepAssociation) (ManagedCloseEvidence, error)
}

type ManagedCloseEvidence struct {
	RunID, WorkflowUID string
	Resources          []RuntimeResource
	ObservedAt         time.Time
}

func FreezeTrainingPlan(execution Execution, workspace WorkspaceBinding) (TrainingPlan, error) {
	if _, _, err := execution.CanonicalPayloads(); err != nil {
		return TrainingPlan{}, ErrInvalidAdmission
	}
	want := execution.Snapshot.Workspace
	if workspace.Mode != want.Mode || workspace.NamespaceName != execution.Snapshot.Environment.NamespaceName || workspace.NamespaceUID != execution.Snapshot.Environment.NamespaceUID ||
		workspace.PVCName == "" || workspace.PVCUID == "" || workspace.InputSubpath != want.InputSubpath || workspace.TrainingSubpath != want.TrainingSubpath ||
		workspace.ReportsSubpath != want.ReportsSubpath || workspace.PublicationSubpath != want.PublicationSubpath || workspace.PreparedManifestBytes <= 0 || !closeSpecHashPattern.MatchString(workspace.PreparedManifestSHA256) {
		return TrainingPlan{}, ErrRuntimeConflict
	}
	plan := TrainingPlan{TenantID: execution.TenantID, OperationID: execution.OperationID, ExecutionID: execution.ExecutionID, SpecHash: execution.SpecHash,
		Name: "md-" + strings.ToLower(execution.ExecutionID), Snapshot: execution.Snapshot, Workspace: workspace}
	encoded, err := json.Marshal(plan)
	if err != nil {
		return TrainingPlan{}, ErrInvalidAdmission
	}
	digest := sha256.Sum256(encoded)
	plan.RequestSHA256 = hex.EncodeToString(digest[:])
	return plan, nil
}
