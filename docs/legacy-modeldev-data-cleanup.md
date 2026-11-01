# Fixed legacy ModelDev database cleanup

This one-time command implements specification13.4's database phase after the
external retirement/cleanup sequence. Its only write is exact DELETE in the two
existing databases. It does not stop workers, call Create/Stop/release RPCs,
delete Kubernetes/KFP/S3 objects, change permissions, migrate schemas or write
quota accounts. No live invocation has been run by this implementation task.

The sole CLI accepts the actual original task09 inventory bytes:

- `old-inventory.json`: SHA256 `aa5420bd1c2ad02f373509d625a99080b4c6721050de1bcdb3496ad1d15a44d2`;
  original tenant `64c5c35a-f554-432c-b4bc-7b4f36f08420`,50 MD executions and10 input metadata IDs.
- `old-gov-inventory.json`: SHA256 `66a3e9b4eb4edc931e8bc452d1c48e671d97e3412c820c992b6f28044cf54406`;
  Governance tenant2,54 acceptance IDs,23 operation IDs,23 charge IDs and23
  original receipt IDs. The four UNKNOWN acceptances have no original MD row;
  nine historical Governance resources have no remaining MD aggregate.

No arbitrary tenant, owner, table or SQL selector is exposed. The original
inventories must remain as evidence after database deletion. Metadata cleanup
never deletes original input bytes, base models or independently registered READY
Model assets. Standard A/B Namespaces and all other owners remain outside scope.

## Read-only plans and two independent commits

Build/run only on Fedora. Transfer source or individual tools with rsync;
never package a Fedora archive/image for transfer. Database access must pass
through the approved `kubercloud@100.64.128.12` entrance. Owner DSN files must be
bounded regular files,mode0600; DSN/password/token values are never emitted.

Each command defaults to a read-only plan. It reports exact primary keys,
whole-row hashes,row counts, database/schema/role/server/OID identity and actual
catalog evidence. The compiled baseline records original column types, primary
keys, foreign keys and immutable GPU trigger/body. Unexpected references,
triggers, rewrite rules,RLS,inheritance, table kinds or changed known keys block
apply. The complete current catalog and row fingerprints must match again
under bounded transaction locks. No old payload is converted to v4.

```sh
legacy-modeldev-data-cleanup \
  -modeldev-dsn-file /private/md-owner-dsn \
  -governance-dsn-file /private/gov-owner-dsn \
  -original-inventory-file /evidence/09/old-inventory.json \
  -original-governance-inventory-file /evidence/09/old-gov-inventory.json \
  -phase governance > /private/gov-delete-plan.json
```

Absent external qualifications produce a BLOCKED plan, never permission to
apply. With the independently reviewed root receipt, add:

```sh
-root-evidence-file /private/root-cleanup-evidence.json \
-root-evidence-sha256 <actual-root-approved-file-byte-SHA256>
```

Review the resulting complete plan's `ManifestSHA256`. Apply repeats the same
arguments plus:

```sh
-apply -manifest-file /private/gov-delete-plan.json \
-manifest-sha256 <that-exact-ManifestSHA256>
```

Save its entire successful JSON to `/private/gov-delete-receipt.json`, record its
actual file byte SHA256, then plan MD using the same scope/private DSN arguments:

```sh
-phase modeldev \
-governance-receipt-file /private/gov-delete-receipt.json \
-governance-receipt-sha256 <actual-root-reviewed-receipt-byte-SHA256>
```

MD apply needs its own complete reviewed plan and ManifestSHA256. Governance
and ModelDev cannot share an atomic database transaction. A successful Gov
receipt followed by failed/unknown MD apply is **PARTIAL**. Retain that exact
Gov receipt and replan only MD after resolving its concrete refusal; never
recreate quota records, replay Create or fabricate a new release event. A lost
commit response is **RESULT_UNKNOWN** until the exact original IDs and retained
receipt are reconciled. It is not permission to repeat deletion or reconstruct
a successful receipt from a boolean.

## External root receipt is an explicit trust boundary

The command accepts only a caller-supplied, independently approved **byte hash**
of a strict `ani.modeldev.legacy-cleanup-root-evidence.v1` JSON receipt. Every
source proof names an absolute existing regular evidence file and its byte
SHA256; paths/hashes are checked. This command does not authenticate external
Kubernetes/S3/KFP observations itself. The root executor must verify their
actual source and exact scope before supplying the hash. An agent-created
`completed:true` cannot satisfy this interface.

