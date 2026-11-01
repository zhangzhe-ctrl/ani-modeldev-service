// Package originalabe assembles the existing exhausted-attempt owner path.
// It adds no runtime compatibility, creation permit, log exception or DELETE.
package originalabe

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/biz"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/data/execution"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/data/lifecycle"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/data/runtimeproof"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/data/stepidentity"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/data/submission"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/data/trainer"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

const TenantID = "64c5c35a-f554-432c-b4bc-7b4f36f08420"
const ExecutionID = "abe39ad8-c017-416b-8041-6f097e8fd50a"
const OriginalComputeSHA = "3ac063c88726ae2ddf0da7bab2f22363f06554b1343e6a187c6403f84c3cec62"

var ErrGuard = errors.New("ORIGINAL_ABE_GUARD_REJECTED")

type Target struct {
	SpecSHA256      string   `json:"spec_sha256,omitempty"`
	AccessModes     []string `json:"access_modes,omitempty"`
	APIVersion      string   `json:"api_version"`
	Kind            string   `json:"kind"`
	Namespace       string   `json:"namespace"`
	Name            string   `json:"name"`
	UID             string   `json:"uid"`
	OwnerUID        string   `json:"owner_uid,omitempty"`
	ResourceVersion string   `json:"resource_version,omitempty"`
	Present         bool     `json:"present"`
	// The operator must use both preconditions; a cascade is not an absence receipt.
	DeleteOptions *metav1.DeleteOptions `json:"operator_delete_options,omitempty"`
}
type Plan struct {
	Schema             string    `json:"schema"`
	ObservedAt         time.Time `json:"observed_at"`
	TenantID           string    `json:"tenant_id"`
	ExecutionID        string    `json:"execution_id"`
	SourceSHA256       string    `json:"source_sha256"`
	OwnerRevision      uint64    `json:"owner_revision"`
	CloseGeneration    uint64    `json:"close_generation"`
	CloseOperationID   string    `json:"close_operation_id"`
	ComputePlanSHA256  string    `json:"compute_plan_sha256"`
	Attempts           uint32    `json:"attempts"`
	LogsCount          int       `json:"original_captured_log_count"`
	LogsSHA256         string    `json:"original_captured_logs_sha256"`
	Workspace          Target    `json:"retained_workspace"`
	Targets            []Target  `json:"targets_parent_first"`
	NoCurrentWriters   bool      `json:"no_current_writers"`
	KubernetesMutation bool      `json:"kubernetes_mutation"`
	DatabaseMutation   bool      `json:"database_mutation"`
}

func SHA(v []byte) string { h := sha256.Sum256(v); return hex.EncodeToString(h[:]) }
func digest(v any) string {
	b, e := json.Marshal(v)
	if e != nil {
		return ""
	}
	return SHA(b)
}
func SourceSHA(r biz.OperationRecord) string {
	// ExecutionStates are a query projection and have no authority. Include the
	// original immutable admission plus every durable runtime/dispatch/owner fact.
	return digest(struct {
		Admission biz.Admission
		Runtime   biz.ExecutionRuntime
		Dispatch  *biz.PipelineDispatch
		Authority *biz.RunAuthorityCandidate
	}{r.Execution.Admission, r.Runtime, r.Dispatch, r.Authority})
}
func ValidateFixed(r biz.OperationRecord) error {
	s := r.Runtime
	if r.Execution.TenantID != TenantID || r.Execution.ExecutionID != ExecutionID || r.Execution.OperationID != "06ecd780-450f-4c29-a5ef-cc772c007b4b" || r.Execution.SpecHash != "face714e1e21d1093bf1eaba967a3a0ceb6875f70080be23703ba5668ff21cfa" || s.OwnerRevision != 614 || s.CloseGeneration != 2 || s.CloseOperationID != "faada02a-a3a9-426a-b166-2d336eeab388" || s.ComputeCleanup == nil || s.ComputeCleanup.PlanHash != OriginalComputeSHA || s.Workspace == nil || s.Workspace.PVCUID != "009c3b8a-ec2a-4937-be66-90f87eb6a3ff" {
		return ErrGuard
	}
	if err := Qualify(r); err != nil {
		return err
	}
	if len(s.ComputeCleanup.Logs) != 1 || len(s.ComputeCleanup.Targets) != 7 {
		return ErrGuard
	}
	return nil
}

