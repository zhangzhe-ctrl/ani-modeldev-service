package runtimeproof

import (
	"context"
	"crypto/x509"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/zhangzhe-ctrl/ani-modeldev-service/contract/cpup01"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/contract/cpup01/conformance"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/biz"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/data/kfp"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/testsupport/admissionfixture"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	ktesting "k8s.io/client-go/testing"
)

// Only external KFP/Kubernetes records are doubled. This exercises the same
// production FrozenPlan, authenticated Run, completion graph, task resolution
// and current writer checks as ordinary close. Synthetic archives are passed
// only to the private helper: the public method always requires the real fixed
// allowlist hash and has no injectable inventory or policy.
func TestLegacyRetirementArchiveOwnerKeepsCurrentWriterProof(t *testing.T) {
	for _, attack := range []string{"none", "historical-pod-missing", "running-pod", "namespace-uid", "live-workflow", "workflow-reappears", "list-truncated", "wrong-run", "rejected-node-pod"} {
		t.Run(attack, func(t *testing.T) {
			f := newLegacyOwnerFixture(t)
			if attack == "historical-pod-missing" {
				f.pods = f.pods[1:]
			}
			if attack == "running-pod" {
				f.pods[0].Object["status"] = map[string]any{"phase": "Running"}
			}
			if attack == "wrong-run" {
				f.run["display_name"] = "other-execution"
			}
			if attack == "rejected-node-pod" {
				node := addLegacyImageDenial(t, f)
				pod := f.pods[0].DeepCopy()
				pod.SetName("unexpected-persisted-rejected-pod")
				pod.SetUID("unexpected-uid")
				pod.SetOwnerReferences(nil)
				pod.SetAnnotations(map[string]string{"workflows.argoproj.io/node-id": node["id"].(string)})
				f.pods = append(f.pods, *pod)
			}
			objects := []runtime.Object{f.namespace}
			if attack == "namespace-uid" {
				f.namespace.SetUID("replacement-namespace")
			}
			if attack == "live-workflow" {
				objects = append(objects, f.workflow)
			}
			for i := range f.pods {
				objects = append(objects, &f.pods[i])
			}
			kube := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), map[schema.GroupVersionResource]string{{Version: "v1", Resource: "pods"}: "PodList"}, objects...)
			workflowGets := 0
			kube.PrependReactor("get", "workflows", func(action ktesting.Action) (bool, runtime.Object, error) {
				workflowGets++
				if attack == "workflow-reappears" && workflowGets == 2 {
					return true, f.workflow.DeepCopy(), nil
				}
				return false, nil, nil
			})
			if attack == "list-truncated" {
				kube.PrependReactor("list", "pods", func(ktesting.Action) (bool, runtime.Object, error) {
					return true, &unstructured.UnstructuredList{Object: map[string]any{"metadata": map[string]any{"continue": "next-page"}}}, nil
				})
			}
			verifier := New(kube, f.runs, nil, legacyOwnerPlans{f.dispatch})
			before, _ := json.Marshal(f.workflow.Object)
			evidence, err := verifier.verifyOwnerWritersAbsent(context.Background(), f.execution, f.owner, nil, &legacyRetirementWorkflow{workflow: f.workflow, imageContractDenial: true})
			if attack != "none" {
				if err == nil || evidence.OwnerTermination != nil {
					t.Fatalf("%s fabricated writer absence: %+v %v", attack, evidence, err)
				}
				return
			}
			if err != nil || evidence.OwnerTermination == nil || len(evidence.Resources) != len(f.pods) || workflowGets != 2 {
				t.Fatalf("archive owner must still prove real current writers: %+v %v gets=%d", evidence, err, workflowGets)
			}
			for _, resource := range evidence.Resources {
				if resource.Kind != "Pod" || !resource.APIObjectPresent || resource.ExitCode == nil {
					t.Fatal("archive invented a live Workflow or process exit", resource)
				}
			}
			after, _ := json.Marshal(f.workflow.Object)
			if string(before) != string(after) || evidence.OwnerTermination.WorkflowResourceVersion != f.workflow.GetResourceVersion() || evidence.OwnerTermination.WorkflowNodesSHA256 == "" {
				t.Fatal("archive graph was rewritten or provenance lost")
			}
			// The ordinary API still rejects the very same missing owner.
			if evidence, err := verifier.VerifyOwnerWritersAbsent(context.Background(), f.execution, f.owner, nil); err == nil || evidence.OwnerTermination != nil {
				t.Fatal("archive maintenance changed default live owner requirement")
			}
		})
	}
}

