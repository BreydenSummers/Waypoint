# Waypoint v1 PRD traceability

Status: final fail-closed acceptance complete — **Fail; not v1-ready (7 Pass, 15 Fail, 45 Unverified)**  
Latest evidence: [`release-evidence/final-v1-acceptance.md`](release-evidence/final-v1-acceptance.md)  
Canonical requirements: [`../PRD.md`](../PRD.md)  
Execution and task definitions: [`v1-execution-plan.md`](v1-execution-plan.md)  
Design references: [`design-spec.md`](design-spec.md), [`waypoint-mockup.html`](waypoint-mockup.html)  
Architecture records: [`adr/README.md`](adr/README.md)

## How to use this matrix

The identifiers below are planning handles, not replacements for PRD text. A worker cites the relevant IDs in its acceptance evidence. A release owner changes `Planned` to `Pass`, `Fail`, or `Waived` only with links to repeatable test output, screenshots, generated artifacts, or a dated product decision. “Implemented” without evidence is not a valid state.

A PRD change must update its rows, execution tasks, contracts, and affected acceptance gates in the same change. A waiver to a locked decision requires product-owner approval; architectural details may be changed by an ADR if the observable requirement and gate remain intact.

Gate abbreviations are defined in the execution plan: G0 contracts, G1 foundation, G2 capture, G3 workspaces, G4 report/export, G5 release.

### G0 remediation contract freeze

The corrected integration boundary is frozen in [`../contracts/v1/openapi.json`](../contracts/v1/openapi.json)
and its generated catalog. It now normatively covers lossless action persistence, actor
provision/rotate/revoke/read, bounded authoritative actions/entities/evidence reads, standard MCP
Streamable HTTP capture/status, server-assigned out-of-band claims, frozen reports, persisted export
jobs, manifest/receipt binding, and guarded teardown authorization. Synthetic fixtures are verified
by `scripts/verify-contracts.py`. This closes **contract design/drift only**; rows below remain
non-Pass (`Fail` or `Unverified`) until runtime, PostgreSQL, clean-room, security, and operator
evidence satisfy their gates. All v2 deferrals remain unchanged.

### Final acceptance artifact keys

The `2026-08-17T09:20:54Z` fail-closed rerun retains commands, timestamps, exit codes, and raw output
under [`release-evidence/final-v1-acceptance-artifacts/`](release-evidence/final-v1-acceptance-artifacts/).
Rows below link to the relevant repeatable artifact group; unavailable tooling and skipped tests remain
release-blocking rather than becoming proxy passes.

- **ENV** — [host/tool boundary](release-evidence/final-v1-acceptance-artifacts/environment.txt)
- **CONTRACT** — [core contract verifier](release-evidence/final-v1-acceptance-artifacts/verify-contracts.txt)
- **DB** — [Go/PostgreSQL gate summary](release-evidence/final-v1-acceptance-artifacts/go-test-summary.txt), [raw JSON](release-evidence/final-v1-acceptance-artifacts/go-test-json.txt), and [G1 report attempt](release-evidence/final-v1-acceptance-artifacts/g1-report.txt)
- **WEB** — [web tests](release-evidence/final-v1-acceptance-artifacts/web-test.txt) and [deterministic build](release-evidence/final-v1-acceptance-artifacts/web-build.txt)
- **DEPLOY** — [command status ledger](release-evidence/final-v1-acceptance-artifacts/check-status.tsv), [traceability reconciliation](release-evidence/final-v1-acceptance-artifacts/traceability-check.txt), [Compose runtime test](release-evidence/final-v1-acceptance-artifacts/compose-runtime-test.txt), and [smoke attempt](release-evidence/final-v1-acceptance-artifacts/smoke.txt)
- **SOURCE** — [focused implementation/boundary inventory](release-evidence/final-v1-acceptance-artifacts/source-boundary-inventory.txt)
- **SCOPE** — [architecture lint](release-evidence/final-v1-acceptance-artifacts/architecture-lint.txt) and [route/dependency/copy inventory](release-evidence/final-v1-acceptance-artifacts/deferral-inventory.txt)

## 1. Product, data, and audit requirements

