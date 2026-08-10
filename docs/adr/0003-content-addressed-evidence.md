# ADR-0003: Content-addressed evidence storage

- Status: Accepted
- Scope: v1

## Context

Raw stdout, stderr, screenshots, and other evidence can be large and hostile. Keeping bytes in hot
PostgreSQL rows would undermine ingest and query budgets, while adding an object-store service would
violate the small disposable topology. Content addressing enables integrity checks and physical
deduplication, but careless global deduplication could leak another engagement's content existence.

## Decision

- Evidence bytes live in a mounted content-addressed filesystem volume outside hot database rows.
  PostgreSQL stores an immutable logical evidence reference containing engagement, SHA-256 digest,
  byte length, media type, evidence role, source action/provenance, and creation metadata.
- stdout and stderr are always separate logical evidence objects even when their bytes happen to be
  identical. Content supplied by tools never controls a storage path or rendered filename.
- Ingest streams through a bounded temporary file while calculating SHA-256 and length. It verifies
  declared limits/digest, fsyncs and atomically renames the blob to its digest-derived path, then
  commits the database reference. Existing blobs are accepted only after size/hash verification.
- A failed database transaction may leave an unreferenced placed blob, but must never leave a
  committed reference to absent bytes. Startup/periodic reconciliation removes only stale temporary
  files and provably unreferenced blobs after a safety interval; committed evidence is not garbage
  collected during an engagement.
- Physical bytes may deduplicate across references, including across engagements. Logical references,
  authorization checks, quotas, audit output, timing, and API responses remain engagement scoped.
  Upload/read behavior must not reveal whether another engagement already stores a digest.
- Reads and exports revalidate length and digest. A mismatch is a visible integrity failure and may
  never be silently repaired by changing the database digest.
- Filesystem permissions are restrictive and transport is encrypted. Host/disk encryption is a
  deployment responsibility; hashes provide integrity, not confidentiality.

## Consequences

Evidence ingest and export remain streaming and memory bounded without another service. Atomic
filesystem placement cannot be transactionally rolled back with PostgreSQL, so reconciliation is a
required subsystem and tests must exercise every crash point. Physical deduplication is an internal
optimization only; callers always operate on opaque engagement-scoped evidence IDs rather than raw
hash-presence APIs.

Corrections attach superseding metadata or evidence and retain the original under
[ADR-0002](0002-append-only-audit.md). Bundle handling follows
[ADR-0005](0005-export-semantics.md).

## Verification

- Inject interruption before/during/after hash, rename, and DB commit; prove no committed dangling
  reference and safe cleanup of temporary/orphan data after restart.
- Test wrong digest/length, quota breach, duplicate concurrent upload, and corrupted-on-disk read.
- Test cross-engagement read/IDOR and timing/response parity for present versus absent foreign blobs.
- Profile a large upload and enforce the bounded-memory budget.

## Traceability

PRD: proposed `evidence_blob`, raw-first collection, performance principle, and export verification.
Matrix: PRD-DATA-003, PRD-CORE-001, PRD-PERF-002, PRD-REP-002/003. Resolved gaps: TR-D05 and
TR-D17. Tasks: V1-006, V1-009, V1-010, V1-029, V1-032/033.
