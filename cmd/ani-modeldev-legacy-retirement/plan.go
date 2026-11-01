package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/biz"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/data/accelerator"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/data/cleanup"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/data/runtimeproof"
)

func (a *app) buildPlan(ctx context.Context) plan {
	p := plan{Schema: schema, ObservedAt: time.Now().UTC(), ReadOnly: true, ApprovalSHA256: biz.LegacyRetirementApprovalSHA256, Identity: a.identity, DatabaseName: a.databaseName, DatabaseUser: a.databaseUser, Ready: true, Entries: []item{}}
	for _, id := range ids {
		entry, _, _, err := a.qualify(ctx, id)
		if err != nil {
			entry.Blocker = safeReason(err)
			entry.Ready = false
			p.Ready = false
		}
		p.Entries = append(p.Entries, entry)
	}
	return p
}

func (a *app) qualify(ctx context.Context, id string) (item, biz.LegacyRetirementRequest, []byte, error) {
	entry := item{ExecutionID: id}
	scope, ok := biz.LookupLegacyRetirementScope(id)
	if !ok {
		return entry, biz.LegacyRetirementRequest{}, nil, biz.ErrRuntimeConflict
	}
	execution, state, _, owner, fence, err := a.repo.GetLegacyRetirementRecord(ctx, scope.TenantID, id)
	if err != nil {
		return entry, biz.LegacyRetirementRequest{}, nil, fmt.Errorf("READ_ORIGINAL_AGGREGATE: %w", err)
	}
	entry.Owner, entry.OwnerRevision, entry.CloseGeneration, entry.CommandFence, entry.CloseReviewReason, entry.Closed = owner, state.OwnerRevision, state.CloseGeneration, fence, state.CloseReviewReason, state.ClosedAt != nil
	raw, err := a.archive.get(ctx, scope, execution.Snapshot.Environment)
	if err != nil {
		return entry, biz.LegacyRetirementRequest{}, nil, fmt.Errorf("READ_FIXED_OFFICIAL_ARCHIVE: %w", err)
	}
	entry.ArchiveSHA256 = digest(raw)
	if state.LegacyRetirement != nil {
		r := state.LegacyRetirement
		if !biz.LegacyRetirementQualification(execution, owner, state) || state.CloseEvidence == nil {
			return entry, biz.LegacyRetirementRequest{}, nil, biz.ErrRuntimeConflict
		}
		request := biz.LegacyRetirementRequest{Authority: owner, ExpectedOwnerRevision: r.ExpectedOwnerRevision, CloseGeneration: r.CloseGeneration, CommandFence: r.CommandFence, ApprovalSHA256: r.ApprovalSHA256, Actor: r.Actor, Archive: r.Archive, UnrecoverableLogs: r.UnrecoverableLogs, Evidence: *state.CloseEvidence}
		if state.Observation != nil {
			request.Observation = *state.Observation
		}
		entry.OwnerRevision = r.ExpectedOwnerRevision
		entry.ProofSHA256 = proofDigest(request)
		entry.OriginalStepPods = request.Evidence.Resources
		entry.OriginalTrainingResources = request.Observation.Resources
		entry.LogDisposition = request.UnrecoverableLogs
		boundary, err := runtimeproof.NewLegacyRetirementBoundary(a.proof, execution, owner, state, raw)
		if err != nil {
			return entry, request, nil, err
		}
		if biz.LegacyRetirementComputeCleanupApplied(execution, owner, state) {
			err = boundary.VerifyRecordedComputeWritersAbsent(ctx, execution, owner, state)
		} else {
			err = boundary.VerifyRecordedOwnerWritersAbsent(ctx, execution, owner, state.Workspace, *state.CloseEvidence)
		}
		if err != nil {
			return entry, request, nil, fmt.Errorf("CURRENT_RECORDED_OWNER_WRITER_PROOF: %w", err)
		}
		entry.Ready = true
		if execution.Snapshot.ComputeOrigin.DeviceMode == "gpu" {
			proof, err := readGovReleaseProof(ctx)
			if err != nil {
				return entry, request, nil, err
			}
			entry.OriginalGPURelease = &proof
		}
		return entry, request, raw, nil
	}
	evidence, archive, err := a.proof.VerifyLegacyRetirementOwner(ctx, execution, owner, state.Workspace, raw)
	if err != nil {
		return entry, biz.LegacyRetirementRequest{}, nil, fmt.Errorf("CURRENT_OWNER_WRITER_PROOF: %w", err)
	}
	request := biz.LegacyRetirementRequest{Authority: owner, ExpectedOwnerRevision: state.OwnerRevision, CloseGeneration: state.CloseGeneration, CommandFence: fence, ApprovalSHA256: biz.LegacyRetirementApprovalSHA256, Actor: "root:legacy-retirement-20261101", Archive: archive, Evidence: evidence, UnrecoverableLogs: []biz.LegacyTrainingLogDisposition{}}
	if scope.TrainingPodUID != "" {
		observation, err := a.training.ObserveLegacyRetirementTraining(ctx, execution, owner, state)
		if err != nil || !observation.WritersAbsent || observation.Outcome != "FAILED" {
			if err == nil {
				err = biz.ErrRuntimeNotReady
			}
			return entry, request, nil, fmt.Errorf("CURRENT_ORIGINAL_TRAINING_EXIT: %w", err)
		}
		request.Observation = observation
		request.UnrecoverableLogs = []biz.LegacyTrainingLogDisposition{{Status: biz.LegacyTrainingLogsUnrecoverable, PodUID: scope.TrainingPodUID, ContainerID: scope.TrainingContainerID, KubeletErrorBytes: 115, KubeletErrorSHA256: scope.TrainingLogErrorSHA256, ObservedAt: a.config.LogEvidenceObservedAt}}
	}
	if err := a.repo.ValidateLegacyRetirement(ctx, request); err != nil {
		return entry, request, nil, fmt.Errorf("ORIGINAL_CLOSE_CAS_QUALIFICATION: %w", err)
	}
	if err := cleanup.New(a.kube, a.proof).ValidateLegacyRetirementCleanupPlan(ctx, execution, owner, state, evidence, request.Observation); err != nil {
		return entry, request, nil, fmt.Errorf("CURRENT_ORIGINAL_CLEANUP_GUARDS: %w", err)
	}
	entry.ProofSHA256 = proofDigest(request)
	entry.OriginalStepPods = evidence.Resources
	entry.OriginalTrainingResources = request.Observation.Resources
	entry.LogDisposition = request.UnrecoverableLogs
	entry.Ready = true
	if execution.Snapshot.ComputeOrigin.DeviceMode == "gpu" {
		proof, err := readGovReleaseProof(ctx)
		if err != nil {
			return entry, request, nil, err
		}
		entry.OriginalGPURelease = &proof
	}
	return entry, request, raw, nil
}

