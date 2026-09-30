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

CPU-P01 domain and RPC contracts are present. Production business handlers,
persistent repositories, external adapters, and deployment bindings are not yet
assembled; unimplemented business RPCs fail explicitly.
