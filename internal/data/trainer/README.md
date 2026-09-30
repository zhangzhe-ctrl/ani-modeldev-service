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
annotations belong to the binding/reuse checks in this adapter. The first
implementation validates these bindings before returning observations; dedicated
negative test slices are still required before claiming the adapter is ready.
The annotations `modeldev.ani.io/tenant-id`, `modeldev.ani.io/execution-id` and
`modeldev.ani.io/execution-spec-sha256` are the proposed ModelDev-owned resource
binding keys. The full 64-character spec hash is an annotation, not a label.

The initial production method was an explicit `CPU06_NOT_IMPLEMENTED` stub.
Missing SDK dependencies or formatting are tool prerequisites, not behavior RED.
Root owns go.mod/go.sum updates; all
dependency resolution, gofmt and tests run on Fedora against fixed Git source,
and generated/format changes return for review and a new fixed commit.

At source `a4aa47a1d7bfce8544d60650a02f332152d25008`, Fedora Go
`go1.26.7-X:nodwarf5` completed official-proxy `go mod tidy`, `go mod download`
and the three owned Go files' gofmt with exit 0. `GOPROXY=https://proxy.golang.org`
and `GOSUMDB=sum.golang.org` remained enabled. Resolved module sums, generated
file SHA256s and the exact command are archived in the CPU06 run evidence.
The generated go.mod/go.sum and formatting were reviewed and returned locally;
this material was subsequently fixed in a new commit.

The named observer test at `af2407b2eb6a7a90b07a0f51b879b6be966c3bd2` first
hit a cgo `/tmp` quota failure before compilation completed. That attempt is an
environment failure, not RED. A second attempt used task-owned TMPDIR as well as
GOTMPDIR and completed compilation; the test failed in 0.006 seconds, exit 1,
with `observe bound TrainJob: CPU06_NOT_IMPLEMENTED`.

The local first implementation now performs real namespace and TrainJob GETs via
the official client, checks names/namespace, Namespace UID, exact TrainJob UID,
tenant/execution ownership and execution-spec annotation, and preserves each
controller condition separately. Invalid metadata or conflicting terminal
conditions fail closed; missing/stale/unversioned conditions remain Unknown.
Annotation comparison establishes resource correlation, not an independent hash
of the entire rendered CRD spec. Full create-intent/spec reconciliation and Pod
exit/writer history remain future slices. No product transport is wired, no create
operation exists yet, and this is IN_PROGRESS rather than CODE_READY. GREEN and
focused binding-negative RED/GREEN await fixed commits; tests were not weakened.

The latest consumed ENV record still lacks a usable Trainer CRD/Runtime and
application-identity handoff. Server-side dry-run and real resource observation
are NOT_RUN. These missing LIVE inputs do not block the adapter's module tests.
