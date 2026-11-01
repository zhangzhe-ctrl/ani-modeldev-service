package biz

import (
	"reflect"
	"time"

	"github.com/zhangzhe-ctrl/ani-modeldev-service/contract/cpup01"
)

// These constants describe the human-approved, one-time retirement. They are
// deliberately absent from the ordinary runtime configuration and admission.
const LegacyRetirementApprovalSHA256 = "53c1b292af08051c06f254bf05308734e1db9b87e4c48da9ecd3e47a3e11b60e"
const LegacyRetirementArchiveSource = "KFP_V1_PIPELINE_RUNTIME_WORKFLOW_MANIFEST"
const LegacyTrainingLogsUnrecoverable = "TRAINING_LOGS_UNRECOVERABLE"

type LegacyRetirementScope struct {
	TenantID, ClusterID, ExecutionID, NamespaceName, NamespaceUID string
	RunID, WorkflowName, WorkflowUID, ArchiveSHA256               string
	TrainingPodUID, TrainingContainerID, TrainingLogErrorSHA256   string
}

// LookupLegacyRetirementScope returns a value copy of the entire approved
// inventory. No caller can register another execution or substitute an owner.
func LookupLegacyRetirementScope(id string) (LegacyRetirementScope, bool) {
	common := LegacyRetirementScope{TenantID: "64c5c35a-f554-432c-b4bc-7b4f36f08420", ClusterID: "f39bf6e2-f3ed-4191-ada3-caeebf86c220", ExecutionID: id, NamespaceName: "ani-modeldev-tenant-a-v21", NamespaceUID: "94f3afc0-7548-4099-9e11-3fa7fdf6cc67"}
	switch id {
	case "1cce950b-d6e9-453e-aa11-fffd8846224b":
		common.RunID, common.WorkflowName, common.WorkflowUID = "bbd4ad22-5b1a-448a-b71a-1069e18adc2f", "ft-sw2pr", "edcf4a1d-cdf9-44da-9f28-20593d79469f"
		common.ArchiveSHA256 = "098d33c48011dadc33f00a88a3037ba3d232cb96f85d1064b83d53d4b410041c"
		common.TrainingPodUID, common.TrainingContainerID = "49c82c2b-f819-4a03-b7b2-1789999cdd2e", "containerd://4cddb476920a21444eba72d93ba814e17fda84cae8714fda23cc42be3e7fb5ab"
		common.TrainingLogErrorSHA256 = "224eade78cbd65470c1e776fdae13018619345eff1a3be65cfde0ef0c3f20ad0"
	case "4a1a92f7-4d94-439d-a8d7-1833f38c7dbc":
		common.RunID, common.WorkflowName, common.WorkflowUID = "2c372d09-70a8-4557-8b60-20f7ecd7d861", "ft-j4ncq", "1e9521b7-fc75-41f2-a406-690a7c142b73"
		common.ArchiveSHA256 = "9b58e508fb0fca256acca7aafaa6e02876167fc8726b4ec45b9a9130fb1d0b7d"
		common.TrainingPodUID, common.TrainingContainerID = "d8fb153a-5763-4ae7-8d44-63e3f996a830", "containerd://f76126a2196a82f34f390ae377728bfefed945de11b2c47735dfbfab1b6faf06"
		common.TrainingLogErrorSHA256 = "9ce39f9afd60e2fad4686865e363b3978b52907559bf7bfa75aa34dcdeb6f7fc"
	case "eca4d47b-5911-4f0f-8b45-89c1be775925":
		common.RunID, common.WorkflowName, common.WorkflowUID = "5f04cda7-4c66-4f16-a552-053ebd4e5b6f", "md-kpwkw", "d2acb3df-3979-4d60-aed5-16db0ce440fb"
		common.ArchiveSHA256 = "c88268fdbb84fade76d2c488e5211fd697f6f852feb6c0a75b21ef67842142fb"
	default:
		return LegacyRetirementScope{}, false
	}
	return common, true
}

func LegacyRetirementScopeMatches(execution Execution, owner RunAuthorityCandidate) bool {
	s, ok := LookupLegacyRetirementScope(execution.ExecutionID)
	return ok && execution.TenantID == s.TenantID && execution.Snapshot.Environment.ClusterID == s.ClusterID && execution.Snapshot.Environment.NamespaceName == s.NamespaceName && execution.Snapshot.Environment.NamespaceUID == s.NamespaceUID && owner.TenantID == execution.TenantID && owner.ExecutionID == execution.ExecutionID && owner.OperationID == execution.OperationID && owner.SpecHash == execution.SpecHash && owner.NamespaceName == s.NamespaceName && owner.NamespaceUID == s.NamespaceUID && owner.RunID == s.RunID && owner.WorkflowName == s.WorkflowName && owner.WorkflowUID == s.WorkflowUID
}