| ID | PRD source / requirement | Executable interpretation | Tasks | Verification / gate | Status | Latest retained evidence |
|---|---|---|---|---|---|
| PRD-CORE-001 | Context; capture everything | Every executed collector command is durably spooled before upload; parser absence/failure cannot discard metadata, stdout, or stderr. | V1-009, V1-010, V1-013, V1-014, V1-015 | Known/unknown/parser-crash round trips; interrupted upload/restart; G2 | Unverified | DB, WEB, SOURCE |
| PRD-CORE-002 | Context; defensible attributable audit trail | Captures are immutable and exactly one actor is assigned at collection; every meaningful mutation also emits an append-only actor-attributed audit event. | V1-006, V1-007, V1-008, V1-010 | DB constraints, mutation rejection, concurrent audit integration tests; G1/G2 | Unverified | DB, WEB, SOURCE |
| PRD-CORE-003 | Three phases | Recon, Attacks, and Findings are explicit stored classifications/views; every surfaced item traces to source, actor, time, target, and result. | V1-006, V1-017, V1-023, V1-024, V1-026, V1-027 | Seeded provenance journey and UI drill-through; G3/G4 | Unverified | DB, WEB, SOURCE |
| PRD-CORE-004 | Central question | UI shows what was tried, result/worked state, and unworked/empty state without losing failed attempts. | V1-022, V1-023, V1-024, V1-027 | Operator dogfood with success/failure/unknown/empty data; G3/G5 | Unverified | DB, WEB, SOURCE |
| PRD-DATA-001 | PostgreSQL single instance/container | PostgreSQL is the sole database and exactly one PostgreSQL container is in default Compose; no graph database/object-store service. | V1-005, V1-006 | Compose topology and clean-host startup inspection; G1/G5 | Unverified | DB, WEB, SOURCE |
| PRD-DATA-002 | Proposed engagement/actor/action schema | Migrations preserve all PRD fields, add constraints/status metadata and append-only audit/provenance tables, with no loss of semantics. | V1-006 | Schema review and migration tests against field checklist; G1 | Unverified | DB, WEB, SOURCE |
| PRD-DATA-003 | Evidence blobs outside hot rows, content addressed | stdout/stderr/screenshots stream into hash-addressed volume objects; DB stores metadata and references. | V1-009 | hash/dedup/atomicity/quota/orphan/restart tests; G1 | Unverified | DB, WEB, SOURCE |
| PRD-DATA-004 | Stable-key entity dedup | Engagement-scoped SID, MAC, FQDN precedence; `(hostname, IP)` fallback only; conflicts do not auto-merge. | V1-017 | table-driven and concurrent property tests; G3 | Unverified | DB, WEB, SOURCE |
| PRD-DATA-005 | Manual merge/split | Operators can preview, merge, and split ambiguity while retaining/reversing observation provenance and actor history. | V1-018, V1-023 | merge/split/undo/concurrency E2E; G3 | Unverified | DB, WEB, SOURCE |
| PRD-DATA-006 | Multiple concurrent operators | Writes are transaction/concurrency safe; edits use versions; live feeds do not require polling or lose reconnect state. | V1-008, V1-010, V1-011, V1-026 | race/idempotency/optimistic-conflict and two-browser tests; G2/G4 | Unverified | DB, WEB, SOURCE |
| PRD-AUD-001 | Source attribution per action | Store actor, collector/source agent identity/version/platform, exec-host value/method/interface/confidence, egress mode/value/status/observation, pivot chain, target, command/argv/cwd, start/end/duration, execution status/exit/signal/failure, parse/result, receipt/skew, and evidence references without lossy projection. | V1-002, V1-006, V1-010, V1-013 | frozen action schema/fixture; field-for-field collector → PostgreSQL → authoritative action/report assertions; G0/G2 | Unverified | DB, WEB, SOURCE |
| PRD-AUD-002 | Human and AI actions auditable | Human and AI captures use identical fidelity and collection path; no first-party exemption. | V1-007, V1-010, V1-012, V1-013 | side-by-side human/AI contract assertions; G2 | Unverified | DB, WEB, SOURCE |
| PRD-AUD-003 | AI decision context | AI actor has human authorizer/name/model/version; command has `initiated_by=ai` and decision context. | V1-002, V1-007, V1-010 | reject incomplete AI provisioning/capture; acceptance AI journey; G2 | Unverified | DB, WEB, SOURCE |
| PRD-AUD-004 | “What happened where” survives corrections | Immutable capture plus superseding/audit revisions; ordinary APIs cannot erase command/evidence history. | V1-008, V1-009, V1-026 | attempted delete/update and revision reconstruction tests; G4 | Unverified | DB, WEB, SOURCE |

## 2. Collection and integration requirements