// The reviewed digest freezes every original graph/exit and archive identity.
// Observation timestamps are refreshed at apply, and are explicitly excluded.
func proofDigest(request biz.LegacyRetirementRequest) string {
	request.Archive.ObservedAt = time.Time{}
	request.Evidence.ObservedAt = time.Time{}
	request.Observation.ObservedAt = time.Time{}
	raw, _ := json.Marshal(request)
	return digest(raw)
}
func loadPlan(path, sha string) (plan, error) {
	var p plan
	raw, err := readBounded(path, 1<<20)
	if err != nil || len(sha) != 64 || digest(raw) != sha || strictJSON(raw, &p) != nil || p.Schema != schema || !p.ReadOnly || !p.Ready || p.ApprovalSHA256 != biz.LegacyRetirementApprovalSHA256 || len(p.Entries) != len(ids) || p.ObservedAt.IsZero() || time.Since(p.ObservedAt) > 30*time.Minute || p.ObservedAt.After(time.Now()) {
		return p, errors.New("MAINTENANCE_REVIEWED_PLAN_INVALID")
	}
	for i, entry := range p.Entries {
		if entry.ExecutionID != ids[i] || !entry.Ready || entry.Blocker != "" || entry.OwnerRevision == 0 || entry.CommandFence != entry.CloseGeneration || len(entry.ProofSHA256) != 64 {
			return p, errors.New("MAINTENANCE_REVIEWED_SCOPE_INVALID")
		}
	}
	return p, nil
}

