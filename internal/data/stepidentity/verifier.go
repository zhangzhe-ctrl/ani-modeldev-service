// Package stepidentity verifies a managed callback against the current
// Kubernetes TokenReview and resource APIs. KFP Run membership is verified by
// a separate KFP adapter before the use case binds authority.
package stepidentity

import (
	"context"
	"errors"
	"strings"

	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/biz"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/client-go/dynamic"
)

var (
	ErrInvalidConfig = errors.New("MANAGED_STEP_IDENTITY_INVALID_CONFIGURATION")
	ErrUnverified    = errors.New("MANAGED_STEP_IDENTITY_UNVERIFIED")
)

type Verifier struct {
	client   dynamic.Interface
	audience string
}

func New(client dynamic.Interface, audience string) (*Verifier, error) {
	if client == nil || audience == "" || len(audience) > 2048 || strings.ContainsAny(audience, " \t\r\n") {
		return nil, ErrInvalidConfig
	}
	return &Verifier{client: client, audience: audience}, nil
}

func (verifier *Verifier) Verify(ctx context.Context, token string, plan biz.PipelineDispatchPlan, association biz.ManagedStepAssociation) error {
	if verifier == nil || verifier.client == nil || ctx == nil || ctx.Err() != nil || token == "" || len(token) > 16384 || strings.ContainsAny(token, " \t\r\n") {
		return ErrUnverified
	}
	environment := plan.Environment
	serviceAccount := environment.Identities.KFPStepServiceAccount
	if association.NamespaceName != environment.NamespaceName || association.NamespaceUID != environment.NamespaceUID || association.NamespaceUID == "" || association.PodUID == "" || association.WorkflowUID == "" {
		return ErrUnverified
	}
	if len(validation.IsDNS1123Label(association.NamespaceName)) != 0 || len(validation.IsDNS1123Subdomain(serviceAccount)) != 0 || len(validation.IsDNS1123Subdomain(association.PodName)) != 0 || len(validation.IsDNS1123Subdomain(association.WorkflowName)) != 0 {
		return ErrUnverified
	}

	// The official client uses the control workload's API credential. The
	// caller's short-lived token is only the TokenReview body, never a client
	// credential or a cached authorization result.
	review, err := verifier.client.Resource(schema.GroupVersionResource{Group: "authentication.k8s.io", Version: "v1", Resource: "tokenreviews"}).Create(ctx, &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "authentication.k8s.io/v1",
		"kind":       "TokenReview",
		"spec":       map[string]any{"token": token, "audiences": []any{verifier.audience}},
	}}, metav1.CreateOptions{})
	if err != nil || review == nil || review.GetAPIVersion() != "authentication.k8s.io/v1" || review.GetKind() != "TokenReview" {
		return ErrUnverified
	}
	authenticated, _, err := unstructured.NestedBool(review.Object, "status", "authenticated")
	if err != nil || !authenticated || !exactStringSlice(review, []string{verifier.audience}, "status", "audiences") || !exactStringSlice(review, []string{association.PodName}, "status", "user", "extra", "authentication.kubernetes.io/pod-name") || !exactStringSlice(review, []string{association.PodUID}, "status", "user", "extra", "authentication.kubernetes.io/pod-uid") {
		return ErrUnverified
	}
	username, _, err := unstructured.NestedString(review.Object, "status", "user", "username")
	if err != nil || username != "system:serviceaccount:"+environment.NamespaceName+":"+serviceAccount {
		return ErrUnverified
	}
	serviceAccountUID, _, err := unstructured.NestedString(review.Object, "status", "user", "uid")
	if err != nil || serviceAccountUID == "" {
		return ErrUnverified
	}

	namespace, err := verifier.client.Resource(schema.GroupVersionResource{Version: "v1", Resource: "namespaces"}).Get(ctx, association.NamespaceName, metav1.GetOptions{})
	if err != nil || !currentObject(namespace, "v1", "Namespace", "", association.NamespaceName, association.NamespaceUID) {
		return ErrUnverified
	}
	pod, err := verifier.client.Resource(schema.GroupVersionResource{Version: "v1", Resource: "pods"}).Namespace(association.NamespaceName).Get(ctx, association.PodName, metav1.GetOptions{})
	if err != nil || !currentObject(pod, "v1", "Pod", association.NamespaceName, association.PodName, association.PodUID) {
		return ErrUnverified
	}
	podServiceAccount, _, err := unstructured.NestedString(pod.Object, "spec", "serviceAccountName")
	if err != nil || podServiceAccount != serviceAccount || !ownedByWorkflow(pod, association) {
		return ErrUnverified
	}
	account, err := verifier.client.Resource(schema.GroupVersionResource{Version: "v1", Resource: "serviceaccounts"}).Namespace(association.NamespaceName).Get(ctx, serviceAccount, metav1.GetOptions{})
	if err != nil || !currentObject(account, "v1", "ServiceAccount", association.NamespaceName, serviceAccount, serviceAccountUID) {
		return ErrUnverified
	}
	workflow, err := verifier.client.Resource(schema.GroupVersionResource{Group: "argoproj.io", Version: "v1alpha1", Resource: "workflows"}).Namespace(association.NamespaceName).Get(ctx, association.WorkflowName, metav1.GetOptions{})
	if err != nil || !currentObject(workflow, "argoproj.io/v1alpha1", "Workflow", association.NamespaceName, association.WorkflowName, association.WorkflowUID) || ctx.Err() != nil {
		return ErrUnverified
	}
	// These facts prove the current authenticated Pod/Workflow association only.
	// The KFP API must independently prove the Run's frozen execution/spec and
	// actual task Pod membership; labels and self-reported Run IDs grant nothing.
	return nil
}

func exactStringSlice(object *unstructured.Unstructured, expected []string, fields ...string) bool {
	values, found, err := unstructured.NestedStringSlice(object.Object, fields...)
	if err != nil || !found || len(values) != len(expected) {
		return false
	}
	for i := range expected {
		if values[i] != expected[i] {
			return false
		}
	}
	return true
}

func currentObject(object *unstructured.Unstructured, apiVersion, kind, namespace, name, uid string) bool {
	return object != nil && object.GetAPIVersion() == apiVersion && object.GetKind() == kind && object.GetNamespace() == namespace && object.GetName() == name && string(object.GetUID()) == uid && object.GetDeletionTimestamp() == nil
}

func ownedByWorkflow(pod *unstructured.Unstructured, association biz.ManagedStepAssociation) bool {
	controllers := 0
	for _, owner := range pod.GetOwnerReferences() {
		if owner.Controller == nil || !*owner.Controller {
			continue
		}
		controllers++
		if owner.APIVersion != "argoproj.io/v1alpha1" || owner.Kind != "Workflow" || owner.Name != association.WorkflowName || string(owner.UID) != association.WorkflowUID {
			return false
		}
	}
	return controllers == 1
}