The receipt uses the exact exported JSON fields in `RootEvidence`:

- `ObservedAt` is the beginning of the fresh verification window; `ExpiresAt`
  is at most five minutes later and must remain valid through commit.
- `SSH`,`Context`,`APIServer`,`ClusterID`,`KubeSystemUID` equal the five fixed
  test identities from the specification. `InventorySHA256` and
  `GovernanceInventorySHA256` equal the original file hashes above.
- `Databases.modeldev` and `.governance` exactly equal the identities emitted
  by the current read-only plan, including schema,role,server and database OID.
- Exactly three `StopRecords` cover `MODELDEV_WORKERS`,
  `GOVERNANCE_CREATE_PUBLISHER` and `LEGACY_REPLAY_AND_PUBLICATION`. Each records
  actual `SourceUID`,`SourceRevision`,`State:"STOPPED"`,`ObservedAt` and
  `Proof:{Kind,Path,SHA256}`. STOPPED describes that component's proven state;
  the Governance server may continue original Stop/release handling.
- `Probe` records the real authenticated POST `/api/v1/modeldev/executions`,
  HTTP503,request ID,time and exact source proof. Input/schema/auth failures
  cannot substitute for the authorized maintenance503.
- `Dispositions` contain exactly each original execution/resource identity,
  original operation and known spec hash. MD50 use
  `CLEANED_ORIGINAL_EXTERNAL_RESOURCES`; the additional historical Gov resources
  use `PRIOR_AUTHENTICATED_OWNER_RELEASE` (their original canonical authenticated
  owner receipt is independently checked in the database). Both require proof
  kinds `KUBERNETES_ORIGINAL_UIDS`,`KFP_ORIGINAL_RUN_WORKFLOW` and
  `S3_OWNED_OBJECT_VERSIONS`, with exact per-ID UID/object-version scope and the
  preservation boundary in the actual cited evidence. The four UNKNOWN use
  `OWNER_CLOSED_WITHOUT_SIDE_EFFECTS` and the independent proof kind
  `AUTHENTICATED_ORIGINAL_OWNER_CLOSED_NO_SIDE_EFFECTS`. A retry-blocked flag,
  absent row or one404 is insufficient. Missing/extra/different identity or
  missing proof kind blocks deletion.

At most80 dispositions,8 proofs per disposition,256 distinct proof files,
8MiB per file and64MiB cumulative proof bytes are accepted. Evidence files are
read and hashed only; their contents and any credentials are not output.

## Database qualifications and exact deletion

Apply locks the MD tables then Gov tables in a stable lock order, waits up to5s
for in-flight writers and rechecks catalog, row snapshots and qualifications.
Each SQL statement has a20s budget; the whole command has a2-minute budget.
Transaction rollback uses a separate bounded cleanup context after cancellation.
The lock order is separate from the fixed FK deletion order.

Ordinary MD executions use the existing strict `lifecycle.ReadInTransaction`
reader, original canonical/spec identity, CLOSED state, real creation fence and
existing step/compute cleanup qualifications. Materialized executions require
`ComputeCleanupApplied`; GPU executions require the actual original scope,
outbox event and matching APPLIED/ALREADY_APPLIED ACK. Original dispatch,
confirmed Run,authority Namespace/Workflow UID and input metadata identities are
compared to the frozen inventory; references from outside the manifest block.

Only the original three approved retirement executions use
`GetLegacyRetirementRecord` and exported strict legacy pure qualifications.
The GPU retirement must retain its original authenticated prior-release journal
and matching ALREADY_APPLIED evidence after compute cleanup. Its aggregate
GPURelease and both SQL release anchors must remain NULL. CPU retirement records
must not contain a prior-GPU journal. No ordinary reader is relaxed.

Execution `f2168fbf-d7c9-4e96-8cc6-784bb1546083` remains explicitly BLOCKED:
its ordinary reader has not been established as valid and it has no original
outbox/ACK. The tool does not add a legacy marker, scope/outbox, log exemption or
independent release-journal bypass for it. A future separately authorized
qualification requires actual ordinary-reader and authenticated original-event
proof, rather than rewriting historical runtime facts.