type admissionGate struct {
	SourceEvidenceSHA256    string    `json:"source_evidence_sha256"`
	ReviewedPlanSHA256      string    `json:"reviewed_plan_sha256"`
	Schema                  string    `json:"schema"`
	ObservedAt              time.Time `json:"observed_at"`
	ClusterID               string    `json:"cluster_id"`
	KubeSystemUID           string    `json:"kube_system_uid"`
	GovernanceDeploymentUID string    `json:"governance_deployment_uid"`
	GovernanceImage         string    `json:"governance_image"`
	CreatePaused            bool      `json:"create_paused"`
	CreateHTTPStatus        int       `json:"create_http_status"`
	CreateReason            string    `json:"create_reason"`
	ActiveCreateLeases      int       `json:"active_create_leases"`
	RootApplyAuthorized     bool      `json:"root_apply_authorized"`
}

func checkAdmissionGate(path, sha, expectedPlanSHA string) error {
	raw, err := readBounded(path, 64<<10)
	var gate admissionGate
	if err != nil || len(sha) != 64 || digest(raw) != sha || strictJSON(raw, &gate) != nil || gate.Schema != "ani.modeldev.legacy-retirement.stopped-admission.v1" || !isSHA256(gate.SourceEvidenceSHA256) || !isSHA256(gate.ReviewedPlanSHA256) || gate.ReviewedPlanSHA256 != expectedPlanSHA || gate.ClusterID != "f39bf6e2-f3ed-4191-ada3-caeebf86c220" || gate.KubeSystemUID != "3eeb5379-8748-4bf8-8f6d-d7a48ce3bc24" || gate.GovernanceDeploymentUID != "4ae87828-5f2c-4b27-a8b8-d348aeb86ee5" || gate.GovernanceImage == "" || !gate.CreatePaused || gate.CreateHTTPStatus != 503 || gate.CreateReason != "MODELDEV_CREATE_MAINTENANCE" || gate.ActiveCreateLeases != 0 || !gate.RootApplyAuthorized || gate.ObservedAt.IsZero() || time.Since(gate.ObservedAt) > 5*time.Minute || gate.ObservedAt.After(time.Now()) {
		return errors.New("MAINTENANCE_STOPPED_ADMISSION_NOT_PROVED")
	}
	return nil
}

