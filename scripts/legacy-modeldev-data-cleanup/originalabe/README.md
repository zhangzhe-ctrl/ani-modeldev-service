# Original ABE compute cleanup

This finite command addresses only `abe39ad8-c017-416b-8041-6f097e8fd50a`.
The original execution is already CLOSED USER_STOP, StepExit completed, its one
original training log is captured, and three automatic cleanup attempts are
exhausted. It neither grants a fourth attempt nor changes a log qualification.

The default `--phase plan` sets PostgreSQL `default_transaction_read_only=on`.
It freezes the complete original admitted/runtime/dispatch/authority source SHA,
revision 614, generation 2, original compute-plan SHA, log count/digest, namespace,
PVC, current original graph, suspension and no-writer evidence. The original
pipeline allows ReadWriteOnce or ReadWriteMany; this command retains the
actual original PVC access mode and full spec SHA, while checking its original
UID, Bound state, frozen storage class/capacity and empty GC owner references.
Observe requires the reviewed PVC spec SHA unchanged. The output lists
only public identity metadata. Six present objects receive parent-first operator
DeleteOptions with both original UID and current resourceVersion preconditions;
the prepared PVC is retained. The CLI never sends DELETE, PATCH or log reads.

After root review and separately performed operator deletion, `--phase observe`
requires the exact reviewed plan SHA and explicit `--authorize-observe`. It first
re-reads the unchanged original source, checks original namespace/PVC, rejects
any present/replacement/unknown target or current PVC writer, then assembles the
original ManagedSteps, ManagedRuntime and ExecutionCloser with real PostgreSQL
repositories. The exhausted+LogsReady path calls the original
ObserveComputeCleanupAbsence and RecordComputeCleanupResult. All seven original
objects must return actual NotFound. Attempts remain 3, logs and original close
remain unchanged. The original repository serializes the identity row and checks
authority, generation, attempt and complete receipt; it is not an extra atomic
caller revision CAS. Source SHA is checked before that original CAS and every
worker/publisher must remain stopped. An unknown result requires a read-only
state inspection, never an automatic retry or hand-written APPLIED.

Only a same-original-SA normal Pod projection may supply the API token/CA. The
config explicitly names ordinary private files and retains original URL, CA and
SNI; a private dial target may bind the original API Service UID/VIP/443 after a
fresh full-cluster identity proof. No ambient kubeconfig, new SA/RBAC, manually
issued TokenRequest, KFP transport, listener or worker is installed. Every API
request is restricted to GET; raw log endpoints are forbidden. Temporary Pod,
private config/token/binary must be deleted after completion.

```
go build -o originalabe ./scripts/legacy-modeldev-data-cleanup/originalabe/cmd/originalabe
./originalabe --config /private/config.json --config-sha256 SHA --out /private/plan.json
./originalabe --config /private/config.json --config-sha256 SHA \
  --phase observe --plan /private/plan.json --plan-sha256 SHA \
  --authorize-observe --out /private/observe.json
```

Builds/tests run on Fedora. `test_owner_path.py` uses a Go test overlay to add the
owned test source to the existing real-PG runtimeproof fixture package without
copying its setup or changing production/test files outside this directory.
Only the named new behavior tests run. The existing fixture creates admission,
dispatch, authority, workspace, real training/close/log receipts through actual
business adapters; Kubernetes/KFP are explicit external boundary substitutes.
The tested maintenance path is the real Closer and real PostgreSQL CAS, and its
post-setup Kubernetes calls must all be GET with zero KFP traffic and zero DELETE.