// Qualify is also exercised by the real PostgreSQL integration fixture. The
// only CLI entry always calls ValidateFixed before this shared original path.
func Qualify(r biz.OperationRecord) error {
	s := r.Runtime
	c := s.ComputeCleanup
	if r.Authority == nil || r.Dispatch == nil || s.ClosedAt == nil || s.CloseReason != "USER_STOP" || s.CloseReviewReason != "" || s.CloseGeneration == 0 || s.LegacyRetirement != nil || s.StepExitCleanup == nil || s.StepExitCleanup.CompletedAt == nil || s.Workspace == nil || s.Training == nil || s.TrainingHandle == nil || s.Observation == nil || !s.Observation.WritersAbsent || c == nil || c.Phase != "NEEDS_REVIEW" || c.Attempts != 3 || c.OperatorRetry != nil || !c.LogsReady || len(c.ConfirmedAbsent) != 0 || !biz.ComputeCleanupLogsComplete(r.Execution, s, c.Logs) {
		return ErrGuard
	}
	targets, hash, e := biz.ComputeCleanupTargets(r.Execution, *r.Authority, s)
	if e != nil || hash != c.PlanHash || !reflect.DeepEqual(targets, c.Targets) || !reflect.DeepEqual(targets, c.Requested) {
		return ErrGuard
	}
	if r.Execution.OwnerRevision != s.OwnerRevision || r.Dispatch.OwnerRevision != s.OwnerRevision {
		return ErrGuard
	}
	return nil
}
func guard(stage string) error { return fmt.Errorf("%w:%s", ErrGuard, stage) }

func Resource(t biz.RuntimeResource) (schema.GroupVersionResource, error) {
	fixed := map[string]schema.GroupVersionResource{
		"trainer.kubeflow.org/v1alpha1/TrainJob":        {Group: "trainer.kubeflow.org", Version: "v1alpha1", Resource: "trainjobs"},
		"jobset.x-k8s.io/v1alpha2/JobSet":               {Group: "jobset.x-k8s.io", Version: "v1alpha2", Resource: "jobsets"},
		"batch/v1/Job":                                  {Group: "batch", Version: "v1", Resource: "jobs"},
		"v1/Pod":                                        {Version: "v1", Resource: "pods"},
		"scheduling.volcano.sh/v1beta1/PodGroup":        {Group: "scheduling.volcano.sh", Version: "v1beta1", Resource: "podgroups"},
		"argoproj.io/v1alpha1/Workflow":                 {Group: "argoproj.io", Version: "v1alpha1", Resource: "workflows"},
		"trainer.kubeflow.org/v1alpha1/TrainingRuntime": {Group: "trainer.kubeflow.org", Version: "v1alpha1", Resource: "trainingruntimes"},
	}
	g, ok := fixed[t.APIVersion+"/"+t.Kind]
	if !ok {
		return g, ErrGuard
	}
	return g, nil
}
func sameObject(o *unstructured.Unstructured, t biz.RuntimeResource) bool {
	return o != nil && o.GetAPIVersion() == t.APIVersion && o.GetKind() == t.Kind && o.GetNamespace() == t.Namespace && o.GetName() == t.Name && string(o.GetUID()) == t.UID && o.GetResourceVersion() != ""
}
func ownerUID(o *unstructured.Unstructured) (string, error) {
	uid := ""
	for _, v := range o.GetOwnerReferences() {
		if v.Controller != nil && *v.Controller {
			if uid != "" {
				return "", ErrGuard
			}
			uid = string(v.UID)
		}
	}
	return uid, nil
}
func nestedClaim(v any, name string) bool {
	switch x := v.(type) {
	case map[string]any:
		for k, v := range x {
			if k == "claimName" && v == name {
				return true
			}
			if nestedClaim(v, name) {
				return true
			}
		}
	case []any:
		for _, v := range x {
			if nestedClaim(v, name) {
				return true
			}
		}
	}
	return false
}
func refsPVC(o *unstructured.Unstructured, name string) (bool, error) {
	volumes, found, e := unstructured.NestedSlice(o.Object, "spec", "volumes")
	if e != nil {
		return false, ErrGuard
	}
	if !found {
		return false, nil
	}
	for _, v := range volumes {
		m, ok := v.(map[string]any)
		if !ok {
			return false, ErrGuard
		}
		if p, ok := m["persistentVolumeClaim"]; ok {
			pvc, ok := p.(map[string]any)
			if !ok {
				return false, ErrGuard
			}
			claim, ok := pvc["claimName"].(string)
			if !ok {
				return false, ErrGuard
			}
			if claim == name {
				return true, nil
			}
		}
	}
	return false, nil
}

