# ani-modeldev-service

Module: `github.com/zhangzhe-ctrl/ani-modeldev-service`

This repository was generated from ANI's pinned Kratos layout. It is an
independent source snapshot: builds and runtime do not require the layout.

`THIRD_PARTY_NOTICES.go-kratos-layout.txt` preserves the upstream template's
MIT notice. This generated repository intentionally has no project `LICENSE`;
its owner must make that choice before publication.

## Runtime and verification commands

For CPU-P01, execute these commands only in the task's fixed Fedora checkout.
Starting the runtime shell there is module verification, not target-cluster
business acceptance.

The minimum Go toolchain is 1.26.7, matching the verified Fedora release line.
CI reads this requirement from `go.mod`. The prior Go 1.25.7 toolchain failed the
standard-library vulnerability gate; use the declared minimum or a newer
supported toolchain and retain the vulnerability scan. See the
[official Go release history](https://go.dev/doc/devel/release).

```bash
make tools
python3 scripts/with-test-postgres.py make verify
go run ./cmd/ani-modeldev-service -conf ./configs
```

The verification wrapper creates one digest-pinned PostgreSQL test container
with loopback access, CPU/memory limits, temporary data, and a restricted runtime
role, then removes only that invocation's container. It requires Docker and
Python 3. To use an already prepared task-owned test database, provide protected
`CPU_P01_TEST_DATABASE_URL` and `CPU_P01_TEST_DATABASE_ADMIN_URL` environment
references and run `make verify` directly. Missing database conditions fail the
persistence tests; they are never silently skipped. These are module checks.

`make verify` also runs the explicit `revisionupgrade` lane. It builds a private
old-writer test binary from retained commit
`a827457f999646394518c640d7c21915e1321496`, seeds an isolated pre-0012 schema through
the real old repositories, and verifies the migration with the current writers.
Keep full Git history available, as CI does; missing historical source, build,
binary or database conditions fail this lane rather than skip it. The script
records the source and binary SHA256 and removes only its temporary export.
No developer scratch files or prebuilt binary are required. Plain `go test ./...`
excludes this tagged lane and does not establish upgrade verification.

After the initial source commit, run `make supply-chain-tools` and `make audit`;
review and commit `docs/scaffold/bom.cdx.json`. CI deliberately fails when that
runtime SBOM is missing or stale and reruns the vulnerability, secret,
notice-integrity, and license-evidence gates.

The committed listeners are loopback-only local defaults. Override them through
the typed `ANI` environment configuration when the deployment design is added.

## Runtime shell

- Kratos lifecycle with graceful shutdown.
- gRPC and a separate admin HTTP server.
- Structured redacted logs, tracing, metrics, and middleware.
- `/healthz` reports process liveness.
- `/readyz` remains HTTP 503 while the required business adapters are absent;
  starting listeners does not establish business readiness. Real dependency
  checks belong to the production composition when those adapters are added.
- `/metrics` exports the local Prometheus registry.

CPU-P01 domain, persistence and adapter slices are present. An explicit typed
`command` configuration assembles the Governance mTLS `ApplyCloseIntent` handler
and real PostgreSQL repository using mounted connection/certificate references.
Missing materials fail startup; omitting the block keeps the unready shell.
See [command delivery](docs/design/cpu-p01-command-delivery.md) for the trust,
receipt and configuration boundaries. An optional `command.admission_resolution`
block adds read-only snapshot resolution on that same mTLS listener using pinned
mounted facts, the immutable Release catalogue and the existing runtime pool.
See [admission configuration](docs/design/cpu-p01-admission-resolution.md#启动配置)
for material and readiness boundaries. AcceptExecution, Query, Step and the full
training/publication chain are not assembled; unavailable RPCs fail explicitly.
