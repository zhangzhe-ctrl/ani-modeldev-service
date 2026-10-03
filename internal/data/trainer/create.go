package trainer

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"path"
	"reflect"
	"strings"

	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/biz"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/validation"
)

var trainJobs = schema.GroupVersionResource{Group: "trainer.kubeflow.org", Version: "v1alpha1", Resource: "trainjobs"}

// CreateTraining is called only by the holder of the durable first creation
// permit. Reading an existing intent must use FindTraining, never another POST.
func (a *Adapter) CreateTraining(ctx context.Context, plan biz.TrainingPlan) (biz.TrainingHandle, error) {
	wanted, err := a.trainingRequest(ctx, plan)
	if err != nil { return biz.TrainingHandle{}, err }
	resource := a.client.Resource(trainJobs).Namespace(plan.Workspace.NamespaceName)
	existing, err := resource.Get(ctx, plan.Name, metav1.GetOptions{})
	if err == nil { return verifyTrainingObject(plan, wanted, existing) }
	if !apierrors.IsNotFound(err) { return biz.TrainingHandle{}, biz.ErrTrainingUnavailable }
	created, err := resource.Create(ctx, wanted, metav1.CreateOptions{})
	if err != nil { return biz.TrainingHandle{}, biz.ErrTrainingUncertain }
	handle, err := verifyTrainingObject(plan, wanted, created)
	if err != nil { return biz.TrainingHandle{}, biz.ErrTrainingUncertain }
	return handle, nil
}

func (a *Adapter) FindTraining(ctx context.Context, plan biz.TrainingPlan) (biz.TrainingHandle, error) {
	_, handle, err := a.currentTraining(ctx, plan)
	return handle, err
}

func (a *Adapter) currentTraining(ctx context.Context, plan biz.TrainingPlan) (*unstructured.Unstructured, biz.TrainingHandle, error) {
	wanted, err := a.trainingRequest(ctx, plan)
	if err != nil { return nil, biz.TrainingHandle{}, err }
	actual, err := a.client.Resource(trainJobs).Namespace(plan.Workspace.NamespaceName).Get(ctx, plan.Name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) { return nil, biz.TrainingHandle{}, biz.ErrTrainingNotFound }
	if err != nil { return nil, biz.TrainingHandle{}, biz.ErrTrainingUnavailable }
	handle, err := verifyTrainingObject(plan, wanted, actual)
	return actual, handle, err
}

func (a *Adapter) StopTraining(ctx context.Context, plan biz.TrainingPlan, handle biz.TrainingHandle) error {
	actual, current, err := a.currentTraining(ctx, plan)
	if err != nil { return err }
	if current != handle || actual.GetResourceVersion() == "" { return biz.ErrRuntimeConflict }
	suspended, _, err := unstructured.NestedBool(actual.Object, "spec", "suspend")
	if err != nil { return biz.ErrRuntimeConflict }
	if suspended { return nil }
	patch, err := json.Marshal([]map[string]any{
		{"op":"test", "path":"/metadata/uid", "value":handle.TrainJobUID},
		{"op":"test", "path":"/metadata/resourceVersion", "value":actual.GetResourceVersion()},
		{"op":"add", "path":"/spec/suspend", "value":true},
	})
	if err != nil { return biz.ErrTrainingUnavailable }
	result, err := a.client.Resource(trainJobs).Namespace(plan.Workspace.NamespaceName).Patch(ctx, plan.Name, types.JSONPatchType, patch, metav1.PatchOptions{})
	if err != nil { return biz.ErrTrainingUnavailable }
	if result == nil || string(result.GetUID()) != handle.TrainJobUID { return biz.ErrRuntimeConflict }
	suspended, _, err = unstructured.NestedBool(result.Object, "spec", "suspend")
	if err != nil || !suspended { return biz.ErrTrainingUnavailable }
	// Acceptance closes no business state. ObserveTraining must still verify
	// controllers and every currently known or historical writer.
	return nil
}

