# KFP CreateRun boundary (CPU07 candidate)

This package is an unwired first TDD slice. Fixed commit `ffa993b` first produced
Fedora behavior RED against an explicit `KFP_CREATE_RUN_NOT_IMPLEMENTED` stub.
The candidate implementation sends once and confirms only a bounded, complete
response matching the frozen request. HTTP fixture checks and race passed at
`241cec3`; they do not establish real KFP or production integration.
It has no durable submission
worker, Run authority binding, training creation permit, or product entry point.

The target wire contract is the official candidate KFP **2.16.0**
[Run API](https://github.com/kubeflow/pipelines/blob/2.16.0/backend/api/v2beta1/run.proto#L60-L67)
and [RuntimeConfig](https://github.com/kubeflow/pipelines/blob/2.16.0/backend/api/v2beta1/runtime_config.proto#L21-L30).
The module intentionally implements only CreateRun through standard net/http;
no KFP SDK module dependency, browser-header forwarding, or retry API is added.
ENV still reports NOT_RUN and supplies no usable endpoint or real binding.

The owner supplies an explicit HTTPS endpoint, CA trust, connection reference
and timeout. The constructor has no artifact root; the sole CreateRun input is
`PipelineCreateRequest{Admission, Plan, Permit}`. Short-lived bearer identity is regenerated from
the trusted tenant and frozen binding through TokenProvider. The provider must
independently enforce trusted tenant identity; its production implementation is
not supplied here. It must not use the audit actor as enduring permission.
Connection inputs and credentials do not come from public request parameters.
Fixture tokens are synthetic and no credential or raw response body is logged.

PipelineRoot is KFP storage, not PublicationScope. The shared execution snapshot
has no pipeline_root field. The internal dispatch plan separately freezes the
explicit owner reference, revision digest and root; the public SpecHash does not
cover root. The adapter checks the complete admission-to-plan canonical mapping
and permit identity/hash, then encodes PipelineVersion, Experiment, managed SA
and root from that plan. The caller must supply the original committed
reservation; structurally valid values are not proof of persistence or current
authorization. Only `execution_id` and `spec_hash`
are planned long-lived parameters. Display name is not an idempotency key.

The submitting caller must commit SUBMITTING under the shared identity/close
fence before this call. The first PG-plus-TLS submitter test reached its explicit
orchestration stub at fixed `066b331`: real Admission/dependency preflight passed,
then Submit returned PERSISTENCE_UNAVAILABLE as the expected behavior RED.
KFP regression passed at the same source. The next implementation candidate
connects the real reservation, one POST and bounded observation persistence;
its fixed-source GREEN is still pending. This is not leased recovery.
NotSent/Uncertain/Confirmed describe the observed
call only; persistence and authoritative Run CAS remain separate. The candidate
must not follow redirects or automatically repeat a POST. A sent request with
lost, malformed, oversized or otherwise untrusted response remains uncertain;
an HTTP error alone does not prove no creation occurred. Confirmation requires
HTTP 200 JSON at most 1 MiB, no duplicate keys or error field, a nonzero
Run UUID in standard hyphenated form and matching Experiment, display name,
PipelineVersion, managed SA,
execution/spec parameters and root. Extra top-level output fields are allowed.
Run UUIDs may contain uppercase hexadecimal and are returned in lowercase.
A confirmed creation response may already report failed computation; it is not
training success, verified namespace/Pod identity, or authority.

The fixed upstream [API converter](https://github.com/kubeflow/pipelines/blob/2.16.0/backend/src/apiserver/server/api_converter.go#L1430-L1503)
can return HTTP 200 with Run ID and an error after conversion failure. The
response validator retains uncertainty for that case. No 4xx/5xx status is
interpreted here as proof of rejection without side effect.

The first test uses an in-process TLS server which receives the request then
disconnects. It checks one POST, fixed payload and uncertain outcome. It is
neither real KFP nor proof of multi-user authorization, storage, durable worker
recovery, real callbacks, Trainer creation, or LIVE. All execution and format
checks run only on Fedora at an immutable source SHA. Lost-response behavior
passed at `f16bcde`, including race. Second response behavior RED at `ff3329c`
failed only the two expected complete-response cases (pending and already failed
computation); the 27 uncertain-response cases and original disconnect case
passed. Response validation GREEN and race passed at `241cec3` (two main tests,
29 response cases). Boundary regression at `8ef410d` also passed with race:
six main tests / 61 table cases include protected constructor configuration,
preflight/credential failures, cancellation after observed receipt, CA rejection,
null/UTF-8/depth limits and uppercase UUID normalization. These were regression
passes against the existing implementation, not a new RED/fix cycle. Real
provider and product entry-point wiring remain unfinished. The submitter's
NOT_SENT persistence is a required next behavior: the current candidate returns
a persistence error and retains that transient observation, without inventing
UNCERTAIN, granting another permit or claiming the whole use case complete.
Existing root-rejection cases
now target the frozen request rather than removed constructor configuration;
their zero-credential/zero-HTTP assertions remain. The new candidate's test
results must be recorded separately from those historical fixed-source passes.