| ID | PRD source / requirement | Executable interpretation | Tasks | Verification / gate | Status | Latest retained evidence |
|---|---|---|---|---|---|
| PRD-CAP-001 | Terminal wrapper captures any command | Wrapper records command envelope, distinct output streams, exit/timing/source/actor/target and returns command behavior without requiring a parser. | V1-002, V1-013 | fixture command matrix on Windows/Linux/macOS; G2/G5 | Unverified | ENV, DB, SOURCE |
| PRD-CAP-002 | Raw-first unknown tools | Unmatched tools are stored raw with `needs-plugin`; failed parser is visible and also retains raw evidence. | V1-010, V1-015, V1-016 | unknown and crashing/invalid parser fixtures; G2 | Unverified | DB, SOURCE |
| PRD-CAP-003 | Plugin contract and selection | Match binary first, optional argv/regex specificity resolves ties; parser emits schema-validated result/entities; every plugin has sample/expected fixtures. | V1-003, V1-015, V1-016 | cross-repo contract suite and tie/invalid-schema tests; G0/G2 | Unverified | CONTRACT, DB, SOURCE |
| PRD-CAP-004 | Cross-OS wrapper | Release binaries and tests cover Windows/Linux/macOS supported architectures. | V1-013, V1-034 | retained platform matrix artifacts; G5 | Unverified | ENV, DB, SOURCE |
| PRD-CAP-005 | Linux/macOS offline agent | Small agent durably buffers locally through network/process restart and syncs original timestamps exactly once. | V1-014, V1-034 | disconnect/capture/restart/reconnect on Linux/macOS; G2/G5 | Unverified | ENV, DB, SOURCE |
| PRD-CAP-006 | Windows agent deferred | No v1 claim or release gate for offline Windows agent; wrapper remains required on Windows. | V1-001, V1-034, V1-036 | scope/docs/UI claim scan; G5 | Unverified | ENV, DB, SOURCE |
| PRD-CAP-007 | Official REST ingestion | Authenticated, versioned, idempotent REST capture is the common application service used by collectors. | V1-002, V1-010 | OpenAPI conformance and retry/conflict tests; G2 | Unverified | CONTRACT, DB, SOURCE |
| PRD-CAP-008 | Official MCP ingestion | One standard MCP Streamable HTTP endpoint implements initialize/discovery/tools-call; only capture/status tools exist and use the same auth, validation, idempotency, problem, action, evidence, and audit service as REST. | V1-002, V1-012 | protocol conformance plus REST/MCP parity fixture/suite and closed tool inventory; G0/G2 | Unverified | CONTRACT, DB, SOURCE |
| PRD-CAP-009 | Best-effort out-of-band detection | Server-assigned entity/result claims without a valid engagement-scoped captured action remain pending review; link/dismiss is authoritative and audited, and no claim is silently presented as captured. | V1-002, V1-019 | pending/link/dismiss fixtures, queue/API tests, resolution audit, boundary docs; G0/G3/G5 | Unverified | DB, SOURCE |
| PRD-CAP-010 | Honest external-AI boundary | Docs state that wholly out-of-band AI or human execution cannot be guaranteed captured. | V1-019, V1-036 | doc assertion/release review; G5 | Fail | DB, SOURCE |
| PRD-CAP-011 | At-least durable, exactly-once materialization | Collector persists an actor/source capture ID until ack; server idempotency maps retries to one action and rejects mismatched duplicates. | V1-010, V1-013, V1-014 | retry/race/payload-conflict/failure injection; G2 | Unverified | ENV, DB, SOURCE |

## 3. Identity, networking, and real-time requirements

| ID | PRD source / requirement | Executable interpretation | Tasks | Verification / gate | Status | Latest retained evidence |
|---|---|---|---|---|---|
| PRD-ID-001 | No shared token; named operators/accounts | Bootstrap provisions actor-specific high-entropy credentials; no anonymous/default/shared credential; tokens are shown once, digest-stored, role/engagement scoped, revocable. | V1-007, V1-031 | auth matrix, DB/log secret checks, two-operator journey; G1/G5 | Fail | ENV, DB, SOURCE |
| PRD-ID-002 | AI actor first class | `actor.kind=ai_agent`, own token, mandatory human authorization metadata, same role isolation. | V1-007 | provisioning rejection/authorization/revocation tests; G1 | Unverified | ENV, DB, SOURCE |
| PRD-NET-001 | Per-action exec host IP | Collector sends selected local source address and collection method/uncertainty; server does not substitute HTTP peer. | V1-002, V1-010, V1-013 | multi-interface fixture and field round trip; G2 | Unverified | ENV, DB, SOURCE |
| PRD-NET-002 | Per-action egress and pivot chain | Capture stores public/NAT egress value/status/observation and ordered validated optional pivots. | V1-002, V1-005, V1-010 | mode and malformed pivot tests; G2 | Unverified | ENV, DB, SOURCE |
| PRD-NET-003 | Egress auto/manual/off | Startup-valid mode: auto calls only configured resolver; manual/off issue zero discovery traffic; off stores disabled/null and failures are visible. | V1-005 | packet/network-mock assertions and UI/report gap; G1/G4 | Fail | ENV, DB, SOURCE |
| PRD-RT-001 | SSE for live updates, REST writes | Writes remain REST; authorized SSE emits persisted cursor IDs, reconnects via `Last-Event-ID`, and resyncs without data loss. | V1-011, V1-022 | disconnect/gap/slow-client/revocation tests; G2/G3 | Unverified | DB, WEB, SOURCE |
| PRD-RT-002 | v1-lite alerts | Deterministic successful-auth and first-new-segment alerts arrive live, deduplicate, and link to source captures. | V1-020, V1-023 | rule fixtures and SSE E2E; G3 | Unverified | DB, WEB, SOURCE |

