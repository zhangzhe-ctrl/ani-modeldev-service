# KFP CreateRun boundary (CPU07 candidate)

This package is an unwired first TDD slice. Fixed commit `ffa993b` first produced
Fedora behavior RED against an explicit `KFP_CREATE_RUN_NOT_IMPLEMENTED` stub.
The candidate implementation sends once and conservatively treats every
response as uncertain; confirmed-response validation is still outstanding.
It has no durable submission
worker, Run authority binding, training creation permit, or product entry point.

The target wire contract is the official candidate KFP **2.16.0**
[Run API](https://github.com/kubeflow/pipelines/blob/2.16.0/backend/api/v2beta1/run.proto#L60-L67)
and [RuntimeConfig](https://github.com/kubeflow/pipelines/blob/2.16.0/backend/api/v2beta1/runtime_config.proto#L21-L30).
The module intentionally implements only CreateRun through standard net/http;
no KFP SDK module dependency, browser-header forwarding, or retry API is added.
ENV still reports NOT_RUN and supplies no usable endpoint or real binding.

The owner supplies an explicit HTTPS endpoint, CA trust, connection reference,
timeout and KFP artifact root. Short-lived bearer identity is regenerated from
the trusted tenant and frozen binding through TokenProvider. The provider must
independently enforce trusted tenant identity; its production implementation is
not supplied here. It must not use the audit actor as enduring permission.
Connection inputs and credentials do not come from public request parameters.
Fixture tokens are synthetic and no credential or raw response body is logged.

PipelineRoot is KFP storage, not PublicationScope. The present shared execution
snapshot has no pipeline_root field. Before product wiring, its immutable
resolution must become part of the frozen execution contract; fixture wire
tests do not close this gap. PipelineVersion, Experiment and managed SA are
read from the validated frozen admission. Only `execution_id` and `spec_hash`
are planned long-lived parameters. Display name is not an idempotency key.

The future leased worker must commit SUBMITTING under the shared identity/close
fence before this call. NotSent/Uncertain/Confirmed will describe the observed
call only; persistence and authoritative Run CAS remain separate. The candidate
must not follow redirects or automatically repeat a POST. A sent request with
lost, malformed, oversized or otherwise untrusted response remains uncertain;
an HTTP error alone does not prove no creation occurred.

The first test uses an in-process TLS server which receives the request then
disconnects. It checks one POST, fixed payload and uncertain outcome. It is
neither real KFP nor proof of multi-user authorization, storage, durable worker
recovery, real callbacks, Trainer creation, or LIVE. All execution and format
checks run only on Fedora at an immutable source SHA. The lost-response test
is the first behavior; malformed-response, constructor, preflight and redirect
negative tests and response confirmation remain follow-on verification.
