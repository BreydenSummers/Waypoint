# Waypoint API and event contract v1.0.0

This directory is the G0 integration boundary between core and collectors. The canonical API is
[`openapi.json`](openapi.json); a generated aggregate and schema catalog live in `generated/`. JSON Schemas
use draft 2020-12 and are closed by default. Fixtures are synthetic and normative.

## Contract and compatibility policy

- `/api/v1` fixes the breaking-change major. Every request also sends
  `Waypoint-Contract-Version: 1.0.0`; capture envelopes carry the same exact version so a disk spool
  remains self-describing. A mismatch or unsupported exact version returns HTTP 426 with
  `unsupported_contract_version` and `supportedVersions`.
- Patch releases may correct prose, add invalid fixtures, or tighten an implementation bug only when
  the accepted instance set is unchanged. A minor version may add optional fields or event types.
  During the compatibility window the server must honor an explicitly requested older minor and
  return that version's projection; it must not silently send a newer schema.
- Removing or renaming a field, changing meaning, making an optional field required, narrowing an
  accepted value/range, changing idempotency scope, or reusing an event/error code is breaking and
  requires `/api/v2`.
- Objects reject unknown fields. Consumers must still ignore unknown **event types** after durably
  advancing their cursor: the event envelope is stable and event `data` is type-specific. Consumers
  must not treat an unfamiliar event as a stream failure.
- Plugin result `schemaId` and `schemaVersion` are a second negotiation boundary. `parsed` is accepted
  only for a release-registered plugin artifact and result schema; generic envelope validity alone
  never makes parser output trusted.

## Capture transport and trust rules

`POST /captures` is multipart with JSON `envelope` plus mandatory `stdout` and `stderr` binary parts.
Both parts are present even when empty and become distinct logical evidence records even if their
hashes match. Before acknowledging, core streams and verifies each part's exact SHA-256 and byte
length, durably places it, and atomically commits the action, evidence references, and one
`capture.accepted` event. Parser absence or failure never rejects otherwise valid raw capture.

The bearer credential exclusively supplies engagement and actor scope. The envelope intentionally
has no actor or engagement field. In particular:

- a human credential accepts `initiatedBy=manual`;
- an AI credential accepts only `initiatedBy=ai`, requires decision context, and resolves to a
  registered AI actor with name/model/version and a human `authorizedBy` actor;
- `scan-library` is reserved/unavailable and retained only as a compatibility enum; v1 returns `invalid_request` with field
  code `reserved_value` if a caller sends it;
- body IDs cannot override credential scope, and tokens never appear in URLs, schemas, events, logs,
  or error details.

`startedAt` and `endedAt` preserve collector wall-clock while `durationMs` is monotonic elapsed time.
Clock disagreement and receipt skew are surfaced in the acknowledgement/report, not rewritten or
used to discard a capture. `execHost.address` is collector-selected with method/confidence and is
never replaced by the HTTP peer. Egress status is always explicit; its address is absent only for
`disabled` or `resolution_failed`. Pivot hops are ordered from the executing host toward the target
and may not include credentials.

Metadata is limited to 1 MiB encoded JSON; stdout and stderr each have an implementation-configured
quota no larger than the schema ceiling. Servers may configure lower limits and report them in
operator configuration. Assessment strings are untrusted text, never HTML, paths, or commands for
core to execute.

## Idempotency rules

The collector creates and persists `captureId` before command execution. `Idempotency-Key` must be
the same canonical UUID. The durable uniqueness scope is:

```
(credential-derived engagement_id, actor_id, sourceAgent.id, captureId)
```

After evidence integrity and schema/semantic validation, core fingerprints the normalized envelope
plus the verified stdout/stderr role, length, media type, and digest. Multipart boundaries, JSON key
order, request IDs, and authorization headers are excluded.

- First accepted request: HTTP 201, `idempotency=created`, one action and one accepted event.
- Same scope and fingerprint: HTTP 200 with the original IDs/timestamps,
  `idempotency=replayed`, and no new accepted event.
- Same scope/key with any changed normalized payload or evidence: HTTP 409
  `idempotency_conflict`; retain the original action and append one redacted `capture.conflict`
  audit event attributed to the caller.
- A different actor or source agent is an independent scope and reveals nothing about another
  scope. Concurrent equal requests elect one creator; peers wait for its durable outcome and replay.
- Validation/integrity failures do not reserve the key. Collectors retain spool data until a durable
  created/replayed acknowledgement and retry transport ambiguity with the same key.

Normative scenarios are in [`fixtures/idempotency.json`](fixtures/idempotency.json).

## Cursor, pagination, and SSE rules

Audit IDs are positive PostgreSQL `bigint` values serialized as canonical decimal **strings** to
avoid JavaScript precision loss. They are engagement-filtered, strictly increasing, persistent, and
never reused. The maximum is `9223372036854775807`.

- REST history is ascending keyset pagination. `after` is exclusive, `limit` is 1–500, and
  `nextCursor` is the last returned ID. `highWaterCursor` is the transactionally observed stream
  high-water mark (null only for an empty engagement). No offset or unbounded list exists.
- SSE `id` equals `AuditEvent.id`, `event` equals `AuditEvent.type`, and `data` contains the complete
  event JSON. `after` and `Last-Event-ID` are exclusive. If both occur they must match or return
  `cursor_mismatch`.