func TestLegacyRetirementImageContractRequiresExactOriginalNoncreationShape(t *testing.T) {
	for _, attack := range []string{"none", "reason", "scope", "template", "retry-one", "missing-parent", "parent-child", "template-retry", "task-reference", "same-node-pod", "hostNodeName", "podIP", "podName", "podUID", "containerID", "containerStatuses", "outputs", "resourcesDuration", "exitCode", "daemoned", "memoizationStatus"} {
		t.Run(attack, func(t *testing.T) {
			f := newLegacyOwnerFixture(t)
			node := addLegacyImageDenial(t, f)
			nodes, _, _ := unstructured.NestedMap(f.workflow.Object, "status", "nodes")
			parentID := f.workflow.GetName() + "-denied-retry"
			switch attack {
			case "reason":
				node["message"] = "admission webhook \"" + f.workflow.GetNamespace() + ".modeldev.ani.io\" denied the request: ModelDev Pod admission refused: arbitrary_reason"
			case "scope":
				node["templateScope"] = "local/other"
			case "template":
				node["templateName"] = "other-impl"
			case "retry-one":
				node["inputs"] = map[string]any{"parameters": []any{map[string]any{"name": "retry-max-count", "value": "1"}}}
			case "missing-parent":
				delete(nodes, parentID)
			case "parent-child":
				nodes[parentID].(map[string]any)["children"] = []any{"other-attempt"}
			case "template-retry":
				f.workflow.Object["spec"].(map[string]any)["retryStrategy"] = map[string]any{"limit": "1"}
			case "task-reference":
				f.run["run_details"].(map[string]any)["task_details"] = []any{}
			case "same-node-pod":
				f.pods[0].SetAnnotations(map[string]string{"workflows.argoproj.io/node-id": node["id"].(string)})
			case "none":
			default:
				node[attack] = "must-remain-absent"
			}
			nodes[node["id"].(string)] = node
			_ = unstructured.SetNestedMap(f.workflow.Object, nodes, "status", "nodes")
			archive := &legacyRetirementWorkflow{workflow: f.workflow, imageContractDenial: true}
			if serverDeniedAdmissionNode(f.workflow, node) {
				t.Fatal("ordinary admission policy accepted image_contract exception")
			}
			objects := []runtime.Object{f.namespace}
			for i := range f.pods {
				objects = append(objects, &f.pods[i])
			}
			kube := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), map[schema.GroupVersionResource]string{{Version: "v1", Resource: "pods"}: "PodList"}, objects...)
			evidence, err := New(kube, f.runs, nil, legacyOwnerPlans{f.dispatch}).verifyOwnerWritersAbsent(context.Background(), f.execution, f.owner, nil, archive)
			if attack == "none" {
				if err != nil || len(evidence.Resources) != len(f.pods) {
					t.Fatalf("exact denied Create must retain only real exits: %+v %v", evidence, err)
				}
			} else if err == nil || evidence.OwnerTermination != nil {
				t.Fatalf("%s became a fake process exit: %+v %v", attack, evidence, err)
			}
		})
	}
}

func TestLegacyRetirementPublicBoundaryRejectsUnapprovedArchive(t *testing.T) {
	for _, id := range []string{"aaaaaaaa-bbbb-4ccc-8ddd-eeeeeeeeeeee", "1cce950b-d6e9-453e-aa11-fffd8846224b", "4a1a92f7-4d94-439d-a8d7-1833f38c7dbc", "eca4d47b-5911-4f0f-8b45-89c1be775925"} {
		var verifier *Verifier
		execution := biz.Execution{Admission: biz.Admission{ExecutionID: id}}
		evidence, receipt, err := verifier.VerifyLegacyRetirementOwner(context.Background(), execution, biz.RunAuthorityCandidate{}, nil, []byte(`{"kind":"Workflow"}`))
		if !errors.Is(err, biz.ErrRuntimeConflict) || evidence.OwnerTermination != nil || !reflect.DeepEqual(receipt, biz.LegacyRetirementArchiveReceipt{}) {
			t.Fatal("unapproved identity or archive passed immutable public gate", id, err)
		}
	}
}

