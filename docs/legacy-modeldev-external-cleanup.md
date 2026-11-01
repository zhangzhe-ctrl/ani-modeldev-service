# Fixed legacy ModelDev external cleanup

This one-time CLI implements the retained-object part of specification13.3.10
and records the metadata/configuration boundary of13.3.11. It is outside the
service, never creates or closes an execution and never changes authorization.
No live cleanup has been run by this implementation task.

`CleanupPlanFor`/`ApplyExecutionCleanup` in
`internal/biz/execution_operations.go` deliberately retain PVC, S3 publication
and execution/audit records. Those operations and the original owner retirement
must run first; their success cannot substitute for this tool's exact retained
object inventory. This tool does not repeat their workload disposal or alter
any close, log, cleanup, GPU scope/outbox/ACK qualification.

## Fixed scope and evidence boundary

The CLI accepts only the original `old-inventory.json` bytes, SHA256
`aa5420bd1c2ad02f373509d625a99080b4c6721050de1bcdb3496ad1d15a44d2`.
It derives the50 executions,50 original Runs, original pipeline/version and
Experiment IDs,42 recorded PVC identities and original workload UID identities.
Namespace `ani-modeldev-tenant-a-v21` must retain UID
`94f3afc0-7548-4099-9e11-3fa7fdf6cc67`; binding is
`e91440de-0cef-4d88-b9d8-83126f8f8132`. Namespace deletion is unavailable.
Standard A/B Namespaces, shared image Secrets, external inputs/base models and
independent READY Models are preserved. Old input/output physical bytes may be
retained; that does **not** exempt ModelDev-owned KFP execution artifacts.

The original inventory does not provide present UID/RV for all old ConfigMaps,
ServiceAccounts, Roles and RoleBindings, or a complete S3 version inventory.
The root executor must first collect and review those exact facts. The default
plan is BLOCKED when they or any fresh qualification are missing. It does not
adopt an object because its name, namespace or owner label looks familiar.
Even a BLOCKED plan lists frozen PVC/workload UID and KFP identity candidates,
explicitly marked as inventory facts rather than current cluster observations.

Source-proof paths plus SHA256 are an explicit root trust boundary. The tool
verifies their actual bytes and exact receipt/manifest identities. It does not
authenticate the meaning of arbitrary evidence text. Root must independently
validate original close/cleanup/ACK qualifications, stopped sources, real
authenticated maintenance503, ownership and completeness before approving the
scope and prerequisites file hashes. A JSON boolean cannot replace these files.
In particular, ordinary f216 and the four UNKNOWN acceptances receive no new
qualification or exemption. A Run404 is only current record absence, never
proof that its original Workflow or writers exited.

## Invocation

Build/test work runs on Fedora. Source is transferred as individual files or
rsync, never packaged on Fedora. All live access uses one bounded SSH channel
to `kubercloud@100.64.128.12`; there is no alternate host, local cluster client,
automatic reconnect, API retry, privilege grant or credential minting.

```sh
python3 scripts/legacy-modeldev-external-cleanup/cleanup.py \
  --inventory /evidence/09/old-inventory.json \
  --connection /private/external-connections.json \
  --connection-sha256 <root-reviewed-connection-file-byte-SHA256> \
  --scope /private/external-object-scope.json \
  --scope-sha256 <root-reviewed-scope-file-byte-SHA256> \
  --prerequisites /private/original-close-and-stopped-sources.json \
  --prerequisites-sha256 <root-reviewed-prerequisites-file-byte-SHA256> \
  > /private/external-cleanup-plan.json
```

`--mode` defaults to `plan`. To apply that exact reviewed file, repeat the same
arguments and add:

```sh
--mode apply --plan /private/external-cleanup-plan.json \
--plan-sha256 <actual-reviewed-plan-file-byte-SHA256> \
--receipt /private/new-external-cleanup-journal.json
```

`--receipt` must be new. Each transition uses a mode0600 temporary file,
fsync, atomic replace and parent-directory fsync. Intent is durable before
each DELETE. A lost response remains RESULT_UNKNOWN; unfinished cleanup is
PARTIAL. No automatic retry/force/finalizer removal occurs. Retain the journal,
reconcile original IDs and actual state, obtain fresh root qualifications and
generate a new reviewed plan before continuing. Never reuse a successful
boolean or overwrite the original journal. Fresh proofs expire within5minutes;
expiry during a long run preserves a PARTIAL result.

## Exact file interfaces

All JSON fields below are lowercase as defined by `cleanup.py`. Duplicate JSON
fields are rejected. Evidence must be absolute regular files, not symlinks,
bounded to16MiB. `source_proof` values are exactly
`{"path":"/absolute/evidence/file","sha256":"<actual-byte-SHA256>"}`.

The nonsecret connection file contains only two objects:

