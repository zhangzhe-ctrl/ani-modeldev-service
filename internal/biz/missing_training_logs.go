package biz

import (
	"reflect"
	"sort"
	"time"
)

// CleanupMissingTrainingLogsReview acknowledges an unrecoverable final log,
// never the absence of its original computation or a successful log capture.
type CleanupMissingTrainingLogsReview struct {
	ExecutionID                string                  `json:"execution_id"`
	CreateOperationID          string                  `json:"create_operation_id"`
	CloseOperationID           string                  `json:"close_operation_id"`
	AutomaticCleanupPlanSHA256 string                  `json:"automatic_cleanup_plan_sha256"`
	MissingPods                []MissingTrainingLogPod `json:"missing_pods"`
}

type MissingTrainingLogPod struct {
	PodUID            string `json:"pod_uid"`
	PersistedExitCode int32  `json:"persisted_exit_code"`
}

func MissingTrainingLogsReviewFor(record OperationRecord) *CleanupMissingTrainingLogsReview {
	if record.CleanupReview != nil {
		return record.CleanupReview
	}
	if record.Cleanup != nil {
		return record.Cleanup.MissingTrainingLogsReview
	}
	return nil
}

func MissingTrainingLogsMaintenanceEligible(record OperationRecord) bool {
	r := MissingTrainingLogsReviewFor(record)
	if r == nil || record.Authority == nil || r.ExecutionID != record.Execution.ExecutionID || r.CreateOperationID != record.Execution.OperationID || r.CloseOperationID != record.Runtime.CloseOperationID || !validAdmissionID(r.CloseOperationID) || r.CloseOperationID == r.CreateOperationID || record.Runtime.ComputeCleanup == nil || r.AutomaticCleanupPlanSHA256 != record.Runtime.ComputeCleanup.PlanHash || !ComputeCleanupRetryEligible(record.Execution, *record.Authority, record.Runtime) || record.Runtime.Observation == nil || len(record.Runtime.Observation.RecordedLogs) != 0 || len(r.MissingPods) == 0 || len(r.MissingPods) > 64 {
		return false
	}
	expected := []MissingTrainingLogPod{}
	for _, pod := range record.Runtime.Observation.Resources {
		if pod.Kind != "Pod" {
			continue
		}
		if !pod.Terminal || pod.ExitCode == nil {
			return false
		}
		expected = append(expected, MissingTrainingLogPod{PodUID: pod.UID, PersistedExitCode: *pod.ExitCode})
	}
	sort.Slice(expected, func(i, j int) bool { return expected[i].PodUID < expected[j].PodUID })
	return reflect.DeepEqual(expected, r.MissingPods)
}

// ReviewedComputeCleanupApplied proves resource disposal while deliberately
// leaving the exhausted automatic receipt and unavailable logs unchanged.
func ReviewedComputeCleanupApplied(record OperationRecord) bool {
	if !MissingTrainingLogsMaintenanceEligible(record) || record.Cleanup == nil {
		return false
	}
	r := record.Cleanup
	plan, err := CleanupPlanFor(record)
	if err != nil || plan.PlanHash != r.PlanHash || !validAuditActor(r.Actor) || r.StartedAt.IsZero() || record.Runtime.ClosedAt == nil || r.StartedAt.Before(*record.Runtime.ClosedAt) || len(r.Requested) > len(plan.Targets) || len(r.ConfirmedAbsent) > len(r.Requested) || !reflect.DeepEqual(r.Requested, plan.Targets[:len(r.Requested)]) || !reflect.DeepEqual(r.ConfirmedAbsent, r.Requested[:len(r.ConfirmedAbsent)]) {
		return false
	}
	if r.Phase == "APPLIED" {
		return !r.CompletedAt.IsZero() && !r.CompletedAt.Before(r.StartedAt) && reflect.DeepEqual(r.Requested, plan.Targets) && reflect.DeepEqual(r.ConfirmedAbsent, plan.Targets)
	}
	return r.Phase == "RECONCILED" && (r.ReconciledFromPhase == "NEEDS_REVIEW" || r.ReconciledFromPhase == "STARTED") && !r.ReconciledAt.IsZero() && !r.ReconciledAt.Before(r.StartedAt) && (r.CompletedAt.IsZero() || !r.ReconciledAt.Before(r.CompletedAt) && !r.CompletedAt.Before(r.StartedAt)) && reflect.DeepEqual(r.ResolvedAbsent, plan.Targets)
}

func ComputeResourcesSettledAt(state ExecutionRuntime) time.Time {
	if at := ComputeCleanupSettledAt(state.ComputeCleanup); !at.IsZero() {
		return at
	}
	if r := state.ReviewedComputeCleanup; r != nil {
		if r.Phase == "APPLIED" {
			return r.CompletedAt
		}
		if r.Phase == "RECONCILED" {
			return r.ReconciledAt
		}
	}
	return time.Time{}
}
