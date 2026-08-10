# ADR-0004: Parser isolation and raw-first failure behavior

- Status: Accepted
- Scope: v1

## Context

Parser plugins consume attacker-controlled and tool-controlled output. They are useful for structured
results but cannot sit on the capture durability path: no match, timeout, crash, malformed output, or
compromised parser must not lose raw evidence or execute inside the core service. Engagement-time
code upload would turn a convenience feature into a remote-code-execution surface.

## Decision

- The collector durably spools the capture envelope and distinct raw stdout/stderr before parser work.
  Raw action ingestion is independent of parser success. No match yields `needs-plugin`; parser
  failure yields a visible failed/raw status while retaining the same complete capture.
- Parser artifacts are reviewed, release-pinned by version/digest, and distributed from the
  collection repository. There is no engagement-time plugin upload, generation, installation, or
  arbitrary parser path.
- Matching is deterministic: binary name first, then declared argv/regex heuristics, with ties won by
  the most specific declared rule or rejected as ambiguous. The selected plugin ID/version and match
  reason accompany any structured result.
- Parsers run out of process under the least privilege available on each supported platform: no
  network, no application/database/evidence-volume access, read-only bounded input over a controlled
  channel, isolated scratch space, and wall-clock, CPU, memory, process, and output limits. A limit or
  sandbox failure terminates parsing and falls back to raw capture.
- Parser output is untrusted even though the artifact is pinned. It must satisfy the negotiated
  plugin-contract version and registered JSON Schema before core stores a `structured_result` or
  entity observation. Strings are rendered as text, identifiers are normalized by core, and parser
  output cannot choose authorization scope or evidence paths.
- Parser retries are idempotent derived processing. They may append a new structured-result revision
  linked to the immutable action but cannot alter raw bytes or action attribution.

## Consequences

Structured parsing may arrive after the raw action and UI/SSE consumers must represent that state
transition. Isolation has platform-specific implementations, but weakening it silently is not an
acceptable fallback. A platform unable to establish the required boundary may still capture raw and
must report parsing unavailable.

Plugin compatibility is proven with versioned fixtures in both repositories. The core application
contains no shell, parser loader, or generic execution API; its only parser-facing responsibility is
contract and schema validation.

## Verification

- Run known, unknown, ambiguous, crashing, hanging, oversized-output, and invalid-schema fixtures;
  every case retains one immutable raw action.
- Attempt parser network, filesystem, subprocess, and resource-limit escapes on each supported
  parser platform.
- Prove unregistered plugin versions and runtime uploads are rejected.
- Run the same valid/invalid contract fixtures in core and plugin repositories.

## Traceability

PRD: raw-first collection and proposed plugin contract. Matrix: PRD-CORE-001, PRD-CAP-002/003,
PRD-CAP-006, PRD-QUAL-001. Resolved gap: TR-D07. Tasks: V1-002/003, V1-010, V1-015/016,
V1-033/034.