func TestLegacyRetirementArchiveParsingRetainsRawHashAndArchivedVersion(t *testing.T) {
	f := newLegacyOwnerFixture(t)
	delete(f.workflow.Object, "apiVersion")
	delete(f.workflow.Object, "kind")
	labels := f.workflow.GetLabels()
	labels["pipeline/runid"] = f.owner.RunID
	f.workflow.SetLabels(labels)
	raw, _ := json.Marshal(f.workflow.Object)
	scope := biz.LegacyRetirementScope{ExecutionID: f.execution.ExecutionID, NamespaceName: f.owner.NamespaceName, NamespaceUID: f.owner.NamespaceUID, RunID: f.owner.RunID, WorkflowName: f.owner.WorkflowName, WorkflowUID: f.owner.WorkflowUID, ArchiveSHA256: contentSHA(raw)}
	archive, err := parseLegacyRetirementWorkflow(raw, scope)
	if err != nil || archive.workflow.GetKind() != "Workflow" || archive.workflow.GetResourceVersion() != f.workflow.GetResourceVersion() || contentSHA(raw) != scope.ArchiveSHA256 {
		t.Fatal("official omitted TypeMeta or original raw provenance lost", err)
	}
	for _, attack := range []string{"hash", "uid", "name", "namespace", "run"} {
		s := scope
		switch attack {
		case "hash":
			s.ArchiveSHA256 = strings.Repeat("a", 64)
		case "uid":
			s.WorkflowUID = "other"
		case "name":
			s.WorkflowName = "other"
		case "namespace":
			s.NamespaceName = "other"
		case "run":
			s.RunID = "other"
		}
		if _, err := parseLegacyRetirementWorkflow(raw, s); err == nil {
			t.Fatal("wrong archive identity accepted", attack)
		}
	}
}

type legacyOwnerFixture struct {
	execution           biz.Execution
	dispatch            biz.PipelineDispatch
	owner               biz.RunAuthorityCandidate
	run                 map[string]any
	workflow, namespace *unstructured.Unstructured
	pods                []unstructured.Unstructured
	runs                *kfp.Client
}
type legacyOwnerPlans struct{ dispatch biz.PipelineDispatch }

func (p legacyOwnerPlans) Get(context.Context, string, string) (biz.PipelineDispatch, error) {
	return p.dispatch, nil
}

type legacyOwnerToken struct{}

func (legacyOwnerToken) BearerToken(context.Context, string, cpup01.EnvironmentBindingSnapshot) (string, error) {
	return "synthetic-owner-token", nil
}