## 4. Findings, report, lifecycle, and deployment requirements

| ID | PRD source / requirement | Executable interpretation | Tasks | Verification / gate | Status | Latest retained evidence |
|---|---|---|---|---|---|
| PRD-FIND-001 | Manual attack promotion | Only an operator explicitly promotes; evidence action(s) auto-link and required report fields are human completed. No AI autopublish/prefill. | V1-026, V1-027 | promotion permissions/validation/evidence E2E and v2 claim scan; G4 | Unverified | DB, WEB, SOURCE |
| PRD-FIND-002 | Finding fields and attribution | Title, severity, affected entities, evidence, remediation, status, promoter/time, and revision actor/time are persisted/reportable. | V1-006, V1-026 | schema and edit-history reconstruction tests; G4 | Unverified | DB, WEB, SOURCE |
| PRD-REP-001 | PDF from audit trail | Versioned frozen snapshot generated from authoritative actions/findings renders a human PDF with methodology, full source attribution, evidence links, and known pending/dismissed capture gaps. | V1-002, V1-028 | frozen report/action schemas; golden semantic report assertions and malicious-content escaping; G0/G4 | Fail | DB, WEB, SOURCE |
| PRD-REP-002 | Self-contained raw bundle | Completed server job inventories custom-format DB dump, all referenced evidence, PDF, report snapshot, metadata, instructions, and offline verify/restore/regenerate tooling. | V1-002, V1-029 | manifest fixture; clean-room verify/restore/report regeneration after source wipe; G0/G4/G5 | Fail | DB, WEB, SOURCE |
| PRD-REP-003 | SHA-256 every artifact | Manifest lists unique safe path/size/hash for every payload file; manifest is excluded from self-reference and a receipt binds the separate outer archive hash. | V1-001, V1-002, V1-029 | manifest/receipt schema; mutate/add/remove/path attack tests and all-entry verification; G0/G4 | Fail | DB, WEB, SOURCE |
| PRD-REP-004 | Signing deferred with hook | Manifest schema requires a versioned empty signature extension; UI/report says hash-verified and makes no signer-identity claim. | V1-001, V1-002, V1-029, V1-036 | schema fixture and claim/content scan; G0/G4/G5 | Fail | DB, WEB, SOURCE |
| PRD-REP-005 | Export remains live-safe | Server-owned persisted job records consistent cutoff, capacity preflight, progress/cancel/failure/recovery, completion, and immutable verified receipt while capture continues outside the snapshot. | V1-002, V1-029, V1-030, V1-032 | job state fixture; reconnect, concurrent capture/export, disk-full/interruption tests; G0/G4/G5 | Fail | DB, WEB, SOURCE |
| PRD-LIFE-001 | Disposable after export | Supported destroy command re-verifies exact bundle bytes against a persisted completed receipt, consumes a short-lived bound authorization, removes app/DB/evidence volumes, and documents break-glass/direct-admin limits. | V1-002, V1-030, V1-036 | receipt/authorization fixture; blocked-before-export, mutation/replay, successful teardown, forced warning tests; G0/G4/G5 | Fail | DB, WEB, SOURCE |
| PRD-DEP-001 | One-step Compose setup | Clean `docker compose up` starts app + one PostgreSQL container with fresh reachable empty engagement path. | V1-005, V1-031 | clean Linux/macOS/Windows host smoke tests; G1/G5 | Unverified | ENV, DB, DEPLOY, SOURCE |
| PRD-DEP-002 | Install script supported hosts | Idempotent Ubuntu 22.04/24.04 x86_64 install validates config, provisions stack/accounts, and exposes rollback/diagnostics. | V1-031 | clean/repeated VM install tests; G5 | Unverified | ENV, DB, DEPLOY, SOURCE |
| PRD-DEP-003 | Account provisioning | Operational path creates named actors/accounts and one-time credentials without database hand-editing. | V1-007, V1-031 | clean-host provision/revoke/rotate journey; G1/G5 | Unverified | ENV, DB, DEPLOY, SOURCE |
| PRD-DEP-004 | Sensitive local deployment | TLS outside loopback, restrictive secret/data permissions, redacted logs, and explicit host/disk encryption responsibility. | V1-005, V1-007, V1-033, V1-036 | config rejection, permission/log checks, docs; G5 | Fail | ENV, DB, DEPLOY, SOURCE |

