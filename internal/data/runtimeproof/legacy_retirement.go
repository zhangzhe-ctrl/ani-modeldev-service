package runtimeproof

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/biz"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/data/kfp"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// legacyRetirementWorkflow is constructed only after the immutable, original
// execution/owner/archive allowlist has been checked. It is never a Kubernetes
// client, and cannot make an archived owner appear in a live API GET.
type legacyRetirementWorkflow struct {
	workflow            *unstructured.Unstructured
	imageContractDenial bool
}

// The maintenance plan reports only these source-defined qualification stages.
// The normal runtime retains its original error values and response behavior.
type legacyRetirementOwnerFailure struct {
	stage string
	cause error
	facts *LegacyRetirementOwnerFacts
}

// Only public identity/process metadata is eligible for a failed plan receipt.
// No object body, environment, log, Secret data or external message is retained.
type LegacyRetirementOwnerFacts struct {
	APIVersion, Kind, Namespace, Name, UID, ResourceVersion string
	Phase, NodeID, ServiceAccountName, APIReason            string
	Deleting, APIReadFailed                                 bool
}

func (e *legacyRetirementOwnerFailure) Error() string { return e.stage + ": " + e.cause.Error() }
func (e *legacyRetirementOwnerFailure) Unwrap() error { return e.cause }

func LegacyRetirementOwnerFailureStage(err error) string {
	var failure *legacyRetirementOwnerFailure
	if errors.As(err, &failure) {
		return failure.stage
	}
	return ""
}

func LegacyRetirementOwnerFailureFacts(err error) *LegacyRetirementOwnerFacts {
	var failure *legacyRetirementOwnerFailure
	if errors.As(err, &failure) && failure.facts != nil {
		copy := *failure.facts
		return &copy
	}
	return nil
}

func ownerQualificationFailure(archive *legacyRetirementWorkflow, stage string, cause error) error {
	if archive == nil {
		return cause
	}
	return &legacyRetirementOwnerFailure{stage: stage, cause: cause}
}

func ownerObjectQualificationFailure(archive *legacyRetirementWorkflow, stage string, object *unstructured.Unstructured, readErr, cause error) error {
	if archive == nil {
		return cause
	}
	facts := &LegacyRetirementOwnerFacts{APIReadFailed: readErr != nil}
	if readErr != nil {
		switch reason := apierrors.ReasonForError(readErr); reason {
		case "Forbidden", "Unauthorized", "NotFound", "Timeout", "ServerTimeout", "Invalid", "Conflict", "AlreadyExists":
			facts.APIReason = string(reason)
		default:
			facts.APIReason = "Unknown"
		}
	}
	if object != nil {
		facts.APIVersion, facts.Kind = object.GetAPIVersion(), object.GetKind()
		facts.Namespace, facts.Name, facts.UID, facts.ResourceVersion = object.GetNamespace(), object.GetName(), string(object.GetUID()), object.GetResourceVersion()
		facts.Phase, _ = textAt(object, "status", "phase")
		facts.NodeID = object.GetAnnotations()["workflows.argoproj.io/node-id"]
		facts.ServiceAccountName, _ = textAt(object, "spec", "serviceAccountName")
		facts.Deleting = object.GetDeletionTimestamp() != nil
	}
	return &legacyRetirementOwnerFailure{stage: stage, cause: cause, facts: facts}
}

// VerifyLegacyRetirementOwner is the explicitly invoked, one-time maintenance
// boundary. The normal OwnerWriterVerifier interface does not invoke it. The
// caller must persist the returned archive receipt beside the close evidence;
// WorkflowResourceVersion in that evidence is the archived version, never a
// claim that the removed Workflow was observed through the current API.
func (verifier *Verifier) VerifyLegacyRetirementOwner(ctx context.Context, execution biz.Execution, authority biz.RunAuthorityCandidate, workspace *biz.WorkspaceBinding, archiveBytes []byte) (biz.ManagedCloseEvidence, biz.LegacyRetirementArchiveReceipt, error) {
	scope, allowed := biz.LookupLegacyRetirementScope(execution.ExecutionID)
	if !allowed || !biz.LegacyRetirementScopeMatches(execution, authority) || contentSHA(archiveBytes) != scope.ArchiveSHA256 {
		return biz.ManagedCloseEvidence{}, biz.LegacyRetirementArchiveReceipt{}, ownerQualificationFailure(&legacyRetirementWorkflow{}, "ORIGINAL_SCOPE_AND_ARCHIVE_SHA", biz.ErrRuntimeConflict)
	}
	archive, err := parseLegacyRetirementWorkflow(archiveBytes, scope)
	if err != nil {
		return biz.ManagedCloseEvidence{}, biz.LegacyRetirementArchiveReceipt{}, ownerQualificationFailure(&legacyRetirementWorkflow{}, "ARCHIVE_DECODE_IDENTITY", err)
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
