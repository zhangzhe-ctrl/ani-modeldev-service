package runtimeproof

import (
	"context"

	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/biz"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// VerifyClosingRun recovers only the frozen owner association from independent
// KFP and Kubernetes reads. It does not authenticate a late managed-step call.
func (verifier *Verifier) VerifyClosingRun(ctx context.Context, execution biz.Execution, dispatch biz.PipelineDispatch, runID string) (biz.RunAuthorityCandidate, error) {
	if verifier == nil || verifier.kube == nil || verifier.runs == nil || ctx == nil { return biz.RunAuthorityCandidate{}, biz.ErrRuntimeNotReady }
	plan := dispatch.Plan
	if plan.TenantID != execution.TenantID || plan.ExecutionID != execution.ExecutionID || plan.OperationID != execution.OperationID || plan.SpecHash != execution.SpecHash || plan.Environment != execution.Snapshot.Environment { return biz.RunAuthorityCandidate{}, biz.ErrRuntimeConflict }
	if runID == "" {
		var err error
		runID, err = verifier.runs.FindClosingRun(ctx, plan)
		if err != nil { return biz.RunAuthorityCandidate{}, err }
	}
	run, err := verifier.runs.GetManagedRun(ctx, plan, runID)
	if err != nil { return biz.RunAuthorityCandidate{}, err }
	env := plan.Environment
	namespace, err := verifier.kube.Resource(schema.GroupVersionResource{Version: "v1", Resource: "namespaces"}).Get(ctx, env.NamespaceName, metav1.GetOptions{})
	if err != nil || !sameObject(namespace, "v1", "Namespace", "", env.NamespaceName, env.NamespaceUID) || namespace.GetDeletionTimestamp() != nil { return biz.RunAuthorityCandidate{}, biz.ErrRuntimeNotReady }
	candidate := biz.RunAuthorityCandidate{TenantID: execution.TenantID, ExecutionID: execution.ExecutionID, OperationID: execution.OperationID, SpecHash: execution.SpecHash, AttemptID: dispatch.AttemptID, PlanHash: dispatch.PlanHash, RunID: runID, NamespaceName: env.NamespaceName, NamespaceUID: env.NamespaceUID}
	for _, task := range run.Tasks {
		if task.PodName == "" { continue }
		pod, err := verifier.kube.Resource(schema.GroupVersionResource{Version: "v1", Resource: "pods"}).Namespace(env.NamespaceName).Get(ctx, task.PodName, metav1.GetOptions{})
		if err != nil || pod == nil { return biz.RunAuthorityCandidate{}, biz.ErrRuntimeNotReady }
		account, _ := textAt(pod, "spec", "serviceAccountName")
		if task.RunID != runID || pod.GetUID() == "" || pod.GetName() != task.PodName || pod.GetNamespace() != env.NamespaceName || pod.GetAPIVersion() != "v1" || pod.GetKind() != "Pod" || account != env.Identities.KFPStepServiceAccount { return biz.RunAuthorityCandidate{}, biz.ErrRuntimeConflict }
		controllers := 0
		for _, owner := range pod.GetOwnerReferences() {
			if owner.Controller == nil || !*owner.Controller { continue }
			controllers++
			if owner.APIVersion != "argoproj.io/v1alpha1" || owner.Kind != "Workflow" || owner.Name == "" || owner.UID == "" { return biz.RunAuthorityCandidate{}, biz.ErrRuntimeConflict }
			if candidate.WorkflowUID != "" && (candidate.WorkflowName != owner.Name || candidate.WorkflowUID != string(owner.UID)) { return biz.RunAuthorityCandidate{}, biz.ErrRunAuthorityConflict }
			candidate.WorkflowName, candidate.WorkflowUID = owner.Name, string(owner.UID)
		}
		if controllers != 1 { return biz.RunAuthorityCandidate{}, biz.ErrRuntimeConflict }
	}
	if candidate.WorkflowUID == "" { return biz.RunAuthorityCandidate{}, biz.ErrRuntimeNotReady }
	workflow, err := verifier.kube.Resource(schema.GroupVersionResource{Group: "argoproj.io", Version: "v1alpha1", Resource: "workflows"}).Namespace(env.NamespaceName).Get(ctx, candidate.WorkflowName, metav1.GetOptions{})
	if err != nil || !sameObject(workflow, "argoproj.io/v1alpha1", "Workflow", env.NamespaceName, candidate.WorkflowName, candidate.WorkflowUID) || workflow.GetDeletionTimestamp() != nil { return biz.RunAuthorityCandidate{}, biz.ErrRuntimeNotReady }
	return candidate, nil
}