## 5. UX, guide, accessibility, and performance requirements

| ID | PRD source / requirement | Executable interpretation | Tasks | Verification / gate | Status | Latest retained evidence |
|---|---|---|---|---|---|
| PRD-UX-001 | Illustrated expedition shell in v1 | Home is trail map/navigation with Recon, Attacks, Findings, Journey log, and Summit; workspaces stay clean and woodland styling stays in chrome. | V1-021, V1-023, V1-024, V1-027, V1-030 | visual comparison to both references; G3/G5 | Unverified | ENV, WEB, DEPLOY |
| PRD-UX-002 | Non-linear phases | Every phase/milestone is always accessible; fog means empty data, not permission or prior-stage completion. | V1-021, V1-023, V1-024, V1-027 | empty-engagement keyboard/click navigation; G3 | Unverified | ENV, WEB, DEPLOY |
| PRD-UX-003 | Journey log is the audit trail | Live feed uses guide vocabulary while exposing exact actor/time/source/target/result and immutable details. | V1-008, V1-022 | audit-to-log row parity and reconnect E2E; G3 | Unverified | ENV, WEB, DEPLOY |
| PRD-UX-004 | Summit means finalization/export | Summit starts verified report/bundle flow and guarded post-export teardown; it is not a gated wizard stage. | V1-030 | empty/non-empty navigation and export flow dogfood; G4/G5 | Fail | DB, WEB, DEPLOY, SOURCE |
| PRD-UX-005 | Static guide note | Every phase has reviewed always-available briefing independent of AI/network. | V1-025 | content coverage test and offline UI dogfood; G3 | Unverified | ENV, WEB, DEPLOY |
| PRD-UX-006 | Contextual technique notes | Reviewed keyed notes cover what/when/risks and surface contextually; v1 does not promise scan-library exact commands. | V1-025 | content schema/link/search/context tests and scope review; G3 | Unverified | ENV, WEB, DEPLOY |
| PRD-UX-007 | Guide-style errors | Operator-facing validation is actionable/security-domain prose while preserving machine error codes/details for support. | V1-021, V1-023, V1-027, V1-030 | invalid-flow copy review and accessibility checks; G5 | Unverified | ENV, WEB, DEPLOY |
| PRD-UX-008 | Fixed artifact palette and dark mode | Codified PRD palette; map/parchment fixed colors in both modes; surrounding chrome themes; approved contrast pairs. | V1-021, V1-035 | token tests, contrast audit, light/dark screenshots; G3/G5 | Unverified | ENV, WEB, DEPLOY |
| PRD-UX-009 | Motion | Trail/current motion stays under 1s for transitions, remains subtle, and disables with `prefers-reduced-motion`; no scroll hijack. | V1-021, V1-035 | reduced-motion and timing tests/dogfood; G5 | Unverified | ENV, WEB, DEPLOY |
| PRD-A11Y-001 | Map non-visual equivalent | Ordered/labeled state list exists; destinations are real links/buttons with focus and current state. | V1-021, V1-035 | keyboard, accessibility-tree, axe tests; G3/G5 | Unverified | ENV, WEB, DEPLOY |
| PRD-A11Y-002 | Text/contrast/mobile | Minimum 12px map/13px elsewhere; approved contrast; vertical/mobile map and full-screen workspaces/compact trail affordance. | V1-021, V1-035 | computed style/contrast and mobile viewport dogfood; G5 | Unverified | ENV, WEB, DEPLOY |
| PRD-PERF-001 | Views/queries/import feel instant | Common list API p95 <=200ms/p99 <=500ms on baseline; keyset pagination/index plans; no unbounded endpoint. | V1-006, V1-022, V1-023, V1-024, V1-032 | seeded benchmark and query-plan regression; G5 | Fail | DB, SOURCE |
| PRD-PERF-002 | Streaming ingest | Evidence memory <=32 MiB incremental/upload; metadata ack p95 <=500ms after body; no full-output buffering in API. | V1-009, V1-010, V1-032 | 10 GiB/concurrent ingest metrics and heap profile; G2/G5 | Fail | DB, SOURCE |
| PRD-PERF-003 | Fast real-time/UI | Committed action to SSE p95 <=1s; warm primary route usable <=2s; local interactions <=100ms excluding network. | V1-011, V1-021, V1-032 | automated telemetry budgets; G5 | Fail | DB, SOURCE |
| PRD-QUAL-001 | Bulletproof by default | Unit/contract/real-DB/E2E/security/fault/performance/platform tests cover capture and attribution; flakes fail release. | V1-032, V1-033, V1-034, V1-037 | full suite and retained reports; G5 | Fail | ENV, DB, WEB, DEPLOY, SOURCE |
| PRD-QUAL-002 | Dogfooded design loop | Every UI feature is operated in running app and visually inspected desktop/mobile in light and dark, not closed by tests alone. | V1-021–V1-025, V1-027, V1-030, V1-035 | timestamped screenshots and flow checklist; G3/G5 | Unverified | ENV, DB, WEB, DEPLOY, SOURCE |