func (a *Adapter) trainingRequest(ctx context.Context, plan biz.TrainingPlan) (*unstructured.Unstructured, error) {
	if a == nil || a.client == nil || ctx == nil || ctx.Err() != nil || !validTrainingPlan(plan) { return nil, biz.ErrRuntimeConflict }
	workspace, snapshot := plan.Workspace, plan.Snapshot
	namespace, err := a.client.Resource(schema.GroupVersionResource{Version:"v1", Resource:"namespaces"}).Get(ctx, workspace.NamespaceName, metav1.GetOptions{})
	if err != nil { return nil, biz.ErrTrainingUnavailable }
	if !liveIdentity(namespace,"v1","Namespace","",workspace.NamespaceName,workspace.NamespaceUID) { return nil, biz.ErrRuntimeConflict }
	pvc, err := a.client.Resource(schema.GroupVersionResource{Version:"v1", Resource:"persistentvolumeclaims"}).Namespace(workspace.NamespaceName).Get(ctx, workspace.PVCName, metav1.GetOptions{})
	if err != nil { return nil, biz.ErrTrainingUnavailable }
	phase, _, _ := unstructured.NestedString(pvc.Object,"status","phase")
	storageClass, _, _ := unstructured.NestedString(pvc.Object,"spec","storageClassName")
	if !liveIdentity(pvc,"v1","PersistentVolumeClaim",workspace.NamespaceName,workspace.PVCName,workspace.PVCUID) || phase != "Bound" || storageClass != snapshot.Workspace.StorageClass { return nil, biz.ErrRuntimeConflict }
	runtimeResource := schema.GroupVersionResource{Group:"trainer.kubeflow.org",Version:"v1alpha1",Resource:"clustertrainingruntimes"}
	runtimeNamespace := ""
	if snapshot.Release.Runtime.Kind == "TrainingRuntime" { runtimeResource.Resource = "trainingruntimes"; runtimeNamespace = workspace.NamespaceName }
	runtime, err := a.client.Resource(runtimeResource).Namespace(runtimeNamespace).Get(ctx,snapshot.Release.Runtime.Name,metav1.GetOptions{})
	if err != nil { return nil, biz.ErrTrainingUnavailable }
	if runtime == nil || runtime.GetAPIVersion() != "trainer.kubeflow.org/v1alpha1" || runtime.GetKind() != snapshot.Release.Runtime.Kind || runtime.GetName() != snapshot.Release.Runtime.Name || runtime.GetNamespace() != runtimeNamespace || runtime.GetDeletionTimestamp() != nil || !safeRuntime(runtime, snapshot.Release.Runtime.ContentSHA256) { return nil, biz.ErrRuntimeConflict }

	annotations := map[string]any{"modeldev.ani.io/tenant-id":plan.TenantID,"modeldev.ani.io/execution-id":plan.ExecutionID,"modeldev.ani.io/execution-spec-sha256":plan.SpecHash,"modeldev.ani.io/training-request-sha256":plan.RequestSHA256,"modeldev.ani.io/workspace-uid":workspace.PVCUID,"modeldev.ani.io/runtime-spec-sha256":snapshot.Release.Runtime.ContentSHA256}
	return &unstructured.Unstructured{Object:map[string]any{
		"apiVersion":"trainer.kubeflow.org/v1alpha1","kind":"TrainJob",
		"metadata":map[string]any{"name":plan.Name,"namespace":workspace.NamespaceName,"annotations":annotations},
		"spec":map[string]any{
			"runtimeRef":map[string]any{"name":snapshot.Release.Runtime.Name,"kind":snapshot.Release.Runtime.Kind,"apiGroup":"trainer.kubeflow.org"},
			"managedBy":"trainer.kubeflow.org/trainjob-controller", "suspend":false,
			"trainer":map[string]any{"image":snapshot.Program.ImageDigest,"command":stringValues(snapshot.Program.Command),"args":stringValues(snapshot.Program.ResolvedArgs),"numNodes":int64(1),"numProcPerNode":int64(1),"resourcesPerNode":map[string]any{
				"requests":map[string]any{"cpu":resource.NewMilliQuantity(snapshot.Resources.RequestMillicpu,resource.DecimalSI).String(),"memory":resource.NewQuantity(snapshot.Resources.RequestMemoryBytes,resource.BinarySI).String()},
				"limits":map[string]any{"cpu":resource.NewMilliQuantity(snapshot.Resources.LimitMillicpu,resource.DecimalSI).String(),"memory":resource.NewQuantity(snapshot.Resources.LimitMemoryBytes,resource.BinarySI).String()},
			}},
			"podTemplateOverrides":[]any{map[string]any{"targetJobs":[]any{map[string]any{"name":"trainer"}},"spec":map[string]any{
				"serviceAccountName":snapshot.Environment.Identities.TrainerServiceAccount,
				"volumes":[]any{map[string]any{"name":"workspace","persistentVolumeClaim":map[string]any{"claimName":workspace.PVCName}}},
				"containers":[]any{map[string]any{"name":"node","volumeMounts":[]any{
					map[string]any{"name":"workspace","mountPath":"/inputs","subPath":workspace.InputSubpath,"readOnly":true},
					map[string]any{"name":"workspace","mountPath":"/outputs","subPath":workspace.TrainingSubpath,"readOnly":false},
				}}},
			}}},
		},
	}}, nil
}

