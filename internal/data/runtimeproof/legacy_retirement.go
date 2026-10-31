package runtimeproof

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/biz"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/data/kfp"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// legacyRetirementWorkflow is constructed only after the immutable, original
// execution/owner/archive allowlist has been checked. It is never a Kubernetes
// client, and cannot make an archived owner appear in a live API GET.
type legacyRetirementWorkflow struct {
	workflow            *unstructured.Unstructured
	imageContractDenial bool
}

// VerifyLegacyRetirementOwner is the explicitly invoked, one-time maintenance
// boundary. The normal OwnerWriterVerifier interface does not invoke it. The
// caller must persist the returned archive receipt beside the close evidence;
// WorkflowResourceVersion in that evidence is the archived version, never a
// claim that the removed Workflow was observed through the current API.
func (verifier *Verifier) VerifyLegacyRetirementOwner(ctx context.Context, execution biz.Execution, authority biz.RunAuthorityCandidate, workspace *biz.WorkspaceBinding, archiveBytes []byte) (biz.ManagedCloseEvidence, biz.LegacyRetirementArchiveReceipt, error) {
	scope, allowed := biz.LookupLegacyRetirementScope(execution.ExecutionID)
	if !allowed || !biz.LegacyRetirementScopeMatches(execution, authority) || contentSHA(archiveBytes) != scope.ArchiveSHA256 {
		return biz.ManagedCloseEvidence{}, biz.LegacyRetirementArchiveReceipt{}, biz.ErrRuntimeConflict
	}
	archive, err := parseLegacyRetirementWorkflow(archiveBytes, scope)
	if err != nil {
		return biz.ManagedCloseEvidence{}, biz.LegacyRetirementArchiveReceipt{}, err
	}
	evidence, err := verifier.verifyOwnerWritersAbsent(ctx, execution, authority, workspace, archive)
	if err != nil {
		return biz.ManagedCloseEvidence{}, biz.LegacyRetirementArchiveReceipt{}, err
	}
	receipt := biz.LegacyRetirementArchiveReceipt{
		Source:      biz.LegacyRetirementArchiveSource,
		ExecutionID: scope.ExecutionID, NamespaceName: scope.NamespaceName, NamespaceUID: scope.NamespaceUID,
		RunID: scope.RunID, WorkflowName: scope.WorkflowName, WorkflowUID: scope.WorkflowUID,
		SHA256: scope.ArchiveSHA256, ArchivedResourceVersion: archive.workflow.GetResourceVersion(), ObservedAt: evidence.ObservedAt,
	}
	return evidence, receipt, nil
}

func parseLegacyRetirementWorkflow(raw []byte, scope biz.LegacyRetirementScope) (*legacyRetirementWorkflow, error) {
	if contentSHA(raw) != scope.ArchiveSHA256 {
		return nil, biz.ErrRuntimeConflict
	}
	var object map[string]any
	if json.Unmarshal(raw, &object) != nil || object == nil {
		return nil, biz.ErrRuntimeConflict
	}
	workflow := &unstructured.Unstructured{Object: object}
	// The official KFP v1 stored Workflow omits TypeMeta. Supply its known
	// type only in this local typed view; the hash binds the untouched bytes.
	if workflow.GetAPIVersion() == "" {
		workflow.SetAPIVersion("argoproj.io/v1alpha1")
	}
	if workflow.GetKind() == "" {
		workflow.SetKind("Workflow")
	}
	if !sameObject(workflow, "argoproj.io/v1alpha1", "Workflow", scope.NamespaceName, scope.WorkflowName, scope.WorkflowUID) || workflow.GetDeletionTimestamp() != nil || workflow.GetResourceVersion() == "" || workflow.GetLabels()["pipeline/runid"] != scope.RunID {
		return nil, biz.ErrRuntimeConflict
	}
	return &legacyRetirementWorkflow{workflow: workflow, imageContractDenial: scope.ExecutionID == "eca4d47b-5911-4f0f-8b45-89c1be775925"}, nil
}

func (archive *legacyRetirementWorkflow) deniedNode(workflow *unstructured.Unstructured, node map[string]any) bool {
	if serverDeniedAdmissionNode(workflow, node) {
		return true
	}
	return archive.imageContractDenial && legacyImageContractDeniedNode(workflow, node)
}

func (archive *legacyRetirementWorkflow) rejectedTask(workflow *unstructured.Unstructured, task kfp.ManagedTask, tasks []kfp.ManagedTask) bool {
	return serverRejectedManagedTaskWithDeniedNode(workflow, task, tasks, archive.deniedNode)
}

// The approved image_contract exception is precisely a rejected executor
// Create before storage. Every original process field must be absent and the
// existing full zero-retry, parent/child, template and task-reference checks
// remain mandatory. This predicate is unavailable to normal owner verification.
func legacyImageContractDeniedNode(workflow *unstructured.Unstructured, node map[string]any) bool {
	if workflow == nil || node["type"] != "Pod" || node["phase"] != "Error" || node["templateScope"] != "local/"+workflow.GetName() || node["progress"] != "0/1" || node["taskResultSynced"] != true || !admissionRejectionWithoutProcess(node) {
		return false
	}
	name, nameOK := node["name"].(string)
	id, idOK := node["id"].(string)
	if !nameOK || !idOK || !strings.HasPrefix(name, workflow.GetName()+".") || !strings.HasPrefix(id, workflow.GetName()+"-") || workflow.GetNamespace() == "" {
		return false
	}
	executor := node["templateName"] == "metadata-0-2-retry-system-container-impl" || node["templateName"] == "metadata-1-2-retry-system-container-impl"
	message := "admission webhook \"" + workflow.GetNamespace() + ".modeldev.ani.io\" denied the request: ModelDev Pod admission refused: image_contract"
	return executor && node["message"] == message && rejectedExecutorHasZeroRetries(workflow, node)
}