## 6. Explicit deferrals and anti-scope checks

| ID | PRD source / requirement | v1 treatment | Tasks | Verification / gate | Status | Latest retained evidence |
|---|---|---|---|---|---|
| PRD-DEF-001 | Analytical node/attack-path graph deferred | No graph route, schema dependency, visualization, Apache AGE requirement, or graph marketing claim. Entity tables are provenance/dedup, not a graph UI. | V1-001, V1-037 | route/dependency/content inventory; G0/G5 | Pass | SCOPE, SOURCE |
| PRD-DEF-002 | Network-zone/firewall map deferred | No reachability-map visualization; v1 new-segment alert is only a deterministic result. | V1-020, V1-037 | feature/content inventory; G5 | Pass | SCOPE, SOURCE |
| PRD-DEF-003 | Scan library deferred | Preserve enum/extension compatibility only; no guided command catalog or command-trigger UI. | V1-001, V1-025, V1-037 | API/UI/content claim scan; G0/G5 | Pass | SCOPE, SOURCE |
| PRD-DEF-004 | Offensive LLM/live insights deferred | No model dependency/calls, AI recommendations, or live-insight UI. AI actor ingestion remains v1 audit infrastructure. | V1-007, V1-025, V1-037 | dependency/network/content inventory; G5 | Pass | SCOPE, SOURCE |
| PRD-DEF-005 | AI finding prefill deferred | Findings begin only through operator promotion; no generated drafts/autopublish. | V1-026, V1-037 | API/flow negative tests; G4/G5 | Pass | SCOPE, SOURCE |
| PRD-DEF-006 | AI plugin generation deferred | `needs-plugin` queue is retained, but core does not generate/upload code at engagement time. | V1-015, V1-019, V1-037 | endpoint/UI inventory and parser boundary test; G5 | Pass | SCOPE, SOURCE |
| PRD-DEF-007 | Cryptographic signing deferred | No key management or identity claims; only hash manifest and extension hook. | V1-029, V1-036 | report/UI/doc terminology scan; G4/G5 | Pass | SCOPE, SOURCE |

## 7. Resolved gaps and edge-case decisions

These are the implementation interpretations used by the matrix. V1-001 records its architecture
scope in the [ADR index](adr/README.md): system shape in ADR-0001, audit breadth/corrections in
ADR-0002, evidence isolation in ADR-0003, parser trust in ADR-0004, manifest/reconstruction semantics
in ADR-0005, phase/scope vocabulary in ADR-0006, lossless attribution/actor lifecycle/authoritative
reads in ADR-0007, standard MCP in ADR-0008, and persisted export/receipt/teardown lifecycle in
ADR-0009. Remaining rows stay executable planning decisions until their owning task requires a
dedicated ADR.

