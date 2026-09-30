# CPU-P01 API source

These are candidate typed contracts for the existing BFF/Governance and the
ModelDev owner. Their existence does not assert that a handler, authorization
chain, database, KFP/Trainer adapter, or live endpoint has been implemented.

`ani/modeldev/v1/modeldev.proto` separates three authenticated surfaces:

- `ModelDevQueryService` serves authorized BFF reads and download authorization.
  Current permissions apply to every request, including the original actor.
- `ModelDevCommandService` durably receives Governance commands and close
  intents. It acknowledges only committed inbox records or close tombstones.
- `ModelDevStepService` authenticates managed KFP steps, verifies actual
  Run/Workflow/Pod association, and enforces the one authoritative Run binding.
  Ordinary training containers never receive these capabilities.

CreateExecution and StopExecution remain owned by Governance behind the
existing BFF; this module deliberately defines no second facade service.
UserIntent is the shared typed intent delivered with AcceptExecution. The BFF
must strictly decode the original external JSON, preserving absent optional
fields versus explicit values before mapping to protobuf. In particular the
external general_parameters array maps to a present ParameterSelection message
even when empty; omission maps to an absent message. Direct proto-JSON mapping
would change that external shape and is not the public request decoder.

`ani/training/v1/training.proto` defines typed CPU resources, immutable input,
program/Runtime/output references, and TrainingInput/TrainingResult. It has no
GPU, fine-tuning, user CRD, or model-registration capability.

Admission freezes ExecutionSnapshot. Its existing Namespace UID is fixed, but
later Run/PVC/TrainJob/JobSet/Pod UIDs remain separate runtime facts. Program
arguments are ordered arrays and parameters have typed oneof values. Canonical
hashing is the versioned business contract, never protobuf wire serialization.
The user request cannot select a tenant, actor, cluster, service account,
endpoint, arbitrary environment, command, path, or delivery mode. Trusted
Governance actor claims are audit strings; the resource tenant is a UUID.

TrainingResult and PublicationCandidate are claims to verify. A
VerifiedPublication is emitted only after actual upload completion, remote byte
length/SHA256 verification, and durable storage of the publication. Each file,
the manifest, and an optional tar bundle has its own byte length and digest;
the manifest cannot recursively contain its own digest. A publication receipt
does not prove compute closure or authorize cleanup.

Zero/UNSPECIFIED, malformed IDs/digests, unknown fields, invalid explicit nulls,
unset parameter values, and duplicate parameter names are rejected by the
owning application boundary. The first recipe accepts epochs INTEGER=3,
batch_size INTEGER=64, and learning_rate DECIMAL in (0, 0.1]. Explicit parameter
defaults remain distinct from omitted parameters for intent hashing. Default
resolution happens only while freezing the execution snapshot.

Generate and verify with the repository's pinned workflow on Fedora. Do not
edit generated protobuf output by hand or run generation on the workstation.
The domain state, persistence, authorization, and canonicalization contracts
are maintained by the owning implementation, with evidence recorded in the
CPU-P01 execution cards.