```json
{
  "kfp": {"endpoint":"https://existing-kfp-endpoint", "ca_file":"/node/public-ca", "token_file":"/node/private-existing-control-token", "control_pod_uid":"<verified-existing-token-bound-pod-UID>"},
  "s3": {"endpoint":"https://existing-s3-endpoint", "ca_file":"/node/public-ca", "credentials_file":"/node/private-existing-aws-profile", "config_file":"/node/private-existing-aws-config", "profile":"<existing-profile-name>"}
}
```

Private token/credential/config files are read only on the approved node and
must be regular bounded files without group/other access. Token subject must
be `system:serviceaccount:ani-system:modeldev-control`, audience exactly
`pipelines.kubeflow.org`, unexpired and bound to the reviewed control Pod UID.
TLS CA verification remains enabled, redirects and ambient proxies disabled.
S3 uses the existing node AWS CLI/profile with retries disabled; missing CLI,
credentials or read/delete authority is a blocker, not permission to add it.
Credentials and raw artifact bytes never cross SSH or enter output.

Scope schema is `ani.modeldev.external-cleanup.scope.v1`, with exactly:

- `inventory_sha256`, `binding_id`, `connection_sha256` fixed to the reviewed
  original inventory/binding and connection-file hash;
- `configuration_inventory_proof`, `artifact_inventory_proof`: root-reviewed
  complete exact inventory/exclusions source files, not completion flags;
- `kube_targets`: each has `api_version`, `kind`, `namespace`, `name`, `uid`,
  `resource_version`, `object_sha256`, `exclusive_owner_proof`;
- `kfp_targets`: each has `kind`, `id`, `object_sha256`,
  `exclusive_owner_proof`. Kinds: `runs`, `pipeline_versions`, `pipelines`,
  `experiments`;
- `kfp_preserved`: only shared pipeline/version candidates, each has `kind`,
  `id`, `reason:"SHARED_NON_MODELDEV_OWNER"`, `shared_owner_proof`. Every
  original candidate must be classified exactly once. All Runs and the old
  dedicated Experiment must be disposed. A parent Pipeline cannot be deleted
  while a version is preserved or unreviewed;
- `s3_versions`: each has `bucket`, `key`, `version_id`, `etag`, `size_bytes`,
  `sha256`, `run_id`, `exclusive_owner_proof`. It must be a specific non-null
  immutable version under `artifacts/md/<original-Run-ID>/`, with complete
  exclusive ModelDev ownership/version-list evidence. Empty lists require
  actual complete inventory evidence, never inference from a prefix/404.

`object_sha256` hashes compact canonical JSON with sorted keys, UTF-8 and no
extra whitespace. KFP Run hashes cover the full v1 RunDetail. Pipeline hashes
cover the independent v2 Pipeline response, which uses `pipeline_id`.

Prerequisites schema is `ani.modeldev.external-cleanup.prerequisites.v1`, with
exactly `inventory_sha256`, `scope_sha256`, `identity`, `observed_at`,
`expires_at`, `executions`, `stopped_sources`, `maintenance_503`,
`all_original_charges_and_ack_proof`, plus `schema`. `identity` is exactly the
five fixed SSH/context/API-server/cluster-ID/kube-system-UID fields in
`cleanup.py`. Times include UTC offsets and span at most5minutes.

Each original execution appears exactly once as
`execution_id`, `operation_id`, `spec_hash`, `qualification_proof`. That proof
must establish the actual original reader/approved finite retirement path,
closed fence, step/compute cleanup and matched original GPU ACK as applicable.
The tool does not reinterpret or repair those domain facts.

`stopped_sources` contains exactly MODELDEV_WORKERS,
GOVERNANCE_CREATE_PUBLISHER and LEGACY_REPLAY_AND_PUBLICATION; each has
`component`, `source_uid`, `source_revision`, `proof`. Proofs must establish
no active lease, in-flight or late original Create/publication/replay writer.
`maintenance_503` contains method POST, path `/api/v1/modeldev/executions`,
status503, actual `request_id`, and `proof`; schema/auth errors are insufficient.
`all_original_charges_and_ack_proof` binds original full release, matching ACKs
and all-owner account equality. Unknown qualifications prohibit apply.

## Disposal and independent observations

Before every mutation, recheck complete cluster identity, old Namespace UID,
fresh prerequisites, original workload absence and controller/reference lists.
Identity reads reuse the existing sole-entry kubectl context and verify
cluster ID from the existing Resource configuration without printing it.
Kube delete uses both UID and resourceVersion preconditions with Orphan
propagation. It never removes finalizers or shortens grace. Unsupported
resources, replacements, changed bytes, deleting objects and any live/retained
reference block apply. Read-only list pagination is bounded and must finish;
permission/API/pagination failure cannot mean an empty list.

The entire original recorded workload graph must already be absent. Completed
Workflow/Pod objects must still go through original owner cleanup/qualification;
this tool does not delete them to manufacture that precondition. Current Pods,
Deployment/ReplicaSet/StatefulSet/DaemonSet/Job/CronJob templates, old namespace
Workflow/TrainJob/JobSet/TrainingRuntime declarations and global RoleBindings/
ClusterRoleBindings are checked for references to the retained objects. Even a
suspended or zero-replica controller counts as a reference.