| Decision | Gap or tension in source text | Resolution / observable rule |
|---|---|---|
| TR-D01 Audit breadth | `action` is the audit spine, but merges, finding edits, exports, and provisioning also need attribution. | Keep command `action`; add append-only `audit_event` emitted transactionally for every state change. No anonymous system mutation. |
| TR-D02 Egress nullability | “Attacker IP always known” conflicts with explicit `egress=off` and resolver failure. | Exec-host IP and egress status always exist. Egress value may be null only with `disabled`/`resolution_failed`, prominently shown in UI/report. |
| TR-D03 Manifest self-hash | A manifest cannot include a stable hash of itself. | Hash every payload file except the manifest; emit an outer archive hash. Do not describe this as signing. |
| TR-D04 Self-contained reconstruction | DB dump + evidence + PDF is stated, while verification says report reconstructs after wipe. | Also export versioned report-input JSON plus bundled offline verify/restore/regenerate tooling and test it in a clean environment. |
| TR-D05 Evidence dedup vs engagement isolation | Content addressing can accidentally reveal another engagement's hash/existence. | Physical bytes may deduplicate, but API references/authorization and responses remain engagement scoped; no cross-engagement existence oracle. |
| TR-D06 Dedup split provenance | A single mutable entity row cannot safely split DHCP/shared-hostname ambiguity. | Store immutable observations/identifiers and merge lineage; split moves selected observations and is audited/reversible. |
| TR-D07 Parser trust | Plugin code parses hostile tool output, while raw capture must never be gated. | Only release-pinned parser artifacts run out of process with limits; validate output; timeout/crash/invalid output degrades to retained raw. No runtime plugin upload. |
| TR-D08 Exactly-once offline replay | Network acknowledgements can be lost. | Actor/source capture UUID gives idempotent materialization; collectors delete spool only after durable ack; changed duplicate is rejected/audited. |
| TR-D09 SSE history | Auto-reconnect alone can miss events over deploy/outage. | Persist audit IDs, honor `Last-Event-ID`, and provide paginated REST resync. Slow clients are disconnected rather than blocking ingest. |
| TR-D10 Out-of-band detection | It is impossible to detect commands wholly outside Waypoint. | Flag only observed claims lacking captured provenance and document that no observation means no detection guarantee. |
| TR-D11 “Single container” wording | PRD calls PostgreSQL a single instance/one container while Compose must stand up full stack. | One PostgreSQL container, one app image/container by default; no second database/object store. This does not mean the entire system runs in one container. |
| TR-D12 Install support unspecified | “Supported host” has no v1 OS list. | Gate the install script on Ubuntu 22.04/24.04 LTS x86_64; Compose remains the portable default. |
| TR-D13 Alerts examples not contract | “e.g.” notable results leaves v1-lite breadth open. | Initial deterministic rules are successful auth and first newly reachable segment, derived from validated plugin output, deduplicated and source-linked. |
| TR-D14 Guide exact commands | v1 contextual how-to mentions later exact command tied to v2 scan library. | v1 notes cover what/when/risk, not a guided executable command catalog. |
| TR-D15 Teardown bypass | Application cannot stop a host admin from deleting Docker volumes directly. | Guard the supported destroy command after verified export; provide explicit break-glass warning and document host-admin boundary. |
| TR-D16 AI authorization type | Schema text permits actor authorizer in one place, locked decision requires human. | v1 enforces `authorized_by` references a human actor. |
| TR-D17 Raw sensitive data | Capture-everything can include credentials; encryption at rest is not specified. | TLS, restrictive storage permissions, sensitive export warning, and documented host/disk encryption responsibility; do not silently redact immutable raw evidence. UI/logs minimize accidental disclosure. |
| TR-D18 Phase gating | Design spec's generic mockup has locked waypoints, PRD expressly rejects a wizard. | All Waypoint destinations are navigable; visual fog is empty-state only and must never set disabled/permission behavior. |
| TR-D19 Accepted-vs-persisted attribution | The capture envelope validates complete attribution, while a summary action/event projection can silently drop required semantics. | The immutable authoritative action nests the complete accepted envelope plus credential-derived actor/engagement, receipt/skew, evidence references, and audit cursor; summaries never replace it. |
| TR-D20 Official MCP meaning | A custom HTTP route named MCP is not interoperable with MCP clients. | One standard Streamable HTTP JSON-RPC endpoint supports initialize, initialized notification, tools/list, and only capture/status tools; application behavior delegates to REST's service. |
| TR-D21 Export authority | Browser-local progress/receipt cannot prove a live-safe self-contained export or safely guard teardown. | Persist server-owned export state and server-generated verified receipt; teardown re-verifies exact bytes and consumes a short-lived receipt-bound authorization through external orchestration. |

## 8. Release evidence index

Reconciled during the final fail-closed acceptance; do not mark the release complete if any applicable row lacks a durable artifact.