func verifyTrainingObject(plan biz.TrainingPlan, wanted, actual *unstructured.Unstructured) (biz.TrainingHandle,error) {
	if actual == nil || actual.GetAPIVersion() != wanted.GetAPIVersion() || actual.GetKind() != "TrainJob" || actual.GetNamespace() != plan.Workspace.NamespaceName || actual.GetName() != plan.Name || actual.GetUID() == "" || actual.GetDeletionTimestamp() != nil { return biz.TrainingHandle{},biz.ErrRuntimeConflict }
	for key,value := range wanted.GetAnnotations() { if actual.GetAnnotations()[key] != value { return biz.TrainingHandle{},biz.ErrRuntimeConflict } }
	expectedSpec, _, _ := unstructured.NestedMap(wanted.Object,"spec")
	actualSpec, found, err := unstructured.NestedMap(actual.Object,"spec")
	if err != nil || !found { return biz.TrainingHandle{},biz.ErrRuntimeConflict }
	// The one mutable field is suspension. All computation, template, identity
	// and workspace fields must remain the complete original request.
	if suspended, ok := actualSpec["suspend"].(bool); ok { expectedSpec["suspend"] = suspended }
	expectedBytes, _ := json.Marshal(expectedSpec)
	actualBytes, _ := json.Marshal(actualSpec)
	if string(expectedBytes) != string(actualBytes) { return biz.TrainingHandle{},biz.ErrRuntimeConflict }
	return biz.TrainingHandle{NamespaceUID:plan.Workspace.NamespaceUID,TrainJobUID:string(actual.GetUID()),PVCUID:plan.Workspace.PVCUID},nil
}

func validTrainingPlan(plan biz.TrainingPlan) bool {
	w, s := plan.Workspace, plan.Snapshot
	if !validBinding(biz.TrainJobBinding{TenantID:plan.TenantID,ExecutionID:plan.ExecutionID,SpecSHA256:plan.SpecHash,Namespace:w.NamespaceName,NamespaceUID:w.NamespaceUID,Name:plan.Name,UID:w.PVCUID}) || plan.Name != "md-"+plan.ExecutionID || len(plan.RequestSHA256) != 64 { return false }
	if _, err := hex.DecodeString(plan.RequestSHA256); err != nil { return false }
	if w.Mode != "EXECUTION_PVC" || w.Mode != s.Workspace.Mode || w.NamespaceName != s.Environment.NamespaceName || w.NamespaceUID != s.Environment.NamespaceUID || len(validation.IsDNS1123Subdomain(w.PVCName)) != 0 || len(validation.IsDNS1123Subdomain(s.Environment.Identities.TrainerServiceAccount)) != 0 { return false }
	if w.InputSubpath != s.Workspace.InputSubpath || w.TrainingSubpath != s.Workspace.TrainingSubpath || w.ReportsSubpath != s.Workspace.ReportsSubpath || w.PublicationSubpath != s.Workspace.PublicationSubpath || w.InputSubpath == w.TrainingSubpath { return false }
	for _, subpath := range []string{w.InputSubpath,w.TrainingSubpath} { if subpath == "" || path.IsAbs(subpath) || path.Clean(subpath) != subpath || subpath == ".." || strings.HasPrefix(subpath,"../") { return false } }
	if s.Release.Runtime.APIGroup != "trainer.kubeflow.org" || (s.Release.Runtime.Kind != "TrainingRuntime" && s.Release.Runtime.Kind != "ClusterTrainingRuntime") || len(validation.IsDNS1123Subdomain(s.Release.Runtime.Name)) != 0 || !reflect.DeepEqual(s.Release.Runtime.TargetJobs,[]string{"trainer"}) { return false }
	if len(s.Program.Command) == 0 || !strings.Contains(s.Program.ImageDigest,"@sha256:") || s.Resources.Nodes != 1 || s.Resources.ProcessesPerNode != 1 || s.Resources.RequestMillicpu <= 0 || s.Resources.LimitMillicpu < s.Resources.RequestMillicpu || s.Resources.RequestMemoryBytes <= 0 || s.Resources.LimitMemoryBytes < s.Resources.RequestMemoryBytes { return false }
	return true
}