// Archive provenance remains separate from live Kubernetes object evidence.
// ArchivedResourceVersion is the archived owner's version, never a live GET.
type LegacyRetirementArchiveReceipt struct {
	Source                                                                     string
	ExecutionID, NamespaceName, NamespaceUID, RunID, WorkflowName, WorkflowUID string
	SHA256, ArchivedResourceVersion                                            string
	ObservedAt                                                                 time.Time
}

// A disposition records an unavailable original stream. It contains no lines,
// captured flag, or TrainingLog and cannot satisfy the ordinary log contract.
type LegacyTrainingLogDisposition struct {
	Status, PodUID, ContainerID string
	KubeletErrorBytes           int64
	KubeletErrorSHA256          string
	ObservedAt                  time.Time
}

type LegacyRetirementRequest struct {
	Authority                                            RunAuthorityCandidate
	ExpectedOwnerRevision, CloseGeneration, CommandFence uint64
	ApprovalSHA256, Actor                                string
	Archive                                              LegacyRetirementArchiveReceipt
	UnrecoverableLogs                                    []LegacyTrainingLogDisposition
	TrainingExitKind                                     LegacyRetirementTrainingExitKind
	Observation                                          TrainingRuntimeObservation
	Evidence                                             ManagedCloseEvidence
}

// Record is an immutable qualification journal, not a permission to send work.
// It freezes the exact CAS inputs that admitted the maintenance-only close.
type LegacyRetirementRecord struct {
	PriorGPURelease                                      *LegacyRetirementPriorGPURelease `json:",omitempty"`
	Authority                                            RunAuthorityCandidate
	ExpectedOwnerRevision, CloseGeneration, CommandFence uint64
	ApprovalSHA256, Actor                                string
	Archive                                              LegacyRetirementArchiveReceipt
	UnrecoverableLogs                                    []LegacyTrainingLogDisposition
	TrainingExitKind                                     LegacyRetirementTrainingExitKind
	RecordedAt                                           time.Time
}

