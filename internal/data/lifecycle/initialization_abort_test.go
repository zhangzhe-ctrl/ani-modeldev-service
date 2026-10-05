package lifecycle

import (
	"errors"
	"testing"
	"time"

	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/biz"
)

func TestInitializationAbortCloseEvidenceIsOwnerOnlyAndCannotBecomeSuccess(t *testing.T) {
	now := time.Now().UTC()
	authority := biz.RunAuthorityCandidate{RunID: "run-id", WorkflowUID: "workflow-uid", NamespaceName: "tenant-ns"}
	for _, attack := range []string{"valid", "after-deletion", "future-stop", "ordinary-managed", "natural-success", "mixed-exit", "training-kind", "foreign-owner", "missing-object", "creation-open", "missing-sandbox", "suffix-mismatch", "late-exit"} {
		t.Run(attack, func(t *testing.T) {
			proof := &biz.PodInitializationAbort{PodResourceVersion: "1170948", NodeName: "ani-01", Phase: "Failed", RestartPolicy: "Never", PodGeneration: 2, DeletedAt: now.Add(-time.Second), NotInitializedAt: now.Add(-10 * time.Second), SandboxStoppedAt: now.Add(-2 * time.Second), DeclaredInitContainers: []string{"init", "kfp-launcher"}, DeclaredContainers: []string{"main", "wait"}, InitContainerExits: []biz.InitializationContainerExit{{Name: "init", ContainerID: "containerd://actual-init", ImageID: "actual-init-image", StartedAt: now.Add(-5 * time.Second), FinishedAt: now.Add(-3 * time.Second)}}, UnstartedInitContainers: []string{"kfp-launcher"}, UnstartedContainers: []string{"main", "wait"}}
			evidence := biz.ManagedCloseEvidence{RunID: authority.RunID, WorkflowUID: authority.WorkflowUID, ObservedAt: now, OwnerTermination: &biz.ManagedOwnerTermination{RunState: "CANCELED", RunFinishedAt: now.Add(-time.Second), WorkflowPhase: "Failed", WorkflowFinishedAt: now.Add(-time.Second), WorkflowResourceVersion: "123"}, Resources: []biz.RuntimeResource{{APIVersion: "v1", Kind: "Pod", Namespace: authority.NamespaceName, Name: "aborted-pod", UID: "aborted-pod-uid", OwnerUID: authority.WorkflowUID, APIObjectPresent: true, Terminal: true, CreationDisabled: true, InitializationAbort: proof}}}
			reason := "DEADLINE"
			switch attack {
			case "after-deletion":
				proof.SandboxStoppedAt = now.Add(-500 * time.Millisecond)
			case "future-stop":
				proof.SandboxStoppedAt = now.Add(time.Minute)
			case "ordinary-managed":
				evidence.OwnerTermination = nil
				evidence.SkippedTasks = []biz.ManagedSkippedTask{{TaskName: "train-wait", TaskID: "wait-task"}, {TaskName: "collect", TaskID: "collect-task"}, {TaskName: "publish", TaskID: "publish-task"}}
			case "natural-success":
				reason = "NATURAL_TERMINAL"
			case "mixed-exit":
				zero := int32(0)
				evidence.Resources[0].ExitCode = &zero
			case "training-kind":
				evidence.Resources[0].Kind = "TrainJob"
			case "foreign-owner":
				evidence.Resources[0].OwnerUID = "another-workflow"
			case "missing-object":
				evidence.Resources[0].APIObjectPresent = false
			case "creation-open":
				evidence.Resources[0].CreationDisabled = false
			case "missing-sandbox":
				proof.SandboxStoppedAt = time.Time{}
			case "suffix-mismatch":
				proof.UnstartedInitContainers = []string{"wrong-init"}
			case "late-exit":
				proof.InitContainerExits[0].FinishedAt = now
			}
			if got := validCloseEvidence(authority, reason, evidence); got != (attack == "valid" || attack == "after-deletion") {
				t.Fatalf("%s close evidence acceptance=%v", attack, got)
			}
		})
	}
}

func TestInitializationAbortCannotBeUsedAsTrainingObservation(t *testing.T) {
	state := biz.ExecutionRuntime{Workspace: &biz.WorkspaceBinding{NamespaceName: "tenant-ns"}, Training: &biz.TrainingPlan{Name: "training"}, TrainingHandle: &biz.TrainingHandle{TrainJobUID: "training-uid"}}
	observation := biz.TrainingRuntimeObservation{Handle: *state.TrainingHandle, Outcome: "FAILED", ObservedAt: time.Now().UTC(), Resources: []biz.RuntimeResource{{APIVersion: "trainer.kubeflow.org/v1alpha1", Kind: "TrainJob", Namespace: "tenant-ns", Name: "training", UID: "training-uid", APIObjectPresent: true, Terminal: true, InitializationAbort: &biz.PodInitializationAbort{}}}}
	if _, err := mergeObservation(state, observation); !errors.Is(err, biz.ErrRuntimeConflict) {
		t.Fatalf("training borrowed owner-only initialization abort: %v", err)
	}
}
