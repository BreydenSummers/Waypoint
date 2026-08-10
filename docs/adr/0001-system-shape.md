# ADR-0001: System shape and repository boundaries

- Status: Accepted
- Scope: v1

## Context

Waypoint must be disposable, fast under concurrent use, straightforward to deploy, and split across
a core repository and a parser/collector repository. The PRD locks PostgreSQL to one instance and
one container, but that wording does not require the application and database to share a container.
Raw evidence needs filesystem storage without introducing an object-store service. REST, SSE, MCP,
collectors, parsers, and report rendering must not become independent mutation paths.

## Decision

- The core repository owns a Go application/API service and a React + TypeScript web client. The web
  client is built to static assets served by the application image.
- Default Compose topology is one versioned application container plus exactly one PostgreSQL
  container. PostgreSQL is the only database. A mounted content-addressed evidence volume is part of
  the logical persistence layer, not another database or network service.
- PostgreSQL is authoritative for engagement state, metadata, authorization, provenance, and audit
  ordering. All writes pass through the core application service and its authorization/audit
  transaction boundary.
- REST is the write protocol. SSE is an authorized, resumable read feed over persisted audit-event
  IDs. MCP exposes only narrow authenticated ingestion/status tools and delegates to the same core
  services as REST; it is not a second implementation of writes.
- The collection repository owns Go operator-wrapper and remote-agent binaries, the plugin SDK,
  release-pinned parser artifacts, and parser fixtures. Core owns versioned ingestion and structured
  result schemas plus compatibility fixtures. Both sides negotiate explicit contract versions.
- Core never executes assessment commands. Only the wrapper or remote agent executes them. Parser
  execution follows [ADR-0004](0004-parser-isolation.md).
- A pinned headless Chromium build in the application image renders a versioned print-only route from
  frozen report input. Export semantics follow [ADR-0005](0005-export-semantics.md).

## Consequences

The default deployment has two containers but only one PostgreSQL container and no graph database,
object store, message broker, external renderer, or model service. Go keeps the service and
cross-platform collector operational surfaces small; React/TypeScript supports an accessible SVG
shell and data-heavy workspaces. Cross-repository integration must be contract- and fixture-based,
so neither repository may rely on unpublished implementation details in the other.

A filesystem evidence volume is intentionally coupled to PostgreSQL references and therefore needs
the atomic placement and reconciliation rules in [ADR-0003](0003-content-addressed-evidence.md).
Scaling by adding independent writers or bypassing the application transaction would violate this
record.

## Verification

- Inspect default Compose topology: one app and one PostgreSQL container, with no additional data
  service.
- Run shared contract fixtures against REST and MCP and prove equivalent authorization,
  idempotency, validation, and audit output.
- Prove SSE reconnect uses persisted IDs and that slow readers cannot block ingest.
- Inventory the core image and routes to prove there is no command-execution endpoint.

## Traceability

PRD: locked decisions “Data store,” “Real-time updates,” and AI first-class ingestion; collection and
reporting sections. Matrix: PRD-DATA-001, PRD-CAP-007, PRD-CAP-008, PRD-RT-001, PRD-DEP-001.
Resolved gap: TR-D11. Tasks: V1-002–V1-005, V1-010–V1-012, V1-021, V1-028.
