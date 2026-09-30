# Runtime verification

The generated service keeps executable runtime checks beside the source. From a
clean checkout, run:

```bash
make tools
python3 scripts/with-test-postgres.py make verify
```

`make verify` checks that typed configuration, public protobuf, sqlc queries,
and shared contract vectors regenerate without a content change, rejects stale
formatting or module metadata, and runs `go test`,
`go vet`, `go build`, and `go mod verify`.

The runtime integration tests start real loopback gRPC and admin HTTP listeners.
They check gRPC health, disabled reflection, health/readiness/metrics responses,
Kratos error encoding, middleware recovery/metadata/validation, request metrics,
trace-correlated structured logging, and graceful application stop. These tests
prove only the generic local runtime. They do not prove a database, broker,
provider, external telemetry backend, container, Kubernetes deployment, or
business contract.

The execution repository tests separately require real PostgreSQL. The wrapper
starts a digest-pinned, resource-limited container, keeps its credentials in a
private temporary location, and passes separate migration/runtime references to
the test process. The runtime role is NOSUPERUSER/NOBYPASSRLS; each test applies
the versioned migration in its own schema and grants only required operations.
Afterward the wrapper checks the ownership label and removes only its exact test
container. Fedora may instead use the already prepared task-exclusive database
by supplying both protected `CPU_P01_TEST_DATABASE_*_URL` references and invoking
`make verify`. Missing dependencies fail with behavior NOT_RUN, not a skipped
security check. These real database results do not prove the target cluster.

For CPU-P01, run these commands on Fedora against a fixed source commit. The
workstation is used only to edit, review, and transfer source and results. The
production composition currently has no persistent execution repository or real
business adapters. Its process health may return HTTP 200 and gRPC health may
report SERVING, but `/readyz` must remain HTTP 503 until the required business
dependencies are actually wired and verified. A generated business RPC contract
does not make that RPC available: unimplemented operations must fail explicitly.
The production-composition test observes this boundary through real HTTP and
gRPC listeners while retaining logging and graceful-stop checks. Dependency
readiness belongs with the real application composition added in CPU04; there is
no configuration switch that declares absent business capabilities ready.

For vulnerability and secret scans, a reproducible CycloneDX JSON runtime SBOM,
and notice/license-inventory consistency checks (with `jq` available):

```bash
make supply-chain-tools
make audit
```

Vulnerability results are valid only for the scanner and database state at the
time of execution. The pinned Gitleaks module scans all local Git refs with its
embedded detector set and redacts any finding in command output; CI requests a
full-history checkout before running it. The SBOM
generator requires a committed revision, ignores only its own output while
checking cleanliness, and builds an isolated synthetic Git snapshot from all
tracked files except the prior SBOM. This prevents both a self-reference and
accidental discovery of a parent Git repository through `TMPDIR`.

The committed SBOM targets the Linux/amd64 runtime graph and excludes test-only
dependencies. Byte reproducibility is claimed only for the same source snapshot
and recorded CycloneDX binary; its own binary hashes intentionally make output
from a separately built scanner distinguishable.

For a newly generated repository, first commit the generated source, then run
`make supply-chain-tools` and `make audit`, review the result, and commit
`docs/scaffold/bom.cdx.json`. The CI gate rejects a missing or stale SBOM and
reruns the scanner and notice/license-evidence checks. The retained upstream
notice is not a project license; choose the service's license before publication.
