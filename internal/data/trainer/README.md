# CPU06 Trainer adapter

Source task: `/home/chabking/workspace/ANI-doc/02-issues/modeldev/cpu-p01/tasks/CPU06.md`.
Source contract: CPU-P01 `reference/v04/source/development.md`, D08 and D12;
current ModelDev `TrainingStatus` / `ResourceObservation` Proto and domain model.

The first slice exercises `Adapter.ObserveTrainJob` through the official
Kubernetes dynamic client against an explicit HTTP API substitute. A bound
resource with `Suspended=True` followed by `Complete=False` must remain
non-complete; the observer preserves the resource's actual UID and generation.
It returns controller conditions only. Pod exit, writer absence, publication,
authorization and durable creation permission remain separate required facts.

Pinned upstream design source:

- [Trainer v2.1.0 API](https://github.com/kubeflow/trainer/blob/v2.1.0/pkg/apis/trainer/v1alpha1/trainjob_types.go)
  defines TrainJob `Suspended`, `Complete`, `Failed` as independent conditions.
- [Trainer v2.1.0 dependency definition](https://github.com/kubeflow/trainer/blob/v2.1.0/go.mod)
  selects Kubernetes client-go/apimachinery v0.34.1. Use these fixed client
  versions for the initial adapter dependency candidate; do not infer a deployed
  Trainer version from this source reference.

The test uses only read requests. It is not a second training entrypoint and is
not wired to product transports. Namespace UID, exact TrainJob UID and owner
annotations belong to the binding/reuse checks in this adapter; subsequent
test slices must reject mismatches before returning usable observations.
The annotations `modeldev.ani.io/tenant-id`, `modeldev.ani.io/execution-id` and
`modeldev.ani.io/execution-spec-sha256` are the proposed ModelDev-owned resource
binding keys. The full 64-character spec hash is an annotation, not a label.

Initial production method is an explicit `CPU06_NOT_IMPLEMENTED` stub. The
first test has not run and RED is NOT_RUN. Missing SDK dependencies or formatting
are tool prerequisites, not behavior RED. Root owns go.mod/go.sum updates; all
dependency resolution, gofmt and tests run on Fedora against fixed Git source,
and generated/format changes return for review and a new fixed commit.

The latest consumed ENV record still lacks a usable Trainer CRD/Runtime and
application-identity handoff. Server-side dry-run and real resource observation
are NOT_RUN. These missing LIVE inputs do not block the adapter's module tests.
