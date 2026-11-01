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

The current fixed DB scope is **54 MD aggregates**, the original50 plus the four
original Governance commands55/57/59/60. Those exact original commands were
stopped through the ordinary BFF, durably fenced, delivered once using the
corrected original21bf codec and closed by the original native NoDispatch path.
They retain their original operation/spec/intent bytes, actor and accepted_at.
Governance deletion remains exactly54 acceptances,23 operations,23 charges and
23 original receipts. No user/tenant/role/policy table is added to the deletion
or compared as a whole-table historical row-count baseline.

This explicit maintenance revision emits
`ani.modeldev.legacy-db-cleanup.plan.v2`. Old v1 plans and prior receipts are
refused, even when their own hash is internally consistent. A fresh complete
manifest/hash and actual54-row snapshot are required; an old50/zero-MD-row
qualification cannot authorize the four newly durable records.

Only `27cf73a3-4107-4c07-86f7-862ce9fb413d`,
`c9b470e0-d9cb-4989-a7f2-d1fbfbe72ab9`,
`91e351e7-102e-4807-b0d2-9236c1426da9` and
`f148135c-640f-47ed-b9b1-4b039d4709d1` are included by this correction. Their
normal native canonical admission/runtime reader must pass. Their original
CLOSED/USER_STOP/generation1/NoDispatch facts, absent dispatch/confirmed
Run/authority and absent workspace/training/publication/runtime creation are
required. StepExitCleanup, ComputeCleanup and GPURelease remain NULL; this tool
never writes APPLIED or creates an outbox/journal for them. Governance requires
their original canonical admission, matching original durable Create and Stop
receipts, ACKED states, no retries/leases and no quota operation/charge for these
four CPU commands. Every other UNKNOWN identity remains refused. Missing or
changed original actor/time/canonical bytes, missing ACK or a newly materialized
resource blocks this finite qualification.

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

### Seven original c99 aggregates

The fixed original inventory contains seven CPU executions whose original
`ani.modeldev.execution-spec.v3` snapshot bytes predate `training_image`.
Official c99 accepts their full original admission envelopes; official21bf
rejects the same bytes first at `snapshot training image`. This affects both
normal historical queries and the current cleanup admission/compute predicates.
Specification13.1/13.2/13.3.8/13.4 require retiring those original aggregates,
while13.5 and the v4 contract prohibit a runtime v3 compatibility reader. The
small correction is an independent, read-only native c99 qualification tool,
consumed only by this finite database deletion command. Neither service runtime,
`Snapshot.Validate`, canonical bytes, close facts nor quota records change.

`scripts/legacy-modeldev-data-cleanup/originalc99` is a separate Go module pinned
to the official commit `c99c9f1081c754f7e38ceccd056db9a032ca3c27`, without a
replace or copied reader. Its scope is exactly:

```text
111952ee-9d72-46d9-9981-c2a1ead73289
4534a7de-480c-4a8f-ae76-e647cf7c158c
5953c2e1-107e-4d46-af6d-4c718c7fb10c
5f0b866e-31d1-4b44-8a7c-5cc900767e00
c0fa3739-d279-4bb3-85be-0f74b8d4773a
e94344af-a825-4ba2-9fa1-838638cc4d5f
ec57bafc-56bd-4f7e-8f65-45527aa4d0e3
```

It has no write flag. On Fedora, build from that nested module with the pinned
Go1.26.10 toolchain. Retain the actual build receipt, source-file hash manifest
and binary SHA. After the approved sole-entry full-identity verification, run:

```sh
original-c99-qualifier -dsn-file /private/md-owner-dsn \
  > /private/original-c99-qualification.json
```

The command begins a PostgreSQL READ ONLY, repeatable-read transaction. Each
original PK/operation, canonical intent/snapshot byte SHA and actor/accepted-at
SHA is fixed in source. Native c99 `Admission.CanonicalPayloads` and
`lifecycle.ReadInTransaction` must pass; original CLOSED, close evidence, owner
authority and actual step/compute cleanup are required as applicable. GPU
outbox/ACK checks remain in the native qualifier as well as the existing main
cleanup tool. All seven fixed originals are CPU; this creates no GPU exception.
Foreign/new runtime JSON fields, including retirement markers, are refused.
Output contains finite guard names, original IDs and SHA values, never raw
canonical/runtime payloads. A blocked record has no qualification predicate;
process exit0 alone is not qualification.

The successful native predicate is
`ORIGINAL_C99_NATIVE_CLOSED_CLEANUP_ACK`. The output binds the actual executing
binary SHA, database/schema/role/server/OID, database observation time and a
five-minute expiry, and sorted full-row SHA lists for all eight aggregate
tables, including empty child sets. The original input-version metadata still
passes the main command's unchanged inventory/reference checks.

The root executor must inspect that output and the actual build receipt, then
include their exact file byte hashes in the independently approved root receipt:

```json
{
  "OriginalC99Qualification": {
    "Kind": "ORIGINAL_C99_NATIVE_QUALIFICATION",
    "Path": "/private/original-c99-qualification.json",
    "SHA256": "<actual-reviewed-file-byte-SHA256>"
  },
  "OriginalC99Build": {
    "Kind": "ORIGINAL_C99_BUILD_RECEIPT",
    "Path": "/private/original-c99-build-receipt.json",
    "SHA256": "<actual-reviewed-file-byte-SHA256>"
  }
}
```

