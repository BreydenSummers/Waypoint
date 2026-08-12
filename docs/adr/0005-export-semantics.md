# ADR-0005: Export snapshot, integrity, and teardown semantics

- Status: Accepted
- Scope: v1

## Context

Waypoint is disposable only if an exported engagement remains defensible after the live instance is
wiped. The PRD requires a database dump, evidence, PDF, and SHA-256 for every artifact, and asks that
the report reconstruct after teardown. A manifest cannot contain a stable hash of itself. SHA-256
also does not identify who produced an export, so calling the v1 result signed would be false.

## Decision

- An export records a cutoff and freezes one consistent PostgreSQL snapshot. The versioned report
  input JSON and custom-format database dump are read from that snapshot; referenced evidence is the
  immutable content selected by it. Capture continues outside the snapshot while export runs.
- The report PDF is rendered only from the frozen report input, never from changing live queries.
  Export metadata records schema/contract versions, cutoff, generation tool versions, engagement,
  and known attribution/capture gaps.
- A self-contained bundle contains the database dump, every referenced evidence object, report-input
  JSON, rendered PDF, metadata, and version-compatible offline verify/restore/regenerate tooling and
  instructions. “Self-contained” means no live Waypoint instance or network is needed; documented
  clean-host runtime prerequisites are allowed and must be included in metadata.
- Payload paths are canonical safe relative paths with no symlinks. The versioned manifest lists
  each payload path exactly once with byte length and SHA-256 and rejects extra, missing, duplicate,
  traversing, or changed payloads.
- The manifest is bundle metadata, not a payload entry, because self-hashing is recursive. After the
  archive containing payloads and manifest is finalized, export emits a separate SHA-256 sidecar for
  the complete archive. Verification checks the sidecar first when present, then the manifest and
  all payloads before restore or report regeneration.
- The manifest schema contains an empty, versioned `signatures` extension point. v1 creates no keys
  or signatures and product copy must say “SHA-256 verified” or “hash verified,” never “signed,”
  “signature verified,” or equivalent identity/provenance claims.
- Export is successful only after local verification completes and a durable export receipt is
  audited. Persisted job transitions, receipt authority, and the external destroy handshake follow
  [ADR-0009](0009-export-jobs-receipts-and-teardown.md). The supported destroy flow requires that
  receipt and exact verified bundle path. An interactive `--force` is an explicit preservation
  bypass; host administrators can also delete volumes directly, and both limits are documented.
- Bundles contain sensitive client data. Export warns operators, writes restrictive permissions
  where supported, never silently redacts immutable evidence, and documents encrypted transport and
  storage responsibilities.

## Consequences

The PDF and restored database describe a precise cutoff even while later actions continue in the
live engagement. The archive sidecar covers the manifest without pretending the manifest hashes
itself. Hashes detect changes relative to the exported baseline but do not establish signer identity
or protect confidentiality.

Export needs capacity preflight, progress, cancellation before finalization, safe retry, and
clean-room compatibility testing. A verified export does not automatically destroy anything; Summit
and teardown are separate audited operator decisions.

## Verification

- Export during concurrent capture and prove dump, report input, PDF semantics, and evidence agree on
  the recorded cutoff.
- Mutate, add, remove, duplicate, truncate, or path-traverse payloads and prove verification fails
  before restore; mutate the manifest and prove the archive sidecar fails.
- Restore and regenerate report content in a clean environment after deleting the source instance.
- Scan UI/report/docs for signing claims and assert the signature extension is empty in v1.
- Prove normal destroy is blocked before verified receipt and succeeds for the exact verified bundle.

## Traceability

PRD: locked decisions “Persistence vs. disposability,” “Report output,” and “Bundle integrity,” plus
reporting and teardown verification. Matrix: PRD-REP-001–005, PRD-LIFE-001. Resolved gaps: TR-D03,
TR-D04, TR-D15, TR-D17. Tasks: V1-028–V1-030, V1-032/033, V1-036/037.
