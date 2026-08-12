# ADR-0008: Standard MCP transport and service parity

- Status: Accepted
- Scope: v1

## Context

The PRD requires an official API plus MCP ingestion path so AI actors can use the same attributable
capture path as collectors. Custom HTTP routes whose names contain `mcp` are not an MCP server and
cannot be discovered by standard clients. A second MCP-specific capture implementation would also
risk different authorization, idempotency, validation, or audit behavior.

## Decision

- Core exposes one authenticated MCP Streamable HTTP endpoint at `/api/v1/mcp`. It implements MCP
  JSON-RPC 2.0 initialization, `notifications/initialized`, `tools/list`, and `tools/call` using the
  pinned protocol revision in the contract. It does not expose custom REST-shaped `/mcp/*` aliases.
- The v1 tool inventory is closed to `waypoint_ingest_capture` and `waypoint_capture_status`.
  `waypoint_ingest_capture` accepts the same capture envelope, capture ID/idempotency key, and
  distinct stdout/stderr content as REST. Accepted calls delegate to the exact capture application
  service and return the durable capture acknowledgement. `waypoint_capture_status` performs an
  actor/source/capture-scoped status lookup without revealing foreign-scope existence.
- HTTP bearer authentication supplies actor and engagement. MCP session IDs are transport state, not
  credentials or authorization scope. Revocation and role checks are identical to REST. The server
  advertises only the MCP `tools` capability; there is no generic shell, filesystem, SQL, prompt,
  resource, sampling, or command-execution capability.
- JSON-RPC protocol/dispatch failures use standard MCP/JSON-RPC errors. Application validation,
  idempotency, integrity, quota, and authorization failures are returned as a tool error containing
  the same stable RFC 9457 problem object used by REST. A successful tool call produces the same
  action, evidence references, and append-only event as REST; it records event origin `mcp`.
- MCP's JSON tool transport carries evidence as base64 and may have a lower configured size ceiling
  than streaming REST. Exceeding it returns the shared non-retryable quota problem and preserves the
  caller's spool; it never truncates or changes accepted semantics.

## Consequences

Standard MCP clients can initialize and discover Waypoint without bespoke route knowledge. Protocol
conformance and application parity are independently testable. Base64 is intentionally a narrow
agent integration path; large collector output remains on streaming REST.

MCP is audit infrastructure for externally operated AI actors. It does not add a Waypoint offensive
model, AI guidance, a guided command catalog, or a server-side command runner.

## Verification

- Run an MCP protocol conformance client through initialize, initialized notification, tools/list,
  and tools/call over Streamable HTTP.
- Replay equivalent REST and MCP captures and compare authorization decisions, fingerprint scope,
  durable acknowledgement, action projection, evidence hashes, and audit data except transport
  origin.
- Test revoked/foreign/viewer credentials, changed duplicate IDs, malformed JSON-RPC, unknown tools,
  oversized evidence, and status existence-oracle resistance.
- Inventory the MCP tool list and core routes to prove no shell or deferred capability is exposed.

## Traceability

PRD: AI-actor capture “First-class ingestion endpoint” and documented residual boundary. Matrix:
PRD-CAP-008, PRD-AUD-002/003, PRD-ID-002, PRD-CAP-010. ADR links:
[ADR-0001](0001-system-shape.md), [ADR-0002](0002-append-only-audit.md), and
[ADR-0007](0007-attribution-lifecycle-and-read-model.md). Tasks: V1-002, V1-010, V1-012,
V1-019, V1-033.
