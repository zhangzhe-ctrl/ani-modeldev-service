package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kschema "k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
)

// Kubernetes omits item TypeMeta on the wire, but the pinned UnstructuredList
// decoder restores it from the authenticated collection. This guards against
// mistaking the raw API shape for a missing-identity qualification failure.
func TestLegacyRetirementExistingDynamicPodListDecoder(t *testing.T) {
	pod := map[string]any{"metadata": map[string]any{"name": "original", "namespace": "tenant-a", "uid": "original-pod-uid", "resourceVersion": "27", "ownerReferences": []any{map[string]any{"uid": "original-owner"}}}, "spec": map[string]any{"serviceAccountName": "original-sa"}, "status": map[string]any{"phase": "Succeeded"}}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/namespaces/tenant-a/pods" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"apiVersion": "v1", "kind": "PodList", "metadata": map[string]any{"resourceVersion": "42"}, "items": []any{pod}})
	}))
	defer server.Close()
	client, err := dynamic.NewForConfig(&rest.Config{Host: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	list, err := client.Resource(kschema.GroupVersionResource{Version: "v1", Resource: "pods"}).Namespace("tenant-a").List(context.Background(), metav1.ListOptions{})
	if err != nil || list == nil || len(list.Items) != 1 {
		t.Fatalf("actual dynamic decode failed: %v", err)
	}
	got := list.Items[0].DeepCopy()
	if got.GetAPIVersion() != "v1" || got.GetKind() != "Pod" {
		t.Fatalf("existing decoder did not restore Pod TypeMeta: %q/%q", got.GetAPIVersion(), got.GetKind())
	}
	delete(got.Object, "apiVersion")
	delete(got.Object, "kind")
	if !reflect.DeepEqual(pod, got.Object) {
		t.Fatal("identity, owner, spec or status changed during decode")
	}
}
