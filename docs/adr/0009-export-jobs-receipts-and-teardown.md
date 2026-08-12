# ADR-0009: Persisted export jobs, receipts, and teardown authorization

- Status: Accepted
- Scope: v1

## Context

Export can run for minutes, survive browser disconnects, encounter capacity or renderer failures,
and overlap live capture. Browser-local progress or a caller-authored receipt cannot prove a
self-contained bundle was produced and verified. Guarded teardown therefore needs server-owned,
audited state tied to exact verified bytes, while the actual deployment destroy operation must be
able to stop and remove the application that authorized it.

## Decision

- Export is a persisted, engagement-scoped job created by an authorized actor. Its monotonic state
  machine is `queued` → `preflighting` → `running` → `verifying` → `completed`, with explicit
  `cancel_requested`, `cancelled`, and `failed` terminal/recovery paths. Progress, stage, byte counts,
  cutoff/snapshot identity, request actor, timestamps, and safe failure code are durable and readable
  after reconnect. Every accepted state transition emits `export.state-changed` in the same database
  transaction.
- Capacity preflight occurs before snapshot/export work. Cancellation is best-effort before final
  verification and never turns partial files into a completed bundle. Retry creates a new job linked
  to the prior one; it does not rewrite job history. Capture remains live outside the consistent
  snapshot described by [ADR-0005](0005-export-semantics.md).
- Completion requires the database dump, referenced evidence, frozen report input, PDF, metadata,
  offline tools/instructions, manifest, and outer archive hash to be generated and independently
  verified. The persisted job links the exact manifest and archive digests, safe bundle locator,
  cutoff, and a server-generated immutable receipt.
- A receipt is created only by successful local verification. It records export/job/engagement,
  manifest digest, outer archive digest, exact bundle locator, cutoff, verifier/tool version,
  verification time, and causal actor. Reads never accept a caller-authored receipt as authority.
  Receipts are invalidated visibly if later re-verification of the referenced bytes fails; history is
  retained.
- The supported destroy command first re-verifies the actual archive and manifest against the
  receipt, then asks core for a short-lived, single-use teardown authorization bound to the receipt,
  bundle locator, both digests, engagement, and actor confirmation. The external deployment
  orchestrator consumes that authorization and removes app, database, and evidence volumes. Core
  records `teardown.authorized`/consumption before shutdown; a browser button alone cannot destroy or
  authorize destruction.
- Force teardown remains an explicit interactive host-admin bypass outside the normal authorization
  API. v1 receipts attest hash verification only. The signature extension remains empty and no
  signer identity is claimed.

## Consequences

The browser becomes a reconnectable client of server-owned export state rather than the authority
for success. Jobs and receipts add durable metadata but make interruption, cancellation, support,
and teardown decisions reconstructable. Because an application cannot remove its own container
reliably, final deletion belongs to the shipped deployment/CLI boundary after core authorization.

A receipt does not make copied bytes immutable, confidential, or attributable to a cryptographic
identity. Teardown must fail closed when the exact current bundle no longer verifies.

## Verification

- Restart the app/browser during every job stage and prove state/progress resumes without duplicate
  completion or loss of causal audit data.
- Run concurrent capture/export, capacity failure, cancellation, renderer failure, disk-full, and
  verification failure; prove only a consistently verified job reaches `completed`.
- Mutate the archive, manifest, payload, locator, receipt, or authorization and prove teardown is
  blocked. Prove replay/expiry and cross-engagement authorization are rejected.
- Restore completed bytes in a clean room, regenerate equivalent report semantics, then consume a
  valid authorization and prove all live volumes are gone.
- Assert the manifest signature extension is empty and operator surfaces do not claim signer
  identity.

## Traceability

PRD: persistence/disposability, report output, bundle integrity, reporting/auditing, and teardown
verification. Matrix: PRD-REP-001–005, PRD-LIFE-001, PRD-UX-004. Resolved gaps: TR-D03, TR-D04,
TR-D15. ADR links: [ADR-0002](0002-append-only-audit.md),
[ADR-0003](0003-content-addressed-evidence.md), and [ADR-0005](0005-export-semantics.md). Tasks:
V1-002, V1-028–V1-030, V1-032/033, V1-036/037.