func newLegacyOwnerFixture(t *testing.T) *legacyOwnerFixture {
	t.Helper()
	raw, err := os.ReadFile("testdata/kfp-2.16-failed-before-prepare.json")
	if err != nil {
		t.Fatal(err)
	}
	var record struct {
		Workflow map[string]any
		Pods     []map[string]any
		Run      map[string]any
	}
	if json.Unmarshal(raw, &record) != nil {
		t.Fatal("fixture JSON")
	}
	snapshot := conformance.SnapshotV1()
	intent := cpup01.Intent{Name: "legacy-owner-boundary", Kind: "GENERAL_TRAINING", ImplementationID: snapshot.Implementation.ImplementationID, ComputePresetID: snapshot.ComputeOrigin.ComputePresetID, MLP: &cpup01.MLPParameters{}, DatasetVersionID: snapshot.Input.InputVersionID}
	admission := admissionfixture.Bind(t, biz.Admission{TenantID: "11111111-2222-4333-8444-555555555555", Actor: "governance:synthetic-fixture", OperationID: "bbbbbbbb-cccc-4ddd-8eee-ffffffffffff", ExecutionID: "aaaaaaaa-bbbb-4ccc-8ddd-eeeeeeeeeeee", Intent: intent, Snapshot: snapshot, AcceptedAt: snapshot.DeadlineAt.Add(-time.Hour).UTC()})
	plan, err := (biz.PipelineDispatchRequest{Admission: admission, Owner: biz.PipelineOwnerConfiguration{Reference: "legacy-owner-fixture", RevisionSHA256: strings.Repeat("a", 64), PipelineRoot: "s3://fixture-kfp-artifacts/managed-root"}}).Freeze()
	if err != nil {
		t.Fatal(err)
	}
	hash, err := plan.Digest()
	if err != nil {
		t.Fatal(err)
	}
	f := &legacyOwnerFixture{execution: biz.Execution{Admission: admission}, workflow: &unstructured.Unstructured{Object: record.Workflow}, run: record.Run}
	f.dispatch = biz.PipelineDispatch{Plan: plan, PlanHash: hash, AttemptID: "66666666-7777-4888-8999-aaaaaaaaaaaa", State: biz.PipelineDispatchConfirmed, ConfirmedRuns: []biz.PipelineConfirmedRun{{RunID: record.Run["run_id"].(string)}}}
	env := admission.Snapshot.Environment
	f.workflow.SetNamespace(env.NamespaceName)
	at := time.Now().UTC().Add(-time.Minute).Format(time.RFC3339Nano)
	f.workflow.SetLabels(map[string]string{"workflows.argoproj.io/completed": "true", "pipeline/runid": record.Run["run_id"].(string)})
	_ = unstructured.SetNestedField(f.workflow.Object, at, "status", "finishedAt")
	nodes, _, _ := unstructured.NestedMap(f.workflow.Object, "status", "nodes")
	for _, raw := range nodes {
		raw.(map[string]any)["finishedAt"] = at
	}
	_ = unstructured.SetNestedMap(f.workflow.Object, nodes, "status", "nodes")
	f.run["finished_at"] = at
	f.run["experiment_id"], f.run["display_name"], f.run["service_account"] = env.ExperimentID, plan.DisplayName, env.Identities.KFPStepServiceAccount
	f.run["pipeline_version_reference"] = map[string]any{"pipeline_id": plan.PipelineID, "pipeline_version_id": plan.PipelineVersionID}
	f.run["runtime_config"] = map[string]any{"parameters": map[string]any{"execution_id": plan.ExecutionID, "spec_hash": plan.SpecHash}, "pipeline_root": plan.Owner.PipelineRoot}
	f.owner = biz.RunAuthorityCandidate{TenantID: admission.TenantID, ExecutionID: admission.ExecutionID, OperationID: admission.OperationID, SpecHash: admission.SpecHash, AttemptID: f.dispatch.AttemptID, PlanHash: hash, RunID: record.Run["run_id"].(string), NamespaceName: env.NamespaceName, NamespaceUID: env.NamespaceUID, WorkflowName: f.workflow.GetName(), WorkflowUID: string(f.workflow.GetUID())}
	f.namespace = &unstructured.Unstructured{Object: map[string]any{"apiVersion": "v1", "kind": "Namespace", "metadata": map[string]any{"name": env.NamespaceName, "uid": env.NamespaceUID}}}
	for _, raw := range record.Pods {
		pod := unstructured.Unstructured{Object: raw}
		pod.SetNamespace(env.NamespaceName)
		pod.Object["spec"] = map[string]any{"serviceAccountName": env.Identities.KFPStepServiceAccount, "containers": []any{map[string]any{"name": "main"}}}
		pod.Object["status"] = map[string]any{"phase": "Succeeded", "containerStatuses": []any{map[string]any{"name": "main", "state": map[string]any{"terminated": map[string]any{"exitCode": int64(0), "finishedAt": at}}}}}
		f.pods = append(f.pods, pod)
	}
	peer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/apis/v2beta1/runs/"+f.owner.RunID || r.Header.Get("Authorization") != "Bearer synthetic-owner-token" {
			t.Error("unbound Run read")
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(f.run)
	}))
	t.Cleanup(peer.Close)
	roots := x509.NewCertPool()
	roots.AddCert(peer.Certificate())
	f.runs, err = kfp.New(kfp.Config{ConnectionRef: env.KFPConnectionRef, Endpoint: peer.URL, RootCAs: roots, Timeout: 3 * time.Second}, legacyOwnerToken{})
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func addLegacyImageDenial(t *testing.T, f *legacyOwnerFixture) map[string]any {
	t.Helper()
	prefix := f.workflow.GetName()
	executorID, retryID, controllerID := prefix+"-denied-executor", prefix+"-denied-retry", prefix+"-denied-controller"
	finished, _, _ := unstructured.NestedString(f.workflow.Object, "status", "finishedAt")
	name := prefix + ".root.denied.executor"
	message := "admission webhook \"" + f.workflow.GetNamespace() + ".modeldev.ani.io\" denied the request: ModelDev Pod admission refused: image_contract"
	params := func() map[string]any {
		return map[string]any{"parameters": []any{map[string]any{"name": "retry-max-count", "value": "0"}}}
	}
	node := map[string]any{"id": executorID, "name": name + "(0)", "displayName": "executor(0)", "boundaryID": controllerID, "type": "Pod", "phase": "Error", "templateName": "metadata-0-2-retry-system-container-impl", "templateScope": "local/" + prefix, "startedAt": finished, "finishedAt": finished, "progress": "0/1", "taskResultSynced": true, "nodeFlag": map[string]any{"retried": true}, "inputs": params(), "message": message}
	nodes, _, _ := unstructured.NestedMap(f.workflow.Object, "status", "nodes")
	nodes[executorID] = node
	nodes[retryID] = map[string]any{"id": retryID, "name": name, "displayName": "executor", "boundaryID": controllerID, "type": "Retry", "phase": "Error", "templateName": node["templateName"], "templateScope": node["templateScope"], "startedAt": finished, "finishedAt": finished, "progress": "0/1", "children": []any{executorID}, "inputs": params(), "message": "task '" + name + "' errored: " + message}
	nodes[controllerID] = map[string]any{"id": controllerID, "name": prefix + ".root.denied", "displayName": "workspace-name", "type": "DAG", "phase": "Error", "templateName": "metadata-0-2-retry-system-container-executor", "templateScope": "local/" + prefix, "startedAt": finished, "finishedAt": finished, "progress": "0/1", "children": []any{retryID}, "outboundNodes": []any{executorID}}
	// Give the new rejected task a distinct managed role while retaining the
	// original captured workspace task and all its real executor exits.
	nodes[controllerID].(map[string]any)["displayName"] = "prepare"
	for id, raw := range nodes {
		if id != controllerID && raw.(map[string]any)["displayName"] == "prepare" {
			raw.(map[string]any)["displayName"] = "original-omitted-prepare"
		}
	}
	_ = unstructured.SetNestedMap(f.workflow.Object, nodes, "status", "nodes")
	f.workflow.Object["status"].(map[string]any)["phase"] = "Error"
	templates, _, _ := unstructured.NestedSlice(f.workflow.Object, "spec", "templates")
	var kept []any
	for _, raw := range templates {
		if raw.(map[string]any)["name"] != node["templateName"] {
			kept = append(kept, raw)
		}
	}
	kept = append(kept, map[string]any{"name": node["templateName"], "retryStrategy": map[string]any{"limit": "{{inputs.parameters.retry-max-count}}"}})
	_ = unstructured.SetNestedSlice(f.workflow.Object, kept, "spec", "templates")
	details := f.run["run_details"].(map[string]any)
	tasks := details["task_details"].([]any)
	for _, raw := range tasks {
		if raw.(map[string]any)["display_name"] == "prepare" {
			raw.(map[string]any)["display_name"] = "original-omitted-prepare"
		}
	}
	details["task_details"] = append(tasks, map[string]any{"run_id": f.owner.RunID, "task_id": "79e58ba8-28a4-4b6a-a267-4eac6a8148ff", "display_name": "prepare", "state": "FAILED", "child_tasks": []any{map[string]any{"pod_name": retryID}}}, map[string]any{"run_id": f.owner.RunID, "task_id": "92c0b24d-39b1-4a79-83a1-9643a92b5550", "display_name": "executor", "state": "FAILED", "child_tasks": []any{map[string]any{"pod_name": executorID}}})
	return node
}
