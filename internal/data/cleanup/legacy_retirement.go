package cleanup

import (
	"context"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/biz"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// DeleteLegacyRetirementComputeController is unavailable to the ordinary
// automatic close interface. It replaces only the unavailable-log gate with
// the immutable, finite maintenance journal; all original parent, UID/RV,
// suspension, propagation and observed-absence guards remain shared.
func (adapter *Adapter) DeleteLegacyRetirementComputeController(ctx context.Context, record biz.OperationRecord, target biz.RuntimeResource) error {
	if adapter == nil || adapter.kube == nil || ctx == nil || record.Authority == nil || !biz.LegacyRetirementCleanupAuthorized(record.Execution, *record.Authority, record.Runtime) {
		return biz.ErrCleanupBlocked
	}
	targets, _, err := biz.ComputeCleanupTargets(record.Execution, *record.Authority, record.Runtime)
	if err != nil {
		return err
	}
	plan := biz.CleanupPlan{NamespaceUID: record.Authority.NamespaceUID, Targets: []biz.RuntimeResource{}}
	for _, resource := range targets {
		if resource.Kind != "Workflow" && resource.Kind != "Pod" {
			plan.Targets = append(plan.Targets, resource)
		}
	}
	return adapter.deleteController(ctx, record, target, plan, true)
}

// ValidateLegacyRetirementCleanupPlan performs only current GETs. It checks the
// actual controller-stop acknowledgements needed by the later shared delete
// path, without pretending that CLOSED or any cleanup intent already committed.
func (adapter *Adapter) ValidateLegacyRetirementCleanupPlan(ctx context.Context, execution biz.Execution, owner biz.RunAuthorityCandidate, state biz.ExecutionRuntime, evidence biz.ManagedCloseEvidence, observation biz.TrainingRuntimeObservation, stopFence biz.LegacyRetirementStopFence) error {
	if adapter == nil || adapter.kube == nil || !biz.ValidLegacyRetirementStopFence(execution, owner, state, stopFence) {
		return biz.ErrCleanupBlocked
	}
	pods := adapter.kube.Resource(schema.GroupVersionResource{Version: "v1", Resource: "pods"}).Namespace(owner.NamespaceName)
	for _, fact := range evidence.Resources {
		pod, err := pods.Get(ctx, fact.Name, metav1.GetOptions{})
		if err != nil || pod == nil || string(pod.GetUID()) != fact.UID || pod.GetResourceVersion() == "" {
			return biz.ErrCleanupConflict
		}
		for _, f := range pod.GetFinalizers() {
			if f != "modeldev.ani.io/step-exit-evidence" {
				return biz.ErrCleanupBlocked
			}
		}
	}
	if state.Training == nil {
		return nil
	}
	if state.TrainingHandle == nil || !observation.WritersAbsent || observation.Handle != *state.TrainingHandle {
		return biz.ErrCleanupBlocked
	}
	state.Observation = &observation
	plan := biz.CleanupPlan{NamespaceUID: owner.NamespaceUID, Targets: append([]biz.RuntimeResource{}, observation.Resources...)}
	record := biz.OperationRecord{QueryRecord: biz.QueryRecord{Execution: execution, Runtime: state}, Authority: &owner}
	for _, fact := range observation.Resources {
		resource, ok := targetResource(fact)
		if fact.Kind == "Pod" && fact.APIVersion == "v1" {
			resource = schema.GroupVersionResource{Version: "v1", Resource: "pods"}
			ok = true
		}
		if fact.Kind == "PodGroup" && fact.APIVersion == "scheduling.volcano.sh/v1beta1" {
			resource = schema.GroupVersionResource{Group: "scheduling.volcano.sh", Version: "v1beta1", Resource: "podgroups"}
			ok = true
		}
		if !ok {
			return biz.ErrCleanupConflict
		}
		object, err := adapter.kube.Resource(resource).Namespace(fact.Namespace).Get(ctx, fact.Name, metav1.GetOptions{})
		if err != nil || object == nil || !exactObject(object, fact) || object.GetResourceVersion() == "" {
			return biz.ErrCleanupConflict
		}
		for _, f := range object.GetFinalizers() {
			if fact.Kind != "Pod" || f != "modeldev.ani.io/training-exit-evidence" {
				return biz.ErrCleanupBlocked
			}
		}
		if fact.Kind == "Pod" {
			continue
		}
		if object.GetDeletionTimestamp() != nil {
			return biz.ErrCleanupBlocked
		}
		if fact.Kind == "PodGroup" {
			continue
		}
		if !creationStopped(object) {
			return biz.ErrCleanupBlocked
		}
		if fact.Kind == "TrainJob" {
			if err := adapter.verifyOriginalUnsuspendedSets(ctx, plan, fact, object); err != nil {
				if err := adapter.verifyOriginalSuspendedSetsForPlanAtFence(ctx, record, plan, fact, object, false, stopFence.RequestedAt); err != nil {
					return err
				}
			}
		}
	}
	return nil
}