func safeRuntime(runtime *unstructured.Unstructured, expectedSHA string) bool {
	spec, found, err := unstructured.NestedMap(runtime.Object,"spec")
	if err != nil || !found { return false }
	encoded, err := json.Marshal(spec)
	if err != nil { return false }
	digest := sha256.Sum256(encoded)
	if hex.EncodeToString(digest[:]) != expectedSHA { return false }
	if _, present, _ := unstructured.NestedFieldNoCopy(spec,"mlPolicy","torch","elasticPolicy"); present { return false }
	if _, present, _ := unstructured.NestedFieldNoCopy(spec,"mlPolicy","mpi"); present { return false }
	maxRestarts, present, err := unstructured.NestedInt64(spec,"template","spec","failurePolicy","maxRestarts")
	if err != nil || !present || maxRestarts != 0 { return false }
	jobs, found, err := unstructured.NestedSlice(spec,"template","spec","replicatedJobs")
	if err != nil || !found || len(jobs) != 1 { return false }
	job, ok := jobs[0].(map[string]any)
	if !ok || job["name"] != "trainer" { return false }
	for _, fields := range [][]string{{"replicas"},{"template","spec","parallelism"},{"template","spec","completions"}} { number,present,err := unstructured.NestedInt64(job,fields...); if err != nil || !present || number != 1 { return false } }
	backoff, present, err := unstructured.NestedInt64(job,"template","spec","backoffLimit")
	if err != nil || !present || backoff != 0 { return false }
	label,_,_ := unstructured.NestedString(job,"template","metadata","labels","trainer.kubeflow.org/trainjob-ancestor-step")
	if label != "trainer" { return false }
	pod,found,err := unstructured.NestedMap(job,"template","spec","template","spec")
	if err != nil || !found || pod["restartPolicy"] != "Never" || pod["automountServiceAccountToken"] != false { return false }
	for _, field := range []string{"initContainers","volumes"} { values,_,err := unstructured.NestedSlice(pod,field); if err != nil || len(values) != 0 { return false } }
	for _, field := range []string{"hostNetwork","hostPID","hostIPC"} { value,_,err := unstructured.NestedBool(pod,field); if err != nil || value { return false } }
	nonRoot,_,_ := unstructured.NestedBool(pod,"securityContext","runAsNonRoot")
	if !nonRoot { return false }
	containers,found,err := unstructured.NestedSlice(pod,"containers")
	if err != nil || !found || len(containers) != 1 { return false }
	container,ok := containers[0].(map[string]any)
	if !ok || container["name"] != "node" { return false }
	escalation,present,err := unstructured.NestedBool(container,"securityContext","allowPrivilegeEscalation")
	if err != nil || !present || escalation { return false }
	privileged,_,err := unstructured.NestedBool(container,"securityContext","privileged")
	if err != nil || privileged { return false }
	dropped,_,err := unstructured.NestedStringSlice(container,"securityContext","capabilities","drop")
	if err != nil || !reflect.DeepEqual(dropped,[]string{"ALL"}) { return false }
	for _, field := range []string{"volumeMounts","envFrom"} { values,_,err := unstructured.NestedSlice(container,field); if err != nil || len(values) != 0 { return false } }
	return true
}

func stringValues(values []string) []any { result := make([]any,len(values)); for i,value := range values { result[i] = value }; return result }

func liveIdentity(object *unstructured.Unstructured, apiVersion,kind,namespace,name,uid string) bool {
	return object != nil && object.GetAPIVersion() == apiVersion && object.GetKind() == kind && object.GetNamespace() == namespace && object.GetName() == name && string(object.GetUID()) == uid && object.GetDeletionTimestamp() == nil
}

var _ biz.TrainingRuntime = (*Adapter)(nil)
