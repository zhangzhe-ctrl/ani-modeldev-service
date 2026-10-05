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
	if verifier == nil || verifier.kube == nil || verifier.runs == nil || ctx == nil || ctx.Err() != nil {
		return biz.RunAuthorityCandidate{}, biz.ErrRuntimeNotReady
	}
	plan := dispatch.Plan
	if plan.TenantID != execution.TenantID || plan.ExecutionID != execution.ExecutionID || plan.OperationID != execution.OperationID || plan.SpecHash != execution.SpecHash || plan.Environment != execution.Snapshot.Environment {
		return biz.RunAuthorityCandidate{}, biz.ErrRuntimeConflict
	}
	if runID == "" {
		var err error
		runID, err = verifier.runs.FindClosingRun(ctx, plan)
		if err != nil {
			return biz.RunAuthorityCandidate{}, err
		}
	}
	run, err := verifier.runs.GetManagedRun(ctx, plan, runID)
	if err != nil {
		return biz.RunAuthorityCandidate{}, err
	}
	env := plan.Environment
	namespace, err := verifier.kube.Resource(schema.GroupVersionResource{Version: "v1", Resource: "namespaces"}).Get(ctx, env.NamespaceName, metav1.GetOptions{})
	if err != nil || !sameObject(namespace, "v1", "Namespace", "", env.NamespaceName, env.NamespaceUID) || namespace.GetDeletionTimestamp() != nil {
		return biz.RunAuthorityCandidate{}, biz.ErrRuntimeNotReady
	}
	candidate := biz.RunAuthorityCandidate{TenantID: execution.TenantID, ExecutionID: execution.ExecutionID, OperationID: execution.OperationID, SpecHash: execution.SpecHash, AttemptID: dispatch.AttemptID, PlanHash: dispatch.PlanHash, RunID: runID, NamespaceName: env.NamespaceName, NamespaceUID: env.NamespaceUID}
	// KFP child PodName fields are Argo node IDs and can also refer to DAG
	// successors or omitted stages. Enumerate actual Pods independently, then
	// resolve their controller graph rather than reading node IDs as Pod names.
	references := make(map[string]bool)
	for _, task := range run.Tasks {
		if task.RunID != runID {
			return biz.RunAuthorityCandidate{}, biz.ErrRuntimeConflict
		}
		if task.PodName != "" {
			references[task.PodName] = true
		}
		for _, child := range task.ChildTasks {
			if child.PodName != "" {
				references[child.PodName] = true
			}
		}
	}
	pods, err := verifier.kube.Resource(schema.GroupVersionResource{Version: "v1", Resource: "pods"}).Namespace(env.NamespaceName).List(ctx, metav1.ListOptions{})
	if err != nil || pods == nil || pods.GetContinue() != "" {
		return biz.RunAuthorityCandidate{}, biz.ErrRuntimeNotReady
	}
	for i := range pods.Items {
		pod := &pods.Items[i]
		if !references[pod.GetName()] && !references[pod.GetAnnotations()["workflows.argoproj.io/node-id"]] {
			continue
		}
		account, _ := textAt(pod, "spec", "serviceAccountName")
		if pod.GetUID() == "" || pod.GetName() == "" || pod.GetNamespace() != env.NamespaceName || pod.GetAPIVersion() != "v1" || pod.GetKind() != "Pod" || account != env.Identities.KFPStepServiceAccount {
			return biz.RunAuthorityCandidate{}, biz.ErrRuntimeConflict
		}
		controllers := 0
		for _, owner := range pod.GetOwnerReferences() {
			if owner.Controller == nil || !*owner.Controller {
				continue
			}
			controllers++
			if owner.APIVersion != "argoproj.io/v1alpha1" || owner.Kind != "Workflow" || owner.Name == "" || owner.UID == "" {
				return biz.RunAuthorityCandidate{}, biz.ErrRuntimeConflict
			}
			if candidate.WorkflowUID != "" && (candidate.WorkflowName != owner.Name || candidate.WorkflowUID != string(owner.UID)) {
				return biz.RunAuthorityCandidate{}, biz.ErrRunAuthorityConflict
			}
			candidate.WorkflowName, candidate.WorkflowUID = owner.Name, string(owner.UID)
		}
		if controllers != 1 {
			return biz.RunAuthorityCandidate{}, biz.ErrRuntimeConflict
		}
	}
	if candidate.WorkflowUID == "" {
		return biz.RunAuthorityCandidate{}, biz.ErrRuntimeNotReady
	}
	workflow, err := verifier.kube.Resource(schema.GroupVersionResource{Group: "argoproj.io", Version: "v1alpha1", Resource: "workflows"}).Namespace(env.NamespaceName).Get(ctx, candidate.WorkflowName, metav1.GetOptions{})
	if err != nil || !sameObject(workflow, "argoproj.io/v1alpha1", "Workflow", env.NamespaceName, candidate.WorkflowName, candidate.WorkflowUID) || workflow.GetDeletionTimestamp() != nil {
		return biz.RunAuthorityCandidate{}, biz.ErrRuntimeNotReady
	}
	if _, err := resolvedManagedTasks(workflow, run.Tasks, pods.Items); err != nil {
		return biz.RunAuthorityCandidate{}, err
	}
	return candidate, nil
}