- With no SSE cursor, the connection snapshots its high-water mark and starts live **after** it;
  clients needing history first page REST, retain `highWaterCursor`, then connect using that cursor.
- Heartbeat comments do not advance the cursor. A bounded slow reader is disconnected; ingestion is
  never blocked. Reconnect uses the last fully processed ID.
- The append-only audit remains in REST history even if the bounded SSE delivery window advances.
  A resume value older than the smallest accepted SSE cursor returns HTTP 410 `cursor_expired` with
  that cursor and a REST `resync` link. Resync history, then reconnect from the returned high-water.

Normative edge cases are in [`fixtures/cursors.json`](fixtures/cursors.json).

## Error model and retry behavior

Errors use `application/problem+json` and [`problem.schema.json`](schemas/problem.schema.json), based
on RFC 9457. `code` is the stable branch key; title/detail/field messages are safe operator guidance.
`fieldErrors.pointer` is a JSON Pointer into the envelope, never an echo of supplied data. HTTP status
and body `status` agree. Every response carries `X-Request-ID`; details never contain tokens, raw
output, parser output, or foreign-scope existence.

| HTTP | Code | Automatic retry |
|---:|---|---|
| 400 | `invalid_request`, `cursor_invalid`, `cursor_mismatch` | No; correct request/spool metadata. |
| 401 | `unauthenticated` | No; provision/rotate the actor credential. |
| 403 | `forbidden` | No; correct engagement role/scope. |
| 409 | `idempotency_conflict` | No; investigate changed duplicate; never overwrite. |
| 410 | `cursor_expired` | REST resync, then reconnect. |
| 413 | `payload_too_large` | No automatic retry; preserve spool and escalate quota. |
| 422 | `evidence_integrity_mismatch` | Re-read local spool; retry only after local verification. |
| 426 | `unsupported_contract_version` | No; install a supported client. |
| 429 | `rate_limited` | Yes, using `Retry-After` with jitter. |
| 500/503 | `internal_error`, `service_unavailable` | Yes with bounded exponential backoff and same idempotency key. |

Examples, including a rejected secret-bearing extension, are in
[`fixtures/problems.json`](fixtures/problems.json).

## Event registry

Every meaningful mutation is one same-transaction append-only event with engagement, initiating
actor, server time, origin, subject, request/correlation IDs, and redacted data. Service-derived work
inherits the causal actor/action and uses `origin.kind=service`; it never invents an anonymous actor.
Initial bootstrap is attributed atomically to the first human.

The v1 registry reserves these meanings without requiring all implementation routes at G0:

- `engagement.provisioned`, `engagement.status-changed`;
- `actor.provisioned`, `actor.revoked`;
- `capture.accepted`, `capture.conflict`, `structured-result.appended`;
- `entity.merged`, `entity.split`;
- `finding.promoted`, `finding.revised`, `finding.status-changed`;
- `out-of-band.flagged`, `out-of-band.resolved`, `export.state-changed`, `teardown.authorized`.

`capture.accepted`, `capture.conflict`, `out-of-band.flagged`, and `out-of-band.resolved` have typed
data in the current schema. Other registered payloads remain redacted objects until their owning task
publishes a typed additive contract; the envelope and registry meaning cannot be repurposed. If a
claim cannot be tied back to a captured source action, Waypoint flags it for review instead of
pretending provenance exists.

## Fixtures and verification

The mandatory matrix in [`fixtures/captures/index.json`](fixtures/captures/index.json) covers every
human/AI × known/unknown combination plus invalid attribution, parser state, version/idempotency,
reserved-value, and evidence metadata cases. Valid capture fixtures embed the exact expected human
or AI `capture.accepted` event. Event-specific invalid cases are indexed separately.

Run from the repository root:

```sh
python3 scripts/generate-contracts.py --check
python3 scripts/verify-contracts.py
python3 scripts/lint-architecture.py
```

Generation creates deterministic aggregate/catalog artifacts from OpenAPI's schema registry.
Verification resolves their canonical schema IDs from the checked-in local catalog (never the
network), checks every source schema against the draft meta-schema, checks generated bytes and
OpenAPI operation/reference invariants, and runs every positive and negative
fixture, idempotency/cursor semantics, and event/capture consistency. The verifier requires
`jsonschema` (see `contracts/requirements.txt`).

## PRD traceability

| Requirement | Contract evidence |
|---|---|
| PRD-AUD-001, PRD-NET-001/002 | Capture source, target, timing, result, exec/egress/pivot schema and fixtures. |
| PRD-AUD-002/003, PRD-ID-002 | Actor-kind semantic checks, mandatory AI authorizer/context, paired events. |
| PRD-CAP-001/002/003 | Raw multipart streams, parsed/needs-plugin/failure states, plugin/result envelope. |
| PRD-CAP-007/011 | OpenAPI capture operation and actor/source-scoped retry/conflict matrix. |
| PRD-RT-001 | Persisted string cursor, keyset REST, resumable bounded SSE and cursor fixtures. |
| PRD-CORE-002, PRD-DATA-006 | Same-transaction actor event envelope and immutable retry behavior. |
| PRD-PERF-001/002 | Bounded pages/metadata/evidence and streaming multipart contract. |

Architecture: ADR-0001 (single write/read paths), ADR-0002 (append-only events), ADR-0003
(content-addressed streams), and ADR-0004 (untrusted parser output/raw fallback).
