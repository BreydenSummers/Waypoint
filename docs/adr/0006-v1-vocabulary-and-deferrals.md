# ADR-0006: v1 product vocabulary and deferred capability claims

- Status: Accepted
- Scope: v1

## Context

The generic design reference describes a locked linear wizard, while security-assessment phases run
continuously and in parallel. The PRD also discusses deferred graph, model, scan-library, signing, and
platform features alongside v1 audit infrastructure. Without a fixed vocabulary, UI and docs can
accidentally promise deferred capabilities or misdescribe AI attribution as AI analysis.

## Decision

### Canonical v1 operator language

| Term | Required v1 meaning |
|---|---|
| Recon | Discovered environment data and provenance; one of three always-accessible workspaces. |
| Attacks | Every captured attempt and result, including failure/unknown; always accessible. |
| Findings | Human-confirmed reportable results promoted from Attacks; always accessible even empty. |
| Journey log | The precise audit trail/activity feed, not a decorative diary or separate history. |
| Guide's note | Reviewed static phase briefing and contextual what/when/risk technique notes. It does not imply live AI guidance. |
| Your pack / provisions | Collected recon, credentials, evidence, and decisions carried across workspaces. |
| Break camp / Make camp | Optional chrome language for start/resume and pause/persist; never permission state. |
| Fog on the trail | No data discovered in an area yet; never locked, unauthorized, or prerequisite-gated. |
| Summit / Reach the summit | Finalize and export the PDF plus raw bundle, verify it, then separately offer guarded teardown. |
| AI actor | A first-class attributed command initiator with its own token, model/version, human authorizer, and decision context. This is v1 audit infrastructure, not an offensive model feature. |
| Hash verified | SHA-256 payload/outer-archive integrity checked. It never means signed or author-verified. |

Security terms keep their normal technical meaning. For example, “SMB signing” may appear in a
technique note and is unrelated to cryptographic signing of an export. “Scan” may describe a captured
operator command; only the guided Scan library is deferred.

### Explicitly deferred claims

The following are not routes, dependencies, controls, or marketing claims in v1. They may appear only
when clearly labeled deferred/v2, as an inert compatibility extension, or as a negative boundary:

| Scope ID | Deferred capability |
|---|---|
| `deferred-analytical-graph` | Analytical node/attack-path graph or Apache AGE/Neo4j dependency. |
| `deferred-network-zone-map` | Firewall/network-zone reachability map. |
| `deferred-scan-library` | Guided scan library, command catalog, or command-trigger UI. The `scan-library` initiated-by enum may remain reserved. |
| `deferred-offensive-llm` | Waypoint offensive LLM, live AI insight/recommendation UI, or model calls. AI actor capture remains v1. |
| `deferred-ai-finding-prefill` | AI-generated/prefilled findings or autopublish. Promotion is human initiated. |
| `deferred-ai-plugin-generation` | AI parser generation or engagement-time parser upload. `needs-plugin` remains v1. |
| `deferred-windows-offline-agent` | Offline-buffering remote agent for Windows. The Windows operator wrapper remains v1. |
| `deferred-cryptographic-signing` | Cryptographic signing/key management or signer-identity claims. The empty signature hook remains. |
| `nonlinear-phase-gating` | Locked/disabled phase behavior or prerequisite-based waypoint unlocking. |

The machine-readable policy is [`../v1-scope.json`](../v1-scope.json). Product claim surfaces are
linted line by line. A deferred term is acceptable only with explicit boundary context on that line;
architecture/planning sources are excluded because they necessarily specify both versions. New
suppression paths require architecture review—do not evade a finding with creative spelling.

## Consequences

Product copy may remain cosy, but precise security nouns and technical details must remain visible.
All destinations are real navigable links even when empty. Static guidance and AI-actor attribution
can ship without implying that Waypoint itself reasons about offensive next steps.

Scope lint is intentionally a claim guard, not proof that no deferred implementation exists. G0/G5
also require route, dependency, schema, network, and UI inventories. False positives should refine a
narrow policy pattern or wording; broad path exclusions are not an acceptable shortcut.

## Verification

- Run `python3 scripts/lint-architecture.py`; it validates this record against policy scope IDs and
  runs positive/negative scope-rule regressions before scanning claim surfaces.
- Navigate every empty phase and prove fog never sets disabled, authorization, or prerequisite
  behavior.
- Inventory routes/dependencies/content for all deferred Scope IDs at G0 and G5.
- Review report/export copy for “hash verified” and absence of signer-identity claims.

## Traceability

PRD: v1/deferred boundary, Windows scope, guide content model, non-linear adaptation, vocabulary map,
and explicit deferrals. Matrix: PRD-CAP-006, PRD-FIND-001, PRD-REP-004, PRD-UX-001–007,
PRD-DEF-001–007. Resolved gaps: TR-D14 and TR-D18. Tasks: V1-001, V1-021, V1-025, V1-034–V1-037.