func (a *app) apply(ctx context.Context, p plan, gateFile, gateSHA, reviewedPlanSHA string) error {
	// The complete three-entry preflight precedes the first close mutation.
	requests := map[string]biz.LegacyRetirementRequest{}
	archives := map[string][]byte{}
	for i, id := range ids {
		observed, request, raw, err := a.qualify(ctx, id)
		if err != nil || observed.Owner != p.Entries[i].Owner || observed.OwnerRevision != p.Entries[i].OwnerRevision || observed.CloseGeneration != p.Entries[i].CloseGeneration || observed.CommandFence != p.Entries[i].CommandFence || observed.ArchiveSHA256 != p.Entries[i].ArchiveSHA256 || observed.ProofSHA256 != p.Entries[i].ProofSHA256 {
			return errors.New("MAINTENANCE_REVIEWED_PROOF_CHANGED")
		}
		if p.Entries[i].OriginalGPURelease != nil {
			if observed.OriginalGPURelease == nil || observed.OriginalGPURelease.digest() != p.Entries[i].OriginalGPURelease.digest() {
				return errors.New("MAINTENANCE_ORIGINAL_GOV_RECEIPT_CHANGED")
			}
		} else if observed.OriginalGPURelease != nil {
			return errors.New("MAINTENANCE_ORIGINAL_GOV_SCOPE_CHANGED")
		}
		requests[id], archives[id] = request, raw
	}
	if err := checkAdmissionGate(gateFile, gateSHA, reviewedPlanSHA); err != nil {
		return err
	}
	currentIdentity, err := a.readIdentity(ctx)
	if err != nil || currentIdentity != p.Identity {
		return errors.New("MAINTENANCE_IDENTITY_CHANGED_BEFORE_WRITE")
	}
	reporter, closeReporter, err := accelerator.NewQuotaClient(a.config.Governance)
	if err != nil {
		return errors.New("MAINTENANCE_ORIGINAL_GOVERNANCE_CONNECTION_INVALID")
	}
	defer closeReporter()
	for _, id := range ids {
		if err := checkAdmissionGate(gateFile, gateSHA, reviewedPlanSHA); err != nil {
			return err
		}
		scope, _ := biz.LookupLegacyRetirementScope(id)
		execution, state, _, owner, _, err := a.repo.GetLegacyRetirementRecord(ctx, scope.TenantID, id)
		if err != nil {
			return errors.New("MAINTENANCE_REREAD_REFUSED")
		}
		if state.LegacyRetirement == nil {
			state, err = a.repo.ConfirmLegacyRetirement(ctx, requests[id])
			if err != nil {
				return fmt.Errorf("MAINTENANCE_CLOSE_CAS_REFUSED: %s", safeReason(err))
			}
		}
		if state.StepExitCleanup == nil {
			state, err = a.repo.BeginStepExitCleanup(ctx, owner, state.CloseGeneration, *state.CloseEvidence)
			if err != nil {
				return errors.New("MAINTENANCE_STEP_INTENT_REFUSED")
			}
		}
		if state.StepExitCleanup.CompletedAt == nil {
			boundary, err := runtimeproof.NewLegacyRetirementBoundary(a.proof, execution, owner, state, archives[id])
			if err != nil {
				return err
			}
			removed, err := boundary.ReconcileStepExitCleanup(ctx, execution, owner, state.Workspace, *state.StepExitCleanup)
			if err != nil {
				return fmt.Errorf("MAINTENANCE_STEP_BOUNDARY_REFUSED: %s", safeReason(err))
			}
			state, err = a.repo.CompleteStepExitCleanup(ctx, owner, state.CloseGeneration, removed)
			if err != nil {
				return errors.New("MAINTENANCE_STEP_COMMIT_REFUSED")
			}
		}
		if state.ComputeCleanup == nil {
			state, err = a.repo.BeginComputeCleanup(ctx, owner, state.CloseGeneration)
			if err != nil {
				return errors.New("MAINTENANCE_COMPUTE_INTENT_REFUSED")
			}
		}
		if state.ComputeCleanup.Phase != "APPLIED" && state.ComputeCleanup.Attempts > 0 {
			boundary, err := runtimeproof.NewLegacyRetirementBoundary(a.proof, execution, owner, state, archives[id])
			if err != nil {
				return err
			}
			if absent, observeErr := boundary.ObserveComputeCleanupAbsence(ctx, execution, owner, state); observeErr == nil {
				state, err = a.repo.RecordComputeCleanupResult(ctx, owner, state.CloseGeneration, state.ComputeCleanup.Attempts, absent, "")
				if err != nil {
					return errors.New("MAINTENANCE_EXISTING_CLEANUP_ABSENCE_CAS_REFUSED")
				}
			}
		}
		if state.ComputeCleanup.Phase != "APPLIED" {
			state, err = a.repo.StartComputeCleanupAttempt(ctx, owner, state.CloseGeneration)
			if err != nil {
				return errors.New("MAINTENANCE_COMPUTE_ATTEMPT_REFUSED")
			}
			if scope.TrainingPodUID != "" {
				if err := a.training.ReleaseLegacyRetirementTrainingExit(ctx, execution, owner, state); err != nil {
					return fmt.Errorf("MAINTENANCE_TRAINING_EXIT_RELEASE_REFUSED: %s", safeReason(err))
				}
			}
			boundary, err := runtimeproof.NewLegacyRetirementBoundary(a.proof, execution, owner, state, archives[id])
			if err != nil {
				return err
			}
			removed, boundaryErr := boundary.ReconcileComputeCleanup(ctx, execution, owner, state)
			code := ""
			if boundaryErr != nil {
				code = "BOUNDARY_UNCERTAIN"
			}
			committed, commitErr := a.repo.RecordComputeCleanupResult(ctx, owner, state.CloseGeneration, state.ComputeCleanup.Attempts, removed, code)
			if commitErr != nil {
				return errors.New("MAINTENANCE_COMPUTE_COMMIT_REFUSED")
			}
			state = committed
			if boundaryErr != nil {
				return fmt.Errorf("MAINTENANCE_COMPUTE_BOUNDARY_REFUSED: %s", safeReason(boundaryErr))
			}
		}
		if execution.Snapshot.ComputeOrigin.DeviceMode == "gpu" && state.LegacyRetirement.PriorGPURelease == nil {
			before, err := readGovReleaseProof(ctx)
			if err != nil {
				return err
			}
			ack, err := reporter.ReportGPUResourceReleased(ctx, biz.LegacyRetirementOriginalGPUEvent())
			if err != nil {
				return errors.New("MAINTENANCE_ORIGINAL_GOV_REPLAY_NOT_ACKNOWLEDGED")
			}
			after, err := readGovReleaseProof(ctx)
			if err != nil {
				return err
			}
			receipt, err := priorGPUReceipt(before, after, ack)
			if err != nil {
				return err
			}
			state, err = a.repo.RecordLegacyRetirementGPURelease(ctx, owner, state.OwnerRevision, receipt)
			if err != nil {
				return errors.New("MAINTENANCE_ORIGINAL_GOV_JOURNAL_CAS_REFUSED")
			}
		}
		if execution.Snapshot.ComputeOrigin.DeviceMode == "gpu" && !biz.ValidLegacyRetirementPriorGPURelease(execution, owner, state.LegacyRetirement.PriorGPURelease) {
			return errors.New("MAINTENANCE_FINAL_ORIGINAL_GPU_JOURNAL_REFUSED")
		}

		if !biz.LegacyRetirementComputeCleanupApplied(execution, owner, state) {
			return errors.New("MAINTENANCE_FINAL_QUALIFICATION_REFUSED")
		}
		var event any
		var ack any
		if prior := state.LegacyRetirement.PriorGPURelease; prior != nil {
			event = prior.Event
			ack = prior.ACK
		}
		if state.GPURelease != nil && state.GPURelease.Outbox != nil {
			event = state.GPURelease.Outbox.Event
			ack = state.GPURelease.Outbox.ACK
		}
		if err := json.NewEncoder(os.Stdout).Encode(map[string]any{"execution_id": id, "closed_at": state.ClosedAt, "step_exit_cleanup_complete": state.StepExitCleanup.CompletedAt != nil, "compute_cleanup_phase": state.ComputeCleanup.Phase, "logs_ready": state.ComputeCleanup.LogsReady, "captured_log_count": len(state.ComputeCleanup.Logs), "prior_gpu_release_journal": state.LegacyRetirement.PriorGPURelease, "gpu_release_event": event, "gpu_ack": ack, "owner_revision": state.OwnerRevision}); err != nil {
			return err
		}
	}
	return nil
}