S3 HEAD and actual GET verify the exact immutable VersionID, ETag, length and
SHA before version-specific DELETE. The per-version bound is64MiB; larger or
unversioned artifacts are rejected and require a separately implemented exact
owner tool. Old input/output retention never applies to these execution
artifacts. KFP Run deletion occurs only after independent original workload
absence. Complete global Run/recurring-job and per-pipeline version references
are checked; lack of current principal visibility blocks. Versions are removed
individually, then non-cascade Pipeline deletion requires zero remaining
versions, then the original Experiment. KFP has no resourceVersion/CAS delete:
the tool binds immutable original IDs, reviewed response hashes, repeated reads
and the fresh stopped/exclusive-source proof; it does not claim atomic KFP CAS.
Every DELETE is followed by an independent read; 2xx alone is insufficient.

The API behavior is grounded in official KFP2.16
[Run API](https://raw.githubusercontent.com/kubeflow/pipelines/2.16.0/backend/api/v1beta1/swagger/run.swagger.json),
[Pipeline API](https://raw.githubusercontent.com/kubeflow/pipelines/2.16.0/backend/api/v1beta1/swagger/pipeline.swagger.json)
and [Pipeline server](https://raw.githubusercontent.com/kubeflow/pipelines/2.16.0/backend/src/apiserver/server/pipeline_server.go).
V1 Pipeline GET needs a latest version, so the independent v2 GET is used after
version disposal; v2 DELETE receives no cascade flag.

## DB handoff and remaining boundaries

The ten original InputVersion metadata IDs are reported and delegated to
`scripts/legacy-modeldev-data-cleanup`; there is no second metadata DELETE or
account SQL here. After actual external completion, root binds this full
journal's byte hash and exact per-ID source records into the DB tool's fresh
KUBERNETES_ORIGINAL_UIDS, KFP_ORIGINAL_RUN_WORKFLOW and S3_OWNED_OBJECT_VERSIONS
proofs. `APPLIED_EXTERNAL_ONLY` covers only the reviewed scope. Root must verify
remaining configuration/deployment and historical-resource boundaries before
issuing any complete DB root receipt; this state alone is not that receipt.

Unsupported/NOT_IMPLEMENTED here: deletion of cluster-scoped old webhook/VAP
objects, mutation of shared Deployment mounts/env/image/material references,
unknown/unrecorded PVC UID adoption, S3 objects outside the exact Run artifact
prefix or above64MiB/unversioned objects, and historical extra Gov resources
whose original external IDs are absent from the MD inventory. These cannot be
silently counted as cleared. The root-owned Workbench/source switch and a
newly reviewed exact owner cleanup implementation/evidence handle those facts.
Configuration still referenced by the old Deployment intentionally blocks;
the tool does not alter that running entry or expand its permissions.

Live plan/apply, actual object counts/zero-old-data verification, fresh root
proof collection and final A/B acceptance remain NOT_RUN. Component peer tests
exercise real plan/apply/journal logic with external Kubernetes/KFP/S3 doubles;
they do not establish live deletion or product acceptance. Full repository
tests are not rerun for this isolated maintenance script.


## Component validation

Fedora completed the final unchanged three-source snapshot between
`2026-11-01T00:44:47.404158+00:00` and `2026-11-01T00:44:48.258289+00:00`. Source manifest SHA256:
`c5c960701b7e211569168f8ef85e68dd920fda13e3d7c6170de530784cd3526e`; local and Fedora file hashes match.

- `python3 -m unittest discover ... -p test_*.py -v`:13 tests, exit0.
- `python3 -m py_compile cleanup.py ssh_peer.py test_cleanup.py`:exit0.
- Actual CLI against the pinned original inventory with scope omitted:exit0,
  BLOCKED `EXACT_UID_CONFIGURATION_AND_ARTIFACT_SCOPE_UNKNOWN`,10 original
  metadata IDs reported and no SSH connection opened.

The vertical fixture exercises the actual planner/apply/journal, with only
external peers doubled. It checks UID/RV preconditions, S3 exact version/bytes,
Run→Version→non-cascade Pipeline→Experiment order, retained namespace/input/
shared-Secret boundaries, replaced identity, foreign suspended-controller and
Run/recurring references, remaining original Workflow, missing fresh proof,
expired/changed proof, lost DELETE response/no retry and complete pagination.
A real Python node-helper subprocess additionally parses the full fixed
cluster identity and executes external kubectl/aws fixture programs, checking
actual stdin DeleteOptions and exact-version download/hash/delete arguments.
This does not run live kubectl/AWS/KFP against the test cluster.

Raw `receipt.json`, `targeted.log`, `compile.log`, `default-readonly.log` are
retained under the task's `legacy-external-cleanup-final` component evidence.
No full service gate, live cleanup or acceptance is claimed by these results.
