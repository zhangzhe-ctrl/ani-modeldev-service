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
conditions fail closed. The original implementation treated missing/stale/unversioned
condition generations as Unknown; the fixed-upstream review below found this was
an unsupported assumption and records its correction.
Annotation comparison establishes resource correlation, not an independent hash
of the entire rendered CRD spec. Full create-intent/spec reconciliation and Pod
exit/writer history remain future slices. No product transport is wired, no create
operation exists yet, and this is IN_PROGRESS rather than CODE_READY.

The first named observer test passed on Fedora at fixed source
`f2e1efcd952708dbb45307e511665775a128a647` with exit 0 in 0.007 seconds.
The new checkout remained clean. Both TMPDIR and GOTMPDIR use the task directory.
This GREEN covers the Suspended/Complete/Failed distinction and returned identity
through an HTTP API substitute; it does not cover live resource identity.

The next test-only slice covers invalid binding before network access, namespace
recreation before reading a TrainJob, mismatched resource/owner/spec correlation,
unavailable or stale condition generations, ambiguous controller conditions and
preserved NotFound/Forbidden API errors. After Fedora formatting was returned
and committed, all six named tests (including 38 boundary table cases) passed
at `07777765f23013868185afca9dc1e606ad75263f`, exit 0 in 0.036 seconds. The
checkout remained clean. The initial fail-closed implementation already
satisfied these cases, so this is regression PASS with no new behavior RED or
production fix. This evidence remains scoped to the trainer package and the
external HTTP API substitute.

The latest consumed ENV record still lacks a usable Trainer CRD/Runtime and
application-identity handoff. Server-side dry-run and real resource observation
are NOT_RUN. These missing LIVE inputs do not block the adapter's module tests.

## Fixed-upstream condition semantics correction

Spec review of `ca25478...cef25b4` found that the observer erased legitimate
controller reports. CPU06 execution detail 4 requires correct Complete/Failed
interpretation; the exact local task reference
is `ANI-doc/02-issues/modeldev/cpu-p01/tasks/CPU06.md:26`.

The pinned source facts are:

- [Trainer v2.1.0 controller, lines 171–200](https://github.com/kubeflow/trainer/blob/v2.1.0/pkg/controller/trainjob_controller.go#L171-L200)
  emits Suspended/Resumed and its own Failed conditions without observedGeneration.
- [Trainer v2.1.0 JobSet plugin, lines 285–298](https://github.com/kubeflow/trainer/blob/v2.1.0/pkg/runtime/framework/plugins/jobset/jobset.go#L285-L298)
  copies Complete/Failed conditions from JobSet. Their generations, if present,
  cannot be assumed to describe the TrainJob generation.
- [Trainer v2.1.0 go.mod](https://github.com/kubeflow/trainer/blob/v2.1.0/go.mod#L22)
  pins JobSet v0.10.1; its [condition construction, lines 881–945](https://github.com/kubernetes-sigs/jobset/blob/v0.10.1/pkg/controllers/jobset_controller.go#L881-L945)
  adds transition time but omits observedGeneration for Completed.

The corrected boundary preserves each known condition's reported status and
separate metadata: condition presence, optional generation presence and its raw
integer value. Missing conditions remain Unknown; an explicitly reported Unknown
is distinguishable from absence. Null, noninteger or negative generations,
malformed statuses, duplicates and contradictory Complete=True/Failed=True
reports are rejected. No condition generation is compared with TrainJob generation.

These reports do not establish current-spec reconciliation, Pod success,
publication, writer absence, CLOSED, or creation permission. Consumers must
establish those independent facts; no product transport or RPC is wired here.
The earlier stale/future/unversioned Unknown test cases are retained as corrected
report-preservation cases with explicit metadata assertions, based on the upstream
facts above. Existing ownership, missing-condition and API error checks remain.

Fixed candidate `5545f8c9a17d6f7e7eb4fcccfdce6b5d0fbb472b` reached behavioral RED on
Fedora: the trainer package compiled and failed its eight report-preservation
cases plus negative/null generation and conflicting different-generation reports,
exit 1 in 0.047 seconds. The HTTP payloads are source-derived with synthetic
identities and timestamps; they are not live cluster captures. Implementation
`0bd4732` was formatted on Fedora; only observation/test whitespace changed.
After root reviewed and committed that output, fixed source
`0275f4538fd479658f34402746ac55eb6bfe72fc` passed all seven trainer package tests
and 48 table cases in 0.048 seconds, exit 0; the same package passed `-race` in
1.195 seconds, exit 0. The new Fedora worktree remained clean. This closes the
reviewed observer defect within the HTTP boundary; CPU06 remains IN_PROGRESS,
with live observation and the independent Pod/writer facts still NOT_RUN.

## Retaining training exit evidence

First creation requires the frozen Runtime's actual training Pod template at
`spec.template.spec.replicatedJobs[].template.spec.template.metadata.finalizers`
to contain `modeldev.ani.io/training-exit-evidence`. The selected Job must still
be the sole frozen target with the `trainer` ancestor-step label. ModelDev needs
`patch` permission on Pods in the bound tenant namespace. In
[Trainer v2.1.0's override merge](https://github.com/kubeflow/trainer/blob/v2.1.0/pkg/runtime/core/trainingruntime.go#L185-L195),
Pod template override metadata propagates labels and annotations only; placing
this finalizer in a TrainJob override does not retain derived Pods.

The first real all-container exit observation keeps the finalizer and withholds
writer absence so that the exact Pod UID, owner and exit are persisted. A later
observation uses that durable history, revalidates the namespace and complete
TrainJob/JobSet/Job/Pod owner chain, then conditionally removes only ModelDev's
key. The JSON patch tests Pod UID, resourceVersion, ownerReferences and the
complete finalizer list, preserving other controllers' finalizers. Natural
exits may release retention before parent completion; writer absence still
requires the independent parent creation fences or terminal conditions.

Already-created legacy Runs may use `FindTraining`, `ObserveTraining` and
`StopTraining` with their original finalizer-less Runtime, retaining the full
frozen Runtime hash checks. That Runtime cannot authorize new creation. A
previously missing Pod without durable terminal exit remains unresolved: neither
a single NotFound nor a suspended parent recovers the lost process evidence.
