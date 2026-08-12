# ADR-0002: Append-only audit and correction semantics

- Status: Accepted
- Scope: v1

## Context

The PRD calls `action` the audit spine, but provisioning, entity merge/split, finding revisions,
exports, and teardown authorization also affect the defensible record. Captures sometimes contain
incorrect operator-entered metadata, while deleting or silently editing history would defeat the
central audit promise. The running instance is disposable only after its durable record is exported.

## Decision

- `action` remains the immutable command-capture spine. Add an append-only `audit_event` stream for
  every meaningful state mutation: actor/engagement provisioning, credential rotation or revocation;
  capture receipt or conflict; entity merge/split; finding promotion/revision/status change;
  out-of-band flag/resolution; export state/receipt verification or invalidation; and teardown
  authorization/consumption.
- Every action has exactly one engagement-scoped human or AI actor. Every audit event records that
  initiating actor, engagement, server timestamp, event type, subject, correlation/request ID, and
  redacted before/after metadata. Service-derived work inherits the actor and source action that
  caused it and records the service as origin; it does not become an anonymous actor. Initial
  bootstrap atomically attributes its event to the first provisioned human.
- Captured command metadata, raw evidence references, structured-result provenance, and audit events
  cannot be updated or deleted by the normal API or runtime database role. Database constraints and
  privileges enforce this in addition to application checks.
- Corrections create explicit superseding records/events linked to what they correct. Entity identity
  is reconstructed from immutable observations and merge lineage; splitting reassigns observations
  without erasing their origin. Findings remain editable drafts, but every accepted version is
  retained with actor/time and optimistic-version metadata.
- Audit events are inserted in the same database transaction as the state change they describe. A
  state change without its event, or an event claiming an uncommitted change, must not commit.
- Sensitive raw evidence is not silently rewritten or redacted. Logs, previews, and audit metadata
  minimize disclosure; deployment controls and export warnings address the residual sensitivity.
- Normal APIs provide no action, audit-event, or committed-evidence deletion. Guarded whole-instance
  teardown is a lifecycle operation after verified export, not selective history mutation.

## Consequences

The Journey log can be a complete ordered projection of immutable audit events while still allowing
operators to correct mistakes visibly. Storage grows monotonically during an engagement, and reads
must reconstruct current state from revisions/lineage rather than destructive edits. Runtime DB
credentials must not own or bypass append-only protections; a host/database administrator remains an
explicit trust boundary.

Automatic jobs need a causal actor/source. A proposed job with no attributable cause must be
redesigned or explicitly approved in a superseding ADR rather than assigned an anonymous “system”
identity.

## Verification

- Attempt update/delete through both the API and runtime DB role for action, audit, and committed
  evidence references; each attempt fails and the conflict attempt is auditable where applicable.
- Concurrently mutate editable records and prove one state revision and its event commit atomically.
- Reconstruct finding and entity history after correction, merge, split, and reversal.
- Compare Journey log entries to persisted event IDs and actor/source details without gaps.

## Traceability

PRD: guiding principle “Auditable end to end,” core data model, AI-actor capture, reporting/auditing,
and teardown verification. Matrix: PRD-CORE-002, PRD-AUD-002–004, PRD-DATA-005/006,
PRD-FIND-002, PRD-UX-003. Resolved gaps: TR-D01, TR-D06, TR-D16, TR-D17. Tasks: V1-006–V1-010,
V1-018, V1-026, V1-030.
