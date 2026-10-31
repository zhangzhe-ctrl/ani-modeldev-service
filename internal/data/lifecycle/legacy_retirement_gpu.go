package lifecycle

import (
	"context"
	"reflect"

	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/biz"
	lifecyclesql "github.com/zhangzhe-ctrl/ani-modeldev-service/internal/data/lifecycle/sqlc"
)

// RecordLegacyRetirementGPURelease retains the actual old Governance event and
// fresh zero-delta replay ACK after original compute cleanup. No MD release
// anchor, scope, outbox, new event, or quota value is written by this method.
func (repository *LegacyRetirementRepository) RecordLegacyRetirementGPURelease(ctx context.Context, owner biz.RunAuthorityCandidate, expectedRevision uint64, receipt biz.LegacyRetirementPriorGPURelease) (biz.ExecutionRuntime, error) {
	state, _, err := repository.mutate(ctx, owner, func(current *runtimeTransaction) (bool, error) {
		if !current.legacyRetirement || current.state.LegacyRetirement == nil || !biz.LegacyRetirementComputeCleanupApplied(current.execution, owner, current.state) || current.state.GPURelease != nil || !biz.ValidLegacyRetirementPriorGPURelease(current.execution, owner, &receipt) || receipt.ObservedAt.After(current.now) || receipt.ObservedAt.Before(current.state.ComputeCleanup.CompletedAt) {
			return false, biz.ErrRuntimeConflict
		}
		anchor, err := current.queries.GetGPUReleaseAnchor(ctx, lifecyclesql.GetGPUReleaseAnchorParams{TenantID: current.tenantID, ExecutionID: current.executionID})
		if err != nil || anchor.GpuReleaseSealedAt.Valid || anchor.GpuReleaseScopeSha256.Valid {
			return false, biz.ErrRuntimeConflict
		}
		if prior := current.state.LegacyRetirement.PriorGPURelease; prior != nil {
			if !reflect.DeepEqual(*prior, receipt) {
				return false, biz.ErrRuntimeConflict
			}
			return false, nil
		}
		if expectedRevision == 0 || expectedRevision != current.state.OwnerRevision {
			return false, biz.ErrRuntimeConflict
		}
		current.state.LegacyRetirement.PriorGPURelease = &receipt
		return true, nil
	})
	return state, err
}