// Inspect performs only GET/LIST. Present original workers require the actual
// Trainer observer's complete graph and persisted log matching, not Pod phase.
func Inspect(ctx context.Context, kube dynamic.Interface, r biz.OperationRecord) (Plan, error) {
	if e := Qualify(r); e != nil {
		return Plan{}, e
	}
	p := Plan{Schema: "ani.modeldev.original-abe-compute-plan.v1", ObservedAt: time.Now().UTC(), TenantID: r.Execution.TenantID, ExecutionID: r.Execution.ExecutionID, SourceSHA256: SourceSHA(r), OwnerRevision: r.Runtime.OwnerRevision, CloseGeneration: r.Runtime.CloseGeneration, CloseOperationID: r.Runtime.CloseOperationID, ComputePlanSHA256: r.Runtime.ComputeCleanup.PlanHash, Attempts: 3, LogsCount: len(r.Runtime.ComputeCleanup.Logs), LogsSHA256: digest(r.Runtime.ComputeCleanup.Logs)}
	owner := *r.Authority
	s := r.Runtime
	ns, e := kube.Resource(schema.GroupVersionResource{Version: "v1", Resource: "namespaces"}).Get(ctx, owner.NamespaceName, metav1.GetOptions{})
	if e != nil || ns.GetUID() == "" || string(ns.GetUID()) != owner.NamespaceUID || ns.GetDeletionTimestamp() != nil {
		return p, guard("CURRENT_NAMESPACE_IDENTITY")
	}
	pvc, e := kube.Resource(schema.GroupVersionResource{Version: "v1", Resource: "persistentvolumeclaims"}).Namespace(owner.NamespaceName).Get(ctx, s.Workspace.PVCName, metav1.GetOptions{})
	if e != nil || string(pvc.GetUID()) != s.Workspace.PVCUID || pvc.GetDeletionTimestamp() != nil || len(pvc.GetOwnerReferences()) != 0 {
		return p, guard("CURRENT_PVC_IDENTITY_AND_GC_CLOSURE")
	}
	phase, _, phaseErr := unstructured.NestedString(pvc.Object, "status", "phase")
	class, _, classErr := unstructured.NestedString(pvc.Object, "spec", "storageClassName")
	storage, _, storageErr := unstructured.NestedString(pvc.Object, "spec", "resources", "requests", "storage")
	modes, _, modesErr := unstructured.NestedStringSlice(pvc.Object, "spec", "accessModes")
	quantity, quantityErr := resource.ParseQuantity(storage)
	if phaseErr != nil || classErr != nil || storageErr != nil || modesErr != nil || phase != "Bound" || class != r.Execution.Snapshot.Workspace.StorageClass || quantityErr != nil || quantity.Cmp(*resource.NewQuantity(r.Execution.Snapshot.Workspace.CapacityBytes, resource.BinarySI)) != 0 || (len(modes) != 1 || (modes[0] != "ReadWriteOnce" && modes[0] != "ReadWriteMany")) {
		return p, guard("CURRENT_PVC_FROZEN_SCOPE")
	}
	pvcSpec, _, pvcSpecErr := unstructured.NestedMap(pvc.Object, "spec")
	if pvcSpecErr != nil {
		return p, guard("PVC_SPEC_INVALID")
	}
	p.Workspace = Target{SpecSHA256: digest(pvcSpec), AccessModes: modes, APIVersion: "v1", Kind: "PersistentVolumeClaim", Namespace: owner.NamespaceName, Name: pvc.GetName(), UID: string(pvc.GetUID()), ResourceVersion: pvc.GetResourceVersion(), Present: true}
	byUID := map[string]biz.RuntimeResource{}
	presentTraining := false
	missingTraining := false
	for _, t := range s.ComputeCleanup.Targets {
		byUID[t.UID] = t
	}
	for _, t := range s.ComputeCleanup.Targets {
		g, e := Resource(t)
		if e != nil {
			return p, e
		}
		o, e := kube.Resource(g).Namespace(t.Namespace).Get(ctx, t.Name, metav1.GetOptions{})
		item := Target{APIVersion: t.APIVersion, Kind: t.Kind, Namespace: t.Namespace, Name: t.Name, UID: t.UID, OwnerUID: t.OwnerUID}
		if apierrors.IsNotFound(e) {
			p.Targets = append(p.Targets, item)
			if t.Kind != "Workflow" && t.Kind != "TrainingRuntime" {
				missingTraining = true
			}
			continue
		}
		if e != nil || !sameObject(o, t) || o.GetDeletionTimestamp() != nil || len(o.GetFinalizers()) != 0 {
			return p, guard("CURRENT_TARGET_IDENTITY:" + t.Kind + "")
		}
		actualOwner, e := ownerUID(o)
		if e != nil || actualOwner != t.OwnerUID {
			return p, guard("CURRENT_TARGET_ORIGINAL_PARENT:" + t.Kind + "")
		}
		if t.Kind == "TrainingRuntime" && (len(o.GetOwnerReferences()) != 0 || o.GetAnnotations()["modeldev.ani.io/execution-id"] != r.Execution.ExecutionID || o.GetAnnotations()["modeldev.ani.io/tenant-id"] != r.Execution.TenantID || o.GetAnnotations()["modeldev.ani.io/execution-spec-sha256"] != r.Execution.SpecHash) {
			return p, guard("CURRENT_RUNTIME_ORIGINAL_ANNOTATIONS")
		}
		if t.Kind == "Workflow" {
			return p, guard("ORIGINAL_WORKFLOW_REAPPEARED")
		}
		if t.Kind != "Workflow" && t.Kind != "TrainingRuntime" {
			presentTraining = true
		}
		uid, rv := o.GetUID(), o.GetResourceVersion()
		prop := metav1.DeletePropagationBackground
		item.Present = true
		item.ResourceVersion = rv
		item.DeleteOptions = &metav1.DeleteOptions{Preconditions: &metav1.Preconditions{UID: &uid, ResourceVersion: &rv}, PropagationPolicy: &prop}
		p.Targets = append(p.Targets, item)
	}
	if presentTraining {
		// Partial controller GC is not sufficient to create a fresh deletion plan.
		// The all-absent observe phase has its own original repository path below.
		if missingTraining {
			return p, guard("PARTIAL_ORIGINAL_TRAINING_GRAPH")
		}
		fresh, e := trainer.New(kube).ObserveTraining(ctx, *s.Training, *s.TrainingHandle, s.Observation.Resources)
		if e != nil || !fresh.WritersAbsent || fresh.SpecificationMismatch || !reflect.DeepEqual(fresh.Resources, s.Observation.Resources) {
			if e != nil {
				return p, guard("CURRENT_TRAINER_OBSERVATION_REJECTED")
			}
			return p, guard("CURRENT_TRAINING_ORIGINAL_GRAPH_MISMATCH")
		}
	}
	for _, g := range []schema.GroupVersionResource{{Version: "v1", Resource: "pods"}, {Group: "batch", Version: "v1", Resource: "jobs"}, {Group: "jobset.x-k8s.io", Version: "v1alpha2", Resource: "jobsets"}, {Group: "scheduling.volcano.sh", Version: "v1beta1", Resource: "podgroups"}, {Group: "trainer.kubeflow.org", Version: "v1alpha1", Resource: "trainjobs"}, {Group: "argoproj.io", Version: "v1alpha1", Resource: "workflows"}} {
		list, e := kube.Resource(g).Namespace(owner.NamespaceName).List(ctx, metav1.ListOptions{})
		if e != nil || list.GetContinue() != "" || list.GetResourceVersion() == "" {
			return p, guard("CURRENT_COMPLETE_LIST:" + g.Resource + "")
		}
		for i := range list.Items {
			o := &list.Items[i]
			_, known := byUID[string(o.GetUID())]
			if !known {
				for _, ref := range o.GetOwnerReferences() {
					if _, ok := byUID[string(ref.UID)]; ok {
						return p, guard("CURRENT_UNRECORDED_DESCENDANT:" + g.Resource + "")
					}
				}
				if nestedClaim(o.Object["spec"], s.Workspace.PVCName) {
					return p, guard("CURRENT_UNRECORDED_PVC_TEMPLATE:" + g.Resource + "")
				}
			}
			if g.Resource == "pods" {
				uses, e := refsPVC(o, s.Workspace.PVCName)
				if e != nil {
					return p, e
				}
				if uses && !known {
					return p, guard("CURRENT_UNRECORDED_PVC_POD")
				}
			}
		}
	}
	p.NoCurrentWriters = true
	return p, nil
}