// LegacyRetirementQualification validates the finite exception envelope. The
// repository still validates the real exits, graph and unique FrozenPlan Run.
func LegacyRetirementQualification(execution Execution, owner RunAuthorityCandidate, state ExecutionRuntime) bool {
	r := state.LegacyRetirement
	mode := "cpu"
	if execution.ExecutionID == "eca4d47b-5911-4f0f-8b45-89c1be775925" {
		mode = "gpu"
	}
	if execution.Snapshot.ComputeOrigin.DeviceMode != mode {
		return false
	}
	if r == nil || !LegacyRetirementScopeMatches(execution, owner) || r.Authority != owner || r.ApprovalSHA256 != LegacyRetirementApprovalSHA256 || r.ExpectedOwnerRevision == 0 || r.CommandFence == 0 || r.CommandFence != r.CloseGeneration || r.CloseGeneration != state.CloseGeneration || !cpup01.ValidAuditActor(r.Actor) || r.RecordedAt.IsZero() || r.RecordedAt.Before(state.CloseRequestedAt) || state.CloseEvidence == nil || state.CloseEvidence.NoDispatch || state.CloseEvidence.OwnerTermination == nil {
		return false
	}
	if state.ComputeCleanup != nil && (state.ComputeCleanup.LogsReady || len(state.ComputeCleanup.Logs) != 0 || state.ComputeCleanup.OperatorRetry != nil) {
		return false
	}
	if r.PriorGPURelease != nil && (!ValidLegacyRetirementPriorGPURelease(execution, owner, r.PriorGPURelease) || state.GPURelease != nil || state.ClosedAt == nil || state.ComputeCleanup == nil || state.ComputeCleanup.Phase != "APPLIED" || state.ComputeCleanup.CompletedAt.IsZero() || r.PriorGPURelease.ObservedAt.Before(state.ComputeCleanup.CompletedAt) || state.StepExitCleanup == nil || state.StepExitCleanup.CompletedAt == nil) {
		return false
	}
	s, _ := LookupLegacyRetirementScope(execution.ExecutionID)
	a := r.Archive
	if a.Source != LegacyRetirementArchiveSource || a.ExecutionID != s.ExecutionID || a.NamespaceName != s.NamespaceName || a.NamespaceUID != s.NamespaceUID || a.RunID != s.RunID || a.WorkflowName != s.WorkflowName || a.WorkflowUID != s.WorkflowUID || a.SHA256 != s.ArchiveSHA256 || a.ArchivedResourceVersion == "" || a.ArchivedResourceVersion != state.CloseEvidence.OwnerTermination.WorkflowResourceVersion || !a.ObservedAt.Equal(state.CloseEvidence.ObservedAt) || a.ObservedAt.IsZero() || a.ObservedAt.After(r.RecordedAt) {
		return false
	}
	if s.TrainingPodUID == "" {
		return r.TrainingExitKind == "" && len(r.UnrecoverableLogs) == 0 && state.Training == nil && state.TrainingHandle == nil && state.Observation == nil && state.Publication == nil
	}
	if len(r.UnrecoverableLogs) != 1 || state.Training == nil || state.TrainingHandle == nil || state.Workspace == nil || state.Observation == nil || state.Observation.Handle != *state.TrainingHandle || !state.Observation.WritersAbsent || state.Publication != nil || len(state.Observation.Resources) > 64 {
		return false
	}
	if kind, err := ClassifyLegacyRetirementTrainingExit(execution, owner, state, *state.Observation); err != nil || kind != r.TrainingExitKind {
		return false
	}
	log := r.UnrecoverableLogs[0]
	if log.Status != LegacyTrainingLogsUnrecoverable || log.PodUID != s.TrainingPodUID || log.ContainerID != s.TrainingContainerID || log.KubeletErrorBytes != 115 || log.KubeletErrorSHA256 != s.TrainingLogErrorSHA256 || log.ObservedAt.IsZero() || log.ObservedAt.After(r.RecordedAt) {
		return false
	}
	pods := 0
	byUID := map[string]RuntimeResource{}
	for _, resource := range state.Observation.Resources {
		if resource.Namespace != s.NamespaceName || resource.UID == "" || byUID[resource.UID].UID != "" {
			return false
		}
		byUID[resource.UID] = resource
		if resource.Kind == "Pod" {
			pods++
			if resource.UID != s.TrainingPodUID || !resource.Terminal || resource.ExitCode == nil || *resource.ExitCode != 143 || resource.TrainingLog != nil || resource.InitializationAbort != nil || resource.SandboxTermination != nil {
				return false
			}
		}
	}
	train := byUID[state.TrainingHandle.TrainJobUID]
	if pods != 1 || train.Kind != "TrainJob" || train.APIVersion != "trainer.kubeflow.org/v1alpha1" || train.Name != state.Training.Name || train.OwnerUID != "" {
		return false
	}
	counts := map[string]int{}
	for _, resource := range byUID {
		counts[resource.Kind]++
		parent := byUID[resource.OwnerUID]
		switch resource.Kind {
		case "TrainJob":
		case "JobSet":
			if resource.APIVersion != "jobset.x-k8s.io/v1alpha2" || parent.UID != train.UID {
				return false
			}
		case "Job":
			if resource.APIVersion != "batch/v1" || parent.Kind != "JobSet" {
				return false
			}
		case "Pod":
			if resource.APIVersion != "v1" || parent.Kind != "Job" {
				return false
			}
		case "PodGroup":
			if resource.APIVersion != "scheduling.volcano.sh/v1beta1" || parent.UID != train.UID || resource.Name != state.Training.Name || !resource.Terminal {
				return false
			}
		default:
			return false
		}
		if resource.Kind != "Pod" && !resource.Terminal && !resource.CreationDisabled {
			return false
		}
	}
	return counts["TrainJob"] == 1 && counts["JobSet"] == 1 && counts["Job"] == 1 && counts["PodGroup"] <= 1
}

// Cleanup authorization is explicit and never makes LogsReady true. Ordinary
// ComputeCleanupLogsComplete remains strict and rejects these unavailable logs.
func LegacyRetirementCleanupAuthorized(execution Execution, owner RunAuthorityCandidate, state ExecutionRuntime) bool {
	if !LegacyRetirementQualification(execution, owner, state) || state.ClosedAt == nil || state.CloseReviewReason != "" || state.ComputeCleanup == nil || state.ComputeCleanup.LogsReady || len(state.ComputeCleanup.Logs) != 0 || state.ComputeCleanup.OperatorRetry != nil || state.ComputeCleanup.Attempts == 0 || state.ComputeCleanup.Attempts > ComputeCleanupMaximumAttempts {
		return false
	}
	targets, hash, err := ComputeCleanupTargets(execution, owner, state)
	return err == nil && hash == state.ComputeCleanup.PlanHash && state.ComputeCleanup.CloseGeneration == state.CloseGeneration && reflect.DeepEqual(targets, state.ComputeCleanup.Targets) && reflect.DeepEqual(targets, state.ComputeCleanup.Requested)
}

