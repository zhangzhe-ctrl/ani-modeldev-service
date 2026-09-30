# Service repository guidance

This service owns its generated runtime source. It has no runtime dependency on
the layout that created it.

## Boundaries

- `cmd/<service>` is the explicit composition root.
- `internal/service` adapts inbound transport contracts to use cases.
- `internal/biz` owns domain concepts, behavior, use cases, and required ports;
  it must not import Kratos, protobuf, transports, or storage drivers.
- `internal/data` implements outbound ports and owns infrastructure-specific
  code. Add an adapter only when a vertical slice needs it.
- `internal/server` owns transport and cross-cutting runtime wiring, not
  business rules.

Prefer framework capabilities over local replacements. Do not introduce a
shared ANI runtime module, generalized YAML exception registry, or technology
switches unrelated to a real slice. Generate protobuf output through the pinned
workflow and run `make verify` before committing.
