# CPU-P01 collected output manifest

Source: CPU08 and D08–D12 in the frozen task package referenced by
`.scratch/cpu-p01/spec.md`. This is the collected-file manifest, not a replacement
for `ani.training.v1.TrainingResult`, a publication receipt, or upload evidence.

`contract/cpup01.OutputManifestBytes` is the shared canonicalization owner. It
validates a complete `AdmissionEnvelope` and an independently collected inventory.
The inventory must contain exactly the four files required by the frozen output
contract, with matching roles, nonzero bounded sizes, and lowercase SHA256 values.
Duplicate, additional, missing, or differently named files are rejected. The
caller retains ownership of its slices; canonicalization does not mutate them.

Schema `ani.modeldev.output-manifest.v1` contains, in declaration order:

- schema, tenant_id, operation_id, execution_id, execution_spec_hash;
- input_version_id, input_sha256, release_id, image_digest;
- storage_state (always WORKSPACE_ONLY), files.

UUIDs are lowercase. Files are sorted by relative_path; each entry contains
relative_path, role, size_bytes (decimal JSON string), and sha256. JSON uses UTF-8,
no optional whitespace or trailing newline, and no HTML escaping. The digest is
SHA256 over those exact returned bytes. The manifest does not include its own
hash. A future tar bundle has a separate digest over the actual archive bytes.
Snapshot provenance binds all remaining frozen configuration through
execution_spec_hash; credentials, temporary URLs, and current Release pointers
are never included.

The workspace adapter reads the actual four file bodies safely and produces the
manifest in memory with its distinct hash. It does not trust `result.json`, write
over the only output copy, publish objects, or declare that writers have stopped.
Its caller must establish the managed workspace and resource UID binding and the
required write-closure facts before collection. TrainingResult still supplies
outcome, completion time, workspace identity, logs, and metrics as separate facts.

Publication remains a later owner operation: confirm the actual uploader has
completed, verify approved fixed object references and their remote bytes, then
persist publication and artifact records before permitting downloads. KFP
launcher success alone and this collected manifest alone are insufficient.

Contract tests and real local-file module tests execute only on Fedora at fixed
source commits. Their small synthetic bytes are not a real PyTorch checkpoint,
S3 verification, or an independent BFF-authorized verifier Job.
