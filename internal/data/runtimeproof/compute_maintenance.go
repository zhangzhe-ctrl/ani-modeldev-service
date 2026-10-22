package runtimeproof

import (
	"context"
	"reflect"
	"time"

	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/biz"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ biz.ComputeCleanupMaintenanceBoundary = (*Verifier)(nil)

func (verifier *Verifier) VerifyComputeMaintenanceWriters(ctx context.Context, record biz.OperationRecord) error {
	if !biz.ComputeCleanupMaintenanceEligible(record) {
		return biz.ErrCleanupBlocked
	}
	return verifier.verifyComputeCleanupGraph(ctx, record.Execution, *record.Authority, record.Runtime, true)
}

// This is the separately reserved normal operator plan, never another automatic
// attempt or a renewal of its expired deadline. Every original fence remains.
func (verifier *Verifier) DeleteComputeMaintenanceResource(ctx context.Context, record biz.OperationRecord, target biz.RuntimeResource) error {
	plan, err := biz.CleanupPlanFor(record)
	if err != nil || record.Cleanup == nil || record.Cleanup.Phase != "STARTED" || record.Cleanup.PlanHash != plan.PlanHash {
		return biz.ErrCleanupBlocked
	}
	known := false
	for _, expected := range plan.Targets {
		known = known || reflect.DeepEqual(target, expected)
	}
	if !known {
		return biz.ErrCleanupConflict
	}
	bounded, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	if err := verifier.VerifyComputeMaintenanceWriters(bounded, record); err != nil {
		return err
	}
	return verifier.deleteComputeTarget(bounded, record, target)
}

// ObserveComputeMaintenanceAbsent performs GETs only. A still-present original
// or a same-name replacement cannot resolve the exhausted automatic receipt.
func (verifier *Verifier) ObserveComputeMaintenanceAbsent(ctx context.Context, record biz.OperationRecord) ([]biz.RuntimeResource, error) {
	if err := verifier.VerifyComputeMaintenanceWriters(ctx, record); err != nil {
		return nil, err
	}
	absent := []biz.RuntimeResource{}
	for _, target := range record.Runtime.ComputeCleanup.Targets {
		resource, valid := computeResource(target)
		if !valid {
			return nil, biz.ErrCleanupConflict
		}
		current, err := verifier.kube.Resource(resource).Namespace(target.Namespace).Get(ctx, target.Name, metav1.GetOptions{})
		if !apierrors.IsNotFound(err) {
			if err != nil {
				return nil, biz.ErrCleanupUncertain
			}
			if !sameObject(current, target.APIVersion, target.Kind, target.Namespace, target.Name, target.UID) {
				return nil, biz.ErrCleanupConflict
			}
			return nil, biz.ErrCleanupUncertain
		}
		target.APIObjectPresent = false
		absent = append(absent, target)
	}
	return absent, nil
}