func LegacyRetirementComputeCleanupApplied(execution Execution, owner RunAuthorityCandidate, state ExecutionRuntime) bool {
	return LegacyRetirementCleanupAuthorized(execution, owner, state) && state.ComputeCleanup.Phase == "APPLIED" && !state.ComputeCleanup.CompletedAt.IsZero() && ComputeCleanupRemovedValid(state.ComputeCleanup.Targets, state.ComputeCleanup.ConfirmedAbsent, true)
}

// The old Governance generic release receipt predates an MD release anchor.
// This independent retirement journal preserves that actual event; it is never
// converted into a fabricated MD scope/outbox or a new ABORTED_CLEANED event.
type LegacyRetirementPriorGPURelease struct {
	ReceiptID, PayloadSHA256                  string
	Event                                     GPUResourceReleasedEvent
	ACK                                       GPUReleaseACK
	ChargeID, QuotaCode                       string
	OriginalUnits, ReleasedUnits              int64
	BeforeAccounts, AfterAccounts             []LegacyRetirementAccountInvariant
	BeforeEvidenceSHA256, AfterEvidenceSHA256 string
	ObservedAt                                time.Time
}

type LegacyRetirementAccountInvariant struct {
	AccountID                          int64
	QuotaCode                          string
	OccupiedUnits, AllOwnerChargeUnits int64
}

func LegacyRetirementOriginalGPUEvent() GPUResourceReleasedEvent {
	return GPUResourceReleasedEvent{ReleaseEventID: "54c1fb78-da48-4586-990e-333ed8090aef", CreateOperationID: "1bc185ad-0977-4b3f-bb1c-404bc37e94fa", ResourceID: "eca4d47b-5911-4f0f-8b45-89c1be775925", Reason: "RESOURCE_RELEASED"}
}

func ValidLegacyRetirementPriorGPURelease(execution Execution, owner RunAuthorityCandidate, receipt *LegacyRetirementPriorGPURelease) bool {
	if receipt == nil || !LegacyRetirementScopeMatches(execution, owner) || execution.ExecutionID != "eca4d47b-5911-4f0f-8b45-89c1be775925" || execution.OperationID != "1bc185ad-0977-4b3f-bb1c-404bc37e94fa" || execution.Snapshot.ComputeOrigin.DeviceMode != "gpu" || receipt.ReceiptID != "23600748-d80d-5ceb-be35-664a995bb7db" || receipt.PayloadSHA256 != "3e63b8ddc427e1228169804c6a6d28c34899318224257119b0c788b522ca7cad" || receipt.Event != LegacyRetirementOriginalGPUEvent() || receipt.ACK.ReleaseEventID != receipt.Event.ReleaseEventID || receipt.ACK.CreateOperationID != receipt.Event.CreateOperationID || receipt.ACK.ResourceID != receipt.Event.ResourceID || receipt.ACK.Status != "ALREADY_APPLIED" || receipt.ChargeID != "1045b409-962a-4211-aee7-30319bffe459" || receipt.QuotaCode != "gpu.shared_memory_mib" || receipt.OriginalUnits != 24570 || receipt.ReleasedUnits != receipt.OriginalUnits || len(receipt.BeforeAccounts) != 2 || !reflect.DeepEqual(receipt.BeforeAccounts, receipt.AfterAccounts) || !validLegacyRetirementDigest(receipt.BeforeEvidenceSHA256) || receipt.BeforeEvidenceSHA256 != receipt.AfterEvidenceSHA256 || receipt.ObservedAt.IsZero() || receipt.ObservedAt.After(time.Now().UTC()) {
		return false
	}
	for i, a := range receipt.BeforeAccounts {
		if a.OccupiedUnits < 0 || a.OccupiedUnits != a.AllOwnerChargeUnits {
			return false
		}
		if i == 0 && (a.AccountID != 2 || a.QuotaCode != "gpu.physical.count") || i == 1 && (a.AccountID != 6 || a.QuotaCode != "gpu.shared_memory_mib") {
			return false
		}
	}
	return true
}

func validLegacyRetirementDigest(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, c := range value {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}