The build receipt schema is
`ani.modeldev.original-c99-qualifier-build.v1`; fields `CodecCommit`,
`BinarySHA256`, `SourceSHA256`, `GoVersion:"go1.26.10"`, `ExitCode:0` must
describe the actual successful build, not a hand-written completion boolean.
These external source proofs use the same explicit root trust boundary as the
other qualified evidence files. Root receipt expiry must be no later than the
native proof expiry, so the existing final commit-time expiry check covers both.

Absent proofs keep the read-only deletion plan BLOCKED. Wrong identity, codec,
binary/build hash, database, source bytes, scope, expiry or any aggregate row
change refuses qualification. Apply recomputes the exact eight-table row SHAs
after its existing writer-draining locks. The seven cannot use the original3
retirement marker path, and all other executions still use their original
normal/approved-retirement reader. Global FK/trigger/reference checks, original
Gov ACK/charge/account checks, external cleanup proofs, exact DELETE counts and
the two-phase PARTIAL/resume boundary remain mandatory.

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

External resource cleanup, collection/approval of fresh root evidence,
f216 qualification where still required by the current native reader/outbox,
live DB deletion and final
zero-old-data/deployment/A-B acceptance are **NOT_RUN / outside this tool**.
The four original command correction above reads actual ordinary aggregates;
the tool neither creates nor guesses closure rows. No identity outside the
fixed54 can be silently adopted; it requires another explicit scope decision.

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

### Original c99 qualification component receipt

The18-file cleanup-tool source manifest is
`1f103568fc0b1546962a73b1eb32adaa759161fc5a587158badb7d3b196fccbb`;
all18 local/Fedora file hashes match. Fedora Go1.26.10 validation completed with
exit0 for native exact-seven real-PG qualification, current consumer real-PG/race
qualification and row-lock drift, the existing no-ACK/unreleased/unclosed
refusals, both tools' vet/build and both modules' `go mod verify`. The native
test also changes each original canonical/actor/accepted-at identity field and
removes original CloseEvidence in its isolated fixture; all are refused. The
first native test attempt was infrastructure **NOT_RUN** because the unmodified
c99 test helper found this nested module rather than its migrations. The test
now explicitly uses the outer checkout baseline after checking its byte SHA
equals the official c99 baseline; no business reader was changed.

The native qualifier binary SHA is
`a1891a0e388e8fe4a6280e4803c9b2d068a8317908739dabeb2e0036689eb78d`.
The updated deletion binary SHA is
`ac69b711dac92e168988d65959aa87fa93204b004e21abbce2aae014aef0503c`;
it links the original651f5ea retirement code, including TrainingExitKind and
OriginalStopFence, plus this finite qualifier consumer. Original raw fixtures
were mode0600, remained only on Fedora and were deleted after the necessary
tests; only IDs, hashes, finite guards and original gate receipts are retained.
Evidence is in `modeldev-original-c99-qualification/` (`native-pg-final.log`,
`consumer-pg.log`, `qualification-component-receipt.json`,
`build-final-receipt.json`, `native-build-receipt.json`, source manifests and
`private-fixture-deletion-receipt.json`). This is component validation; fresh
live native qualification, approved external proof and database apply remain
**NOT_RUN**. The known old-base full-service gate was not rerun or relabeled.
Negative fixtures prove missing root qualification, an unreleased charge,
unclosed execution, original GPU outbox without ACK, UNKNOWN retry-blocked
without owner closure, new FK, missing receipt row, concurrent owner revision,
third-DELETE cancellation rollback, missing successful prior Gov receipt and
new non-FK acceptance/release-payload references. The three approved retirement
markers remain qualified through the existing maintenance reader; this package
does not duplicate its separate real retirement fixture suite.

### Fixed54 component validation

The final20-file tool source manifest is `f11acffd336dcae46c2f2c4c2b9047595829a03d9e141cfd875c59cb8e5f043c`; all local/Fedora hashes match.
Fedora Go1.26.10 ran the affected isolated tool package against real PostgreSQL
with `-race -count=1`:13 top-level tests PASS and the unchanged original-seven
private-fixture test SKIP because those temporary raw bytes were already deleted
after its prior PASS. The native actual-four admission/runtime/NoDispatch and
Governance Create/Stop receipts pass; changed identity/canonical/actor/time/ACK
and unclosed originals refuse. The fixed50+4 scope and old-v1-plan/prior-receipt
refusal pass, as do both actual DB commits with other-owner/account retention,
original missing-GPU-ACK, unknown-ID, reference drift, concurrency and rollback
checks. `go vet` and `go build -trimpath` also exit0.

The new fixed54 binary SHA256 is
`ea9779597c06b2760eea2328ce86df0d8f718ff0d860df6d24a8c2b84b63a389`.
It supersedes the earlier fixed50 deletion binary for this maintenance. The
native c99 qualifier binary remains unchanged. Raw receipts are
`fixed54-final-receipt.json`, `fixed54-final-pg-race.log`,
`fixed54-final-vet.log`, `fixed54-final-build.log` and
`fixed54-tool-source.sha256` in task09 `legacy-data-cleanup/`. No live DELETE
was executed by these checks.