All54 Gov acceptances must have no retained lease/lease owner, including close
leases. Original operations must be ACKED and lease-free. Every original charge
must match its immutable identity/units and be fully released. Each canonical
`resource-release-v1` receipt's raw payload SHA and event/operation/resource/owner
must match the original record. Accounts must satisfy the all-owner sum
`occupied_units = sum(original_units-released_units)`; account bytes must remain
unchanged after deletion. Additional operation,charge,acceptance and canonical release-payload references block deletion.
Retry-blocked UNKNOWN acceptances receive no automatic exemption.

Gov deletes only its exact acceptance → charge → operation → release-receipt
rows in one transaction. Its successful receipt retains original full charge and
release-receipt evidence plus unchanged before/after accounts. MD requires that
successful original Gov receipt, current absence of those exact Gov rows and a
fresh valid all-owner account invariant before its own transaction can delete:

```text
execution_cleanup_audits,close_intents → execution_runtimes
→ run_authorities,pipeline_confirmed_runs → pipeline_dispatches
→ executions → execution_identities → original input_versions metadata
```

Each table name has the `modeldev_` prefix. Each DELETE binds tenant, every
primary-key field and the complete original current row JSON. Every affected
row must equal1 and each table's total must equal the reviewed count; failures
roll back the complete current database phase. No CASCADE, broad owner filter,
account UPDATE, role/tenant/policy deletion or foreign resource adoption exists.

## Implemented and remaining boundaries

The component validation uses real PostgreSQL/current MD baseline, normal
admission/close code and the actual transaction/row locks. Only external root
Kubernetes/KFP/S3/stopped-source/HTTP proof files are explicit test boundaries.
The old retirement base's full `make verify` has known API-pin/facts/material
baseline failures from its independent retirement validation; this task does
not change those pins or fixtures, rerun an unrelated full suite, or report the
whole old service gate as PASS. Actual targeted logs/receipts accompany delivery.

External resource cleanup, collection/approval of fresh root evidence, the four
UNKNOWN owner-closure proofs, f216 qualification, live DB deletion and final
zero-old-data/deployment/A-B acceptance are **NOT_RUN / outside this tool**.
If new owner-closure tombstones appear for the four UNKNOWN IDs, the current
original scope is refused. They require a newly reviewed complete manifest and
explicit implementation; the tool neither creates nor guesses those rows.

## Component validation receipt

Frozen tool source manifest SHA256:
`d6a92a049ab2ee818d9aa4dfae04c83b47f64f36cece670d55d02e177f54172d`.
Fedora validation ran2026-11-01T00:18:07Z–00:18:30Z with
`GOTOOLCHAIN=auto GOMAXPROCS=2 GOFLAGS=-p=2 GOWORK=off`:

- `go test -race -count=1 -v -timeout=4m ./scripts/legacy-modeldev-data-cleanup`: exit0.
- `go vet ./scripts/legacy-modeldev-data-cleanup`: exit0.
- `go build -trimpath -o <outside-source-tool-path> ./scripts/legacy-modeldev-data-cleanup`: exit0.
- `go mod verify`: exit0.

Raw logs and per-file hashes are retained with the task's final component
receipt (`legacy-data-cleanup-final/receipt.json`, `pg-race.log`, `vet.log`,
`build.log`, `modverify.log`). Source remained unchanged during those commands;
all12 tool files match the retrieved Fedora hashes. This documentation update
adds the observed result after the run.

The positive fixture uses real CPU admission and undispatched close, followed
by both actual DB commits, plus a historical authenticated GPU-owner release
record. It proves exact deletion and account/other-owner/other-tenant retention;
it does not claim a fully materialized training cleanup/live retirement pass.
Negative fixtures prove missing root qualification, an unreleased charge,
unclosed execution, original GPU outbox without ACK, UNKNOWN retry-blocked
without owner closure, new FK, missing receipt row, concurrent owner revision,
third-DELETE cancellation rollback, missing successful prior Gov receipt and
new non-FK acceptance/release-payload references. The three approved retirement
markers remain qualified through the existing maintenance reader; this package
does not duplicate its separate real retirement fixture suite.
