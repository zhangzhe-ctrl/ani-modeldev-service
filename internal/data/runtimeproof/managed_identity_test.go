package runtimeproof_test

import (
	"context"
	"testing"

	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/biz"
)

func TestManagedIdentityReturnsKFPTaskIDOnlyForExactCurrentRolePod(t *testing.T) {
	f := newOwnerCloseFixture(t)
	a := f.authority()
	association := biz.ManagedStepAssociation{RunID: a.RunID, NamespaceName: a.NamespaceName, NamespaceUID: a.NamespaceUID, WorkflowName: a.WorkflowName, WorkflowUID: a.WorkflowUID, PodName: "general-cpu-rnggt-retry-system-container-impl-1467617326", PodUID: "cbee9dc7-116f-432d-b639-334fcadf0e7e"}
	id, err := f.verifier().ResolveManagedTaskID(context.Background(), f.dispatch.Plan, association, "workspace-name")
	if err != nil || id != "35e10c3b-d56f-4260-bdb0-1069ea06f76d" {
		t.Fatalf("real KFP identity unresolved: %q %v", id, err)
	}
	for _, change := range []string{"uid", "role", "workflow", "namespace"} {
		t.Run(change, func(t *testing.T) {
			claim, role := association, "workspace-name"
			if change == "uid" {
				claim.PodUID = "different-pod-uid"
			}
			if change == "role" {
				role = "publish"
			}
			if change == "workflow" {
				claim.WorkflowUID = "different-controller-uid"
			}
			if change == "namespace" {
				claim.NamespaceUID = "different-namespace-uid"
			}
			if id, err := f.verifier().ResolveManagedTaskID(context.Background(), f.dispatch.Plan, claim, role); err == nil || id != "" {
				t.Fatal("unproven task identifier disclosed")
			}
		})
	}
}