| Evidence ID | Required artifact | Covers | State / location |
|---|---|---|---|
| EV-01 | Contract compatibility reports from both repositories | PRD-CAP-003, PRD-CAP-007/008 | Unverified · core contracts and standard MCP fixtures pass, but no retained plugins compatibility/parser report exists; [final acceptance](release-evidence/final-v1-acceptance.md#ev-01ev-16-disposition) |
| EV-02 | Migration/constraint/auth isolation report | PRD-CORE-002, PRD-DATA-001/002, PRD-ID-001/002 | Fail · 39 PostgreSQL tests failed closed and digest text is still accepted directly as a bearer credential; [final acceptance](release-evidence/final-v1-acceptance.md#ev-01ev-16-disposition) |
| EV-03 | Human/AI/unknown-tool capture transcript | PRD-CORE-001, PRD-AUD-001/002/003, PRD-CAP-002 | Unverified · attribution schema is now lossless, but no collector-to-PostgreSQL transcript exists; [final acceptance](release-evidence/final-v1-acceptance.md#ev-01ev-16-disposition) |
| EV-04 | Offline replay + platform matrix | PRD-CAP-004/005/011 | Unverified · no retained plugins runtime or supported-platform artifacts are present; [final acceptance](release-evidence/final-v1-acceptance.md#ev-01ev-16-disposition) |
| EV-05 | Egress mode packet assertions | PRD-NET-001/002/003 | Fail · startup resolver/config and manual/off packet evidence are absent; [final acceptance](release-evidence/final-v1-acceptance.md#ev-01ev-16-disposition) |
| EV-06 | Entity dedup/merge/split provenance report | PRD-DATA-004/005 | Unverified · PostgreSQL tests failed closed and no operator journey is retained; [final acceptance](release-evidence/final-v1-acceptance.md#ev-01ev-16-disposition) |
| EV-07 | Two-browser SSE/concurrency recording | PRD-DATA-006, PRD-RT-001/002 | Unverified · PostgreSQL tests failed closed and no browser recording exists; [final acceptance](release-evidence/final-v1-acceptance.md#ev-01ev-16-disposition) |
| EV-08 | Finding-to-evidence-to-report trace report | PRD-FIND-001/002, PRD-REP-001 | Fail · no authoritative retained PDF exists and report snapshot/gap semantics remain incomplete; [final acceptance](release-evidence/final-v1-acceptance.md#ev-01ev-16-disposition) |
| EV-09 | Bundle manifest, outer hash, clean-room restore log | PRD-REP-002/003/004/005 | Fail · production export is not a PostgreSQL dump/archive or self-contained clean-room restore; fixture bytes remain synthetic; [final acceptance](release-evidence/final-v1-acceptance.md#ev-01ev-16-disposition) |
| EV-10 | Guarded teardown and post-wipe bundle verification | PRD-LIFE-001 | Fail · destroy does not hash exact bundle bytes or consume receipt-bound authorization, and no post-wipe reconstruction ran; [final acceptance](release-evidence/final-v1-acceptance.md#ev-01ev-16-disposition) |
| EV-11 | Clean Compose and installer logs | PRD-DEP-001/002/003 | Unverified · definitions exist, but no Docker stack or supported Ubuntu VM run is retained; [final acceptance](release-evidence/final-v1-acceptance.md#ev-01ev-16-disposition) |
| EV-12 | Performance and fault benchmark report | PRD-PERF-001/002/003, PRD-QUAL-001 | Fail · no measured baseline performance/fault report exists; [final acceptance](release-evidence/final-v1-acceptance.md#ev-01ev-16-disposition) |
| EV-13 | Security test/scan report and residual-boundary review | PRD-DEP-004, PRD-CAP-009/010, PRD-QUAL-001 | Fail · no retained security report; auth, TLS/header/origin, scan, and boundary-doc defects remain; [final acceptance](release-evidence/final-v1-acceptance.md#ev-01ev-16-disposition) |
| EV-14 | Light/dark desktop/mobile screenshots and UX checklist | PRD-UX-001–009, PRD-QUAL-002 | Unverified · no browser binary or retained screenshot/checklist set exists; [final acceptance](release-evidence/final-v1-acceptance.md#ev-01ev-16-disposition) |
| EV-15 | Accessibility tree/keyboard/axe/reduced-motion report | PRD-A11Y-001/002 | Unverified · no browser accessibility artifact exists; [final acceptance](release-evidence/final-v1-acceptance.md#ev-01ev-16-disposition) |
| EV-16 | Route/dependency/copy inventory proving deferrals | PRD-DEF-001–007 | Pass · architecture lint over 159 claim-surface files plus route/dependency/copy inventory show no functioning v2 surface; [final acceptance](release-evidence/final-v1-acceptance.md#ev-01ev-16-disposition) |

## 9. Planning coverage check

The matrix covers every locked-decision row and every verification bullet in the PRD:

- setup/install/provisioning: PRD-DEP-001–003;
- known local and remote capture: PRD-CAP-001/003/004/005;
- raw fallback: PRD-CAP-002;
- distinct human attribution: PRD-ID-001 and PRD-AUD-001;
- AI attribution and rationale: PRD-ID-002 and PRD-AUD-002/003;
- offline buffering: PRD-CAP-005/011;
- findings/report/hash bundle: PRD-FIND-001/002 and PRD-REP-001–005;
- teardown and self-contained restore: PRD-LIFE-001 and PRD-REP-002;
- plugin fixture/core-path tests: PRD-CAP-003 and PRD-QUAL-001;
- mandatory light/dark dogfood: PRD-QUAL-002 and PRD-UX/A11Y rows.

If a future PRD clause cannot map to an ID, task, verification, and retained artifact, planning is incomplete and G5 must not pass.