// denyRuns is an unavailable outbound boundary, not a replacement for KFP
// evidence. CLOSED+completed StepExit must never call it.
type denyRuns struct{}

func (denyRuns) StopManagedRun(context.Context, biz.PipelineDispatchPlan, biz.RunAuthorityCandidate) error {
	return ErrGuard
}
func AssembleCloser(pool *pgxpool.Pool, kube dynamic.Interface) (*biz.ExecutionCloser, error) {
	dispatch := submission.New(pool)
	proof := runtimeproof.New(kube, nil, nil, dispatch)
	identity, e := stepidentity.New(kube, "original-abe-maintenance-unused")
	if e != nil {
		return nil, e
	}
	steps, e := biz.NewManagedSteps(dispatch, execution.New(pool), identity, proof)
	if e != nil {
		return nil, e
	}
	run, e := biz.NewManagedRuntime(steps, lifecycle.New(pool), trainer.New(kube), proof, proof)
	if e != nil {
		return nil, e
	}
	return biz.NewExecutionCloser(run, denyRuns{}, proof, proof, proof)
}

// Observe uses the original Closer and original RecordComputeCleanupResult CAS.
// It cannot grant retry, call ReconcileWithActor or mutate a Kubernetes object.
func Observe(ctx context.Context, pool *pgxpool.Pool, kube dynamic.Interface, reviewed Plan) (biz.ExecutionRuntime, error) {
	r, e := execution.New(pool).GetOperationRecord(ctx, reviewed.TenantID, reviewed.ExecutionID)
	if e != nil {
		return biz.ExecutionRuntime{}, e
	}
	if e = Qualify(r); e != nil || reviewed.SourceSHA256 != SourceSHA(r) || reviewed.ComputePlanSHA256 != r.Runtime.ComputeCleanup.PlanHash || reviewed.OwnerRevision != r.Runtime.OwnerRevision || reviewed.CloseGeneration != r.Runtime.CloseGeneration || reviewed.CloseOperationID != r.Runtime.CloseOperationID || reviewed.Attempts != 3 || !reviewed.NoCurrentWriters {
		return r.Runtime, ErrGuard
	}
	fresh, e := Inspect(ctx, kube, r)
	if e != nil {
		return r.Runtime, e
	}
	if fresh.Workspace.UID != reviewed.Workspace.UID || fresh.Workspace.SpecSHA256 == "" || fresh.Workspace.SpecSHA256 != reviewed.Workspace.SpecSHA256 {
		return r.Runtime, guard("REVIEWED_ORIGINAL_PVC_SPEC_DRIFT")
	}
	for _, v := range fresh.Targets {
		if v.Present {
			return r.Runtime, guard("ORIGINAL_TARGET_STILL_PRESENT")
		}
	}
	closer, e := AssembleCloser(pool, kube)
	if e != nil {
		return r.Runtime, e
	}
	result, e := closer.Reconcile(ctx, reviewed.TenantID, reviewed.ExecutionID)
	if e != nil {
		return result.Runtime, e
	}
	if result.Runtime.ComputeCleanup == nil || result.Runtime.ComputeCleanup.Phase != "APPLIED" || result.Runtime.ComputeCleanup.Attempts != 3 || result.Runtime.ComputeCleanup.OperatorRetry != nil || !reflect.DeepEqual(r.Runtime.ComputeCleanup.Logs, result.Runtime.ComputeCleanup.Logs) || !biz.ComputeCleanupApplied(biz.OperationRecord{QueryRecord: biz.QueryRecord{Execution: r.Execution, Runtime: result.Runtime}, Authority: r.Authority}) {
		return result.Runtime, ErrGuard
	}
	return result.Runtime, nil
}