// Keep errors bounded to known qualification stages and business reasons. Raw
// network/SQL errors and external response bodies never reach receipts/stdout.
func safeReason(err error) string {
	stage := ""
	for _, prefix := range []string{"READ_ORIGINAL_AGGREGATE", "READ_FIXED_OFFICIAL_ARCHIVE", "CURRENT_OWNER_WRITER_PROOF", "CURRENT_RECORDED_OWNER_WRITER_PROOF", "CURRENT_ORIGINAL_TRAINING_EXIT", "ORIGINAL_CLOSE_CAS_QUALIFICATION", "CURRENT_ORIGINAL_CLEANUP_GUARDS"} {
		if strings.HasPrefix(err.Error(), prefix+":") {
			stage = prefix + ":"
			break
		}
	}
	// Preserve only fixed, non-sensitive qualification locations already
	// supplied by the verifier, never arbitrary external error text.
	for _, detail := range []struct{ prefix, code string }{
		{"owner workflow completion:", "ARCHIVE_GRAPH_COMPLETION"},
		{"unprepared workspace request:", "UNPREPARED_WORKSPACE_REQUEST"},
		{"owner pod exits ", "HISTORICAL_POD_EXITS"},
		{"owner remaining managed tasks:", "REMAINING_MANAGED_TASKS"},
	} {
		if strings.Contains(err.Error(), detail.prefix) {
			stage += detail.code + ":"
			break
		}
	}
	for _, sentinel := range []error{biz.ErrRuntimeConflict, biz.ErrRuntimeNotReady, biz.ErrCleanupBlocked, biz.ErrCleanupConflict, biz.ErrCleanupUncertain, biz.ErrTrainingLogsUnavailable, biz.ErrRunAuthorityConflict, biz.ErrPersistence, biz.ErrInvalidAdmission} {
		if errors.Is(err, sentinel) {
			return stage + sentinel.Error()
		}
	}
	return stage + "MAINTENANCE_EXTERNAL_READ_UNAVAILABLE"
}
