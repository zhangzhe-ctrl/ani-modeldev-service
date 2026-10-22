package lifecycle

import (
	"context"
	"encoding/json"
	"reflect"

	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/biz"
)

func (repository *Repository) RecordComputeCleanupResolved(ctx context.Context, owner biz.RunAuthorityCandidate, generation uint64, maintenanceHash string, absent []biz.RuntimeResource) (biz.ExecutionRuntime, error) {
	state, _, err := repository.mutate(ctx, owner, func(current *runtimeTransaction) (bool, error) {
		r := current.state.ComputeCleanup
		if r != nil && r.Phase == "RECONCILED" {
			if r.MaintenancePlanHash != maintenanceHash || !reflect.DeepEqual(r.ResolvedAbsent, absent) {
				return false, biz.ErrCleanupConflict
			}
			return false, nil
		}
		record := biz.OperationRecord{QueryRecord: biz.QueryRecord{Execution: current.execution, Runtime: current.state}, Authority: &owner}
		if generation != current.state.CloseGeneration || !biz.ComputeCleanupMaintenanceEligible(record) || !biz.ComputeCleanupRemovedValid(r.Targets, absent, true) {
			return false, biz.ErrCleanupBlocked
		}
		var phase string
		var planBytes, receiptBytes []byte
		if err := current.transaction.QueryRow(ctx, "SELECT phase,plan,receipt FROM modeldev_execution_cleanup_audits WHERE tenant_id=$1::uuid AND execution_id=$2::uuid AND plan_sha256=$3", owner.TenantID, owner.ExecutionID, maintenanceHash).Scan(&phase, &planBytes, &receiptBytes); err != nil {
			return false, biz.ErrCleanupBlocked
		}
		var plan biz.CleanupPlan
		var manual biz.CleanupReceipt
		if json.Unmarshal(planBytes, &plan) != nil || json.Unmarshal(receiptBytes, &manual) != nil || manual.Phase != phase || plan.PlanHash != maintenanceHash || manual.PlanHash != maintenanceHash || manual.ExecutionID != owner.ExecutionID || plan.TenantID != owner.TenantID || plan.ExecutionID != owner.ExecutionID || plan.OperationID != owner.OperationID || plan.SpecHash != owner.SpecHash || plan.CloseGeneration != generation || plan.NamespaceUID != owner.NamespaceUID || !reflect.DeepEqual(plan.Targets, biz.ComputeCleanupMaintenanceTargets(r)) {
			return false, biz.ErrCleanupConflict
		}
		if phase == "APPLIED" {
			if !reflect.DeepEqual(manual.Requested, plan.Targets) || !reflect.DeepEqual(manual.ConfirmedAbsent, plan.Targets) || manual.CompletedAt.IsZero() {
				return false, biz.ErrCleanupBlocked
			}
		} else if phase != "RECONCILED" || manual.ReconciledAt.IsZero() || !reflect.DeepEqual(manual.ResolvedAbsent, plan.Targets) {
			return false, biz.ErrCleanupBlocked
		}
		r.ReconciledFromPhase, r.Phase = r.Phase, "RECONCILED"
		r.ResolvedAbsent = append([]biz.RuntimeResource{}, absent...)
		r.ReconciledAt, r.MaintenancePlanHash = current.now, maintenanceHash
		return true, nil
	})
	return state, err
}
