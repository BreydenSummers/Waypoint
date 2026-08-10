# Waypoint v1 execution plan

Status: proposed execution baseline  
Source of truth: [`../PRD.md`](../PRD.md)  
Visual references: [`design-spec.md`](design-spec.md), [`waypoint-mockup.html`](waypoint-mockup.html)  
Traceability: [`v1-traceability.md`](v1-traceability.md)

## 1. Outcome and release boundary

v1 is complete when a team can stand up a fresh shared instance, provision individually attributable human and AI actors, capture commands reliably from supported operator and remote platforms, work non-linearly through Recon / Attacks / Findings, and export a verified self-contained engagement bundle before wiping the instance.

The release includes:

- raw-first command capture, parser integration, entity provenance/deduplication, and a durable Journey log;
- REST writes, resumable SSE reads, official REST and MCP ingestion paths, deterministic v1-lite alerts;
- the expedition shell, clean phase workspaces, static guide notes, and contextual technique notes;
- manual attack-to-finding promotion, evidence linkage, PDF generation, database/evidence export, and SHA-256 verification;
- Docker Compose, a supported-host installer, account/token provisioning, guarded teardown, and restore/verification instructions;
- operator wrapper on Windows, Linux, and macOS; offline remote agent on Linux and macOS.

Explicitly excluded: analytical/node/attack-path/network-zone graphs, guided scan library, offensive LLM, AI finding prefill, Windows offline remote agent, plugin generation, and cryptographic signing. The v1 schema retains `scan-library` and signature extension points but does not expose them as functioning features.

## 2. Decisions made to make the PRD executable

These decisions refine, but do not replace, the PRD. A change to a locked PRD decision requires product-owner approval and an update to both planning documents.

### 2.1 System shape and repository boundary

- **Core (`Waypoint`)**: Go API/application service, PostgreSQL, and a React + TypeScript web client built to static assets served by the application. One versioned application image plus one PostgreSQL container is the default Compose topology. Go is selected for bounded resource use, streaming I/O, straightforward cross-platform CLIs, and a small operational surface; React/TypeScript is selected for the SVG map and accessible data-heavy workspaces.
- **Collection (`Waypoint-Plugins`)**: Go wrapper/agent binaries, plugin SDK/contracts, parser implementations, and parser fixtures. The core repo owns ingestion schemas and compatibility fixtures; the plugin repo consumes them. Artifacts negotiate an explicit API/plugin-contract version.
- **Storage**: PostgreSQL is authoritative. Evidence bytes live outside hot rows in a content-addressed filesystem volume; PostgreSQL stores hashes, sizes, media types, and provenance. This remains a single logical store and avoids adding an object-store dependency.
- **No command execution in core**: the server records outcomes; only the explicit wrapper/agent executes commands. Parsers are release-pinned trusted artifacts, run out of process with time/memory/output limits and no engagement-time plugin upload. All parser output remains untrusted input and is schema validated.
- **PDF generation**: a pinned headless Chromium build renders a versioned, print-only report route from a frozen export snapshot. It ships in the application image; there is no external renderer or network dependency.

### 2.2 Audit model and mutability

The PRD's `action` remains the command-execution spine. Add an append-only `audit_event` table for every meaningful state mutation (provision, capture receipt, merge/split, finding edit/promotion, export, teardown authorization). Each event has engagement, actor, timestamp, event type, subject, request/correlation ID, and redacted before/after metadata. The initiating actor is mandatory; server-derived events inherit the actor/source action rather than becoming anonymous.

Captured metadata and evidence are immutable. Corrections are explicit superseding records/events, not in-place history erasure. Finding drafts may be edited optimistically, but each revision is recorded. Audit/event and evidence deletion is forbidden through the normal API.

Add `entity_identifier`, `entity_observation`, and merge lineage around the proposed `entity` table. Deduplication operates on normalized, engagement-scoped identifiers in this order: AD SID, MAC, FQDN, then normalized `(hostname, IP)`. Every merge retains observation provenance; split reassigns selected observations and is reversible/audited. Conflicting stable identifiers never auto-merge.

### 2.3 Capture protocol and delivery semantics

- Each collector creates an actor-scoped UUID capture ID before execution and persists the completed envelope locally until the server acknowledges it.
- Upload is idempotent under `(engagement_id, actor_id, source_agent_id, capture_id)`. A retry returns the original action ID; a different payload for the same key is rejected and audited.
- stdout and stderr remain distinct evidence blobs. Uploads stream through bounded temporary files while hashing; the final database references are committed only after durable blob placement. Interrupted temporary files are reclaimed.
- Timestamps preserve collector wall-clock plus server receipt time and measured duration. Clock skew is surfaced, not silently rewritten.
- Metadata and output limits are explicit and return retryable/non-retryable errors. Collectors retain data until a durable acknowledgement, including after process or host restart.
- SSE uses persisted monotonic `audit_event.id` values as event IDs and honors `Last-Event-ID`; clients resync through paginated REST when history is outside the retention window.
- MCP exposes only narrow, authenticated ingestion/status tools and delegates to the same application service and authorization checks as REST. It is not a second write path.

### 2.4 Attribution and egress

Tokens are random, actor-specific, shown once, stored only as non-reversible digests, scope-bound to an engagement and role, revocable, and never accepted in query strings. AI actors require a human `authorized_by`, model/name/version, `initiated_by=ai`, and decision context for AI command captures. Anonymous/shared/default tokens do not exist.

`exec_host_ip` means the collector-selected source interface address and includes collection method/uncertainty; it is never inferred solely from the HTTP peer. Pivot hops are ordered, typed, and validated. Egress mode is instance startup configuration:

- `auto`: resolve only through the configured endpoint, cache with observed time, and expose resolution failures;
- `manual`: require configured declared values and make no discovery call;
- `off`: make no discovery call and record an explicit `disabled` status with a null egress value.

The PRD phrases “attacker IP always known” but also permits `off`. The executable rule is therefore: exec-host attribution and egress **status** are always present; egress value may be null only when disabled or resolution failed, and the UI/report must make that gap visible. Capture is not dropped on auto-resolution failure.

### 2.5 Export and teardown

Export first freezes a consistent PostgreSQL snapshot and report-input JSON, then creates the PDF, evidence tree, versioned custom-format DB dump, restore/verify tools, and metadata. The manifest hashes every payload file with path, byte length, and SHA-256. A manifest cannot hash itself without recursion, so “every artifact” is defined as every bundle payload **except the manifest itself**; an outer archive SHA-256 is emitted separately. Manifest schema includes an empty `signatures` extension point, but v1 does not claim a signature.

A clean-room acceptance test restores the dump and regenerates/verifies the report without the live instance. The supported `waypoint destroy` flow requires the path and successful local verification of a completed export, then removes application/database/evidence volumes. `--force` is an explicit, interactive break-glass path and is itself documented as bypassing preservation; direct Docker volume deletion cannot technically be prevented and is part of the documented boundary.

### 2.6 Supported environments

- Compose: current Docker Engine + Compose v2 on Linux, macOS, and Windows hosts.
- Install script: Ubuntu Server 22.04 and 24.04 LTS, x86_64, idempotent and non-interactive after validated configuration.
- Wrapper: current supported Windows x86_64, Linux x86_64/arm64, macOS x86_64/arm64.
- Offline agent: Linux x86_64/arm64 and macOS x86_64/arm64. Windows is fast-follow.
- Browsers: current and previous major Chrome, Firefox, Edge, and Safari.

Other platforms may work but are not release gates.

### 2.7 UX interpretation

Recon, Attacks, Findings, Journey log, and Summit are always keyboard-accessible destinations. “Fog” communicates an empty area, never denied navigation. Completion indicators summarize data state and do not gate phases. The Journey log uses security-meaningful prose and always retains precise technical details. The map is atmosphere/navigation; tables/forms are clean work surfaces. Parchment/map colors remain fixed artifacts in dark mode while surrounding chrome themes.

Static guide notes are reviewed repository content. Technique notes explain what/when/risks and link to applicable entities/techniques; exact guided commands remain v2 scope. Alerts are deterministic plugin-output rules in v1 (initially successful authentication and first observation of a newly reachable segment), link to their source action, deduplicate, and never imply AI analysis.

## 3. Security and trust boundaries

| Boundary | Required control |
|---|---|
| Browser / API | TLS outside loopback; secure headers; strict origin policy; short-lived secure account session or actor bearer token; role and engagement checks on every request; no secrets in URL/log/SSE. |
| Actor / engagement | Server derives actor and engagement from credential; body IDs cannot override them. Human authorization is enforced for AI creation. Revocation takes effect on REST, MCP, and SSE reconnect. |
| Collector / ingest | Version negotiation, idempotency, strict schemas, size/rate limits, streaming hashes, clock-skew flag, and no server-side shell interpretation. argv is an array, never reconstructed for execution. |
| Parser / core | Only release-pinned plugins; subprocess resource limits; no engagement-time code upload; JSON Schema validation; raw evidence retained when parsing fails. Parser failure cannot reject raw capture. |
| Untrusted assessment data / UI | Contextual output escaping, text-by-default rendering, explicit safe media previews, CSP, filename normalization, CSV-formula neutralization, and no raw HTML from tools/plugins/notes. |
| Evidence filesystem / DB | Content hash verification, restrictive permissions, atomic writes, no user-controlled paths, engagement authorization on reads, encrypted transport, and host/disk encryption called out as deployment responsibility. |
| SSE / concurrency | Authorized filtered stream, heartbeat, bounded connection/output buffers, persistent cursor, optimistic versions on editable records, transactionally emitted events. Slow clients cannot block ingest. |
| MCP | Same actor tokens/scopes, schemas, rate limits, idempotency, and audit service as REST; no generic shell or arbitrary SQL/filesystem tool. |
| Egress discovery | Exactly zero discovery traffic in manual/off; configured allowlisted destination in auto; timeout/cache/status; no caller-supplied URL (SSRF prevention). |
| Export / import | Canonical safe relative paths, no symlinks/traversal, file count/size limits, escaped report rendering, deterministic snapshot, manifest verification before restore, sensitive-data warning. |
| Provisioning / operations | No built-in shared credential; one-time bootstrap; secret files mode 0600; redacted logs; revocation/rotation; guarded teardown after verified export. |

Residual boundaries must be explicit in operator documentation: commands run wholly outside a wrapper/agent/API/MCP cannot be guaranteed captured; host administrators can read unencrypted mounted data or bypass guarded teardown; `egress=off` intentionally omits public source attribution; v1 manifests detect later changes but do not prove signer identity.

## 4. Performance budgets and data profile

All performance gates run in a release-like Compose stack on a documented 4 vCPU / 8 GiB RAM Linux baseline with 10 concurrent operators, 100,000 actions, 1,000,000 audit events/observations, and 10 GiB evidence:

- paginated list/filter API p95 <= 200 ms and p99 <= 500 ms for indexed common queries;
- metadata ingest acknowledgement p95 <= 500 ms after request body arrival; evidence upload remains streaming with bounded memory (<= 32 MiB incremental RSS per upload);
- committed action visible through SSE p95 <= 1 s;
- primary route usable <= 2 s on a warm local network; route changes/interaction response <= 100 ms excluding network;
- no unbounded result endpoint; keyset pagination is default; query plans for common filters are regression-tested;
- export has progress/cancellation, does not block capture, and completes the baseline 10 GiB bundle within 15 minutes with enough free-space preflight.

These are release budgets, not aspirational metrics. Any changed budget is an explicit architecture/PRD decision.

## 5. Test strategy

### 5.1 Test layers

1. **Unit/property tests**: normalization/dedup precedence, authorization matrix, token handling, manifest canonicalization, alert rules, redaction, path safety, capture state machine, retry/backoff, and time/skew logic.
2. **Contract tests**: versioned OpenAPI/JSON Schema fixtures shared across repos; valid/invalid capture envelopes; plugin output and raw fallback; SSE cursor behavior; MCP parity; report snapshot schema.
3. **Database integration tests**: real PostgreSQL migrations, constraints, tenant isolation, concurrent idempotent ingest, optimistic edits, merge/split reversibility, append-only protections, and indexed plans.
4. **End-to-end tests**: Compose setup, provisioning, two-human and AI attribution, known/unknown tool capture, offline replay, reconnecting SSE, finding promotion, export/verify/restore, and guarded teardown.
5. **Security tests**: cross-engagement IDOR matrix, revoked tokens, XSS payload corpus, malicious filenames/archive paths, oversized/truncated uploads, duplicate-ID conflict, parser timeout/crash, SSRF controls, no-egress packet assertion, dependency/container scans, and secret/log scans.
6. **Performance/soak tests**: baseline profile above, concurrent capture/SSE, slow consumer, restart during upload/export, disk-full and PostgreSQL interruption recovery.
7. **Accessibility/visual tests**: automated axe/keyboard/reduced-motion checks plus the mandatory dogfooded UX gate in light and dark at desktop/mobile sizes.

Flaky tests are release failures, not retry-until-green. Fixtures contain synthetic assessment data only.

### 5.2 Mandatory per-task gates

A worker task is independently complete only when its focused tests pass, migrations/contracts/docs are updated, and its acceptance evidence is attached. UI tasks additionally require running the app, operating the flow, and comparing screenshots in both themes against the design references. Cross-repo tasks must pass the same versioned contract suite in both repositories before integration is accepted.

### 5.3 Release acceptance journey

On a clean host:

1. Start with `docker compose up`; provision two humans and one human-authorized AI actor without a shared token.
2. Capture a known and unknown command through supported wrappers. Verify attribution, source/egress status, timing, target, separate raw streams, parsed result/raw fallback, Journey log, and SSE updates.
3. Disconnect a Linux/macOS remote agent, capture, restart it, reconnect, and prove exactly-once replay with original timestamps.
4. In parallel browser sessions, use all phases non-linearly, merge then split an ambiguous entity, observe deterministic alerts, and verify actor-attributed history.
5. Promote an attack, complete required finding fields, and export. Verify every manifest entry and outer archive hash; restore into a clean environment and regenerate equivalent report content.
6. Run guarded teardown, prove volumes are gone and the exported bundle remains sufficient.
7. Complete automated security/performance/accessibility suites and dogfood every operator flow in light/dark desktop/mobile UI.

## 6. Sequencing and integration gates

### Phase 0 — contracts and skeleton

Freeze vocabulary, architecture records, compatibility policy, OpenAPI/event/plugin schemas, representative fixtures, and CI skeleton before parallel implementation. **Gate G0:** both repos can validate the same synthetic known-tool, unknown-tool, human, and AI capture fixtures; deferred features have no routes or UI claims.

### Phase 1 — durable and attributable foundation

Build Compose/install configuration, migrations, append-only audit primitives, evidence store, actor provisioning/auth, and engagement isolation. **Gate G1:** real-Postgres tests prove no anonymous or cross-engagement write/read and evidence survives service restart without orphaned committed references.

### Phase 2 — capture spine and live collaboration

Implement REST ingest, MCP adapter, idempotency, streaming evidence, raw fallback, SSE cursor, out-of-band flags, and collection wrapper/agent against G0 contracts. **Gate G2:** two human actors plus an AI actor round-trip; unknown/parser-failed tools remain captured; offline replay is exactly-once; manual/off egress makes no network discovery call.

### Phase 3 — recon intelligence and workspaces

Add parser validation, entities/observations, deterministic dedup/merge/split, alerts, expedition shell, Recon/Attacks workspaces, Journey log, and guide content. **Gate G3:** provenance remains intact through merge/split; 100k-action views meet budgets; all phases remain accessible when empty; desktop/mobile light/dark dogfood passes.

### Phase 4 — findings and defensible export

Add promotion/edit history, report snapshot/rendering, bundle/manifest/restore verification, export progress, and guarded teardown. **Gate G4:** a clean-room restore passes and finding/report evidence traces to immutable captures; manifest and outer hash verify; signing is clearly absent rather than implied.

### Phase 5 — release hardening

Complete supported-platform matrix, installer, fault/security/performance suites, operational docs, full acceptance journey, and final visual/accessibility review. **Gate G5:** all PRD trace rows are `Pass` with retained evidence, no critical/high vulnerabilities, no unresolved data-loss defect, and no unapproved v2 scope.

## 7. Small, independently verifiable worker tasks

Priorities: P0 blocks the release spine; P1 is required v1 capability; P2 is release hardening/documentation that may begin later but is still required for v1. `repo` names the owning repository; cross-repo dependencies are contract gates, not permission to edit another assigned worktree.

| ID | Title | Description and independent verification | Role | Repo | Priority | Depends on |
|---|---|---|---|---|---|---|
| V1-001 | Architecture and vocabulary records | Add ADRs for system shape, audit immutability, evidence storage, parser boundary, and export semantics; add a v1/deferred vocabulary check. Verify ADR/link lint. | architecture | Waypoint | P0 | — |
| V1-002 | Versioned API and event contracts | Define OpenAPI, capture/event schemas, error model, cursor/idempotency rules, and valid/invalid fixtures. Verify generated schema and compatibility tests. | API | Waypoint | P0 | V1-001 |
| V1-003 | Plugin interoperability contract | Consume core fixtures; define match/parse/schema/version behavior and known/unknown plugin fixture tests. | collection | Waypoint-Plugins | P0 | V1-002 |
| V1-004 | Core and web skeleton | Scaffold Go service, React/TS client, lint/test/build commands, embedded assets, health/readiness, and CI. Verify production build boots. | platform | Waypoint | P0 | V1-001 |
| V1-005 | Compose/config/egress modes | Add app + single PostgreSQL Compose stack, validated startup config, auto/manual/off resolver abstraction, and no-egress tests. | platform-security | Waypoint | P0 | V1-004 |
| V1-006 | Database and migration foundation | Implement engagement/actor/action/audit/entity/result/finding/evidence schema, constraints, indexes, migration/rollback policy. Verify on real PostgreSQL. | data | Waypoint | P0 | V1-002, V1-004 |
| V1-007 | Actor provisioning and authorization | Implement one-time bootstrap, human/AI provisioning, token digest/revoke/roles, AI human authorization, engagement isolation matrix. | security | Waypoint | P0 | V1-006 |
| V1-008 | Append-only audit service | Transactionally record actor-attributed events and reject mutation/deletion; add correlation/redaction rules. Verify concurrent writes and DB protections. | backend | Waypoint | P0 | V1-006, V1-007 |
| V1-009 | Content-addressed evidence store | Stream/hash/atomically place stdout, stderr, and evidence; deduplicate safely, enforce quotas, recover temp/orphan files. | backend-security | Waypoint | P0 | V1-006 |
| V1-010 | Idempotent REST capture | Build capture ingestion with versioning, actor-derived scope, conflict detection, skew/status metadata, raw fallback, and transactionally linked evidence/audit. | backend | Waypoint | P0 | V1-002, V1-007, V1-008, V1-009 |
| V1-011 | Resumable SSE feed | Add filtered authorized SSE with persistent IDs, Last-Event-ID, heartbeats, bounded queues, and REST resync. | backend | Waypoint | P0 | V1-008, V1-010 |
| V1-012 | MCP ingestion adapter | Expose narrow capture/status MCP tools through the same service; prove REST/MCP authorization, idempotency, and audit parity. | integration-security | Waypoint | P1 | V1-010 |
| V1-013 | Cross-platform operator wrapper | Capture argv/cwd/times/source/pivot/stdout/stderr/exit locally, spool until ack, and pass contracts on Windows/Linux/macOS. | collection | Waypoint-Plugins | P0 | V1-003, V1-010 |
| V1-014 | Offline remote agent | Add durable queue, restart-safe backoff/replay, limits, and Linux/macOS builds. Verify disconnect/capture/restart/reconnect exactly once. | collection | Waypoint-Plugins | P0 | V1-003, V1-010 |
| V1-015 | Parser runner and raw fallback | Match by binary then specificity, run pinned parsers with limits, validate output, preserve raw on no match/crash/invalid output. | collection-security | Waypoint-Plugins | P0 | V1-003, V1-013 |
| V1-016 | Structured result validation | Register compatible plugin schemas and ingest/link structured results without trusting parser content. Verify invalid output cannot corrupt/drop raw action. | backend | Waypoint | P0 | V1-003, V1-010 |
| V1-017 | Entity provenance and dedup | Implement normalized identifiers, observations, stable-key precedence, conflict handling, and concurrency tests. | data | Waypoint | P1 | V1-006, V1-016 |
| V1-018 | Audited merge and split | Implement preview, optimistic conflict check, reversible merge lineage, observation-based split, and actor-attributed events. | backend | Waypoint | P1 | V1-008, V1-017 |
| V1-019 | Out-of-band review queue | Flag imported/entity/result claims lacking a valid captured source action; allow audited resolution and document detection limits. | backend | Waypoint | P1 | V1-016, V1-017 |
| V1-020 | Deterministic notable alerts | Implement deduplicated successful-auth/new-segment rules linked to source captures and SSE. | backend | Waypoint | P1 | V1-011, V1-016, V1-017 |
| V1-021 | Design tokens and expedition shell | Codify palette/themes; build accessible non-linear map/list navigation, responsive chrome, reduced motion, and route skeleton. Dogfood in both themes/sizes. | frontend-UX | Waypoint | P1 | V1-004 |
| V1-022 | Journey log workspace | Build keyset-paginated live audit feed with actor/source/result details, reconnect/resync, filters, and safe output handling. | frontend-UX | Waypoint | P1 | V1-011, V1-021 |
| V1-023 | Recon workspace | Build empty/fog and populated Recon views, entity provenance, merge/split UI, alerts, and accessible tables. | frontend-UX | Waypoint | P1 | V1-018, V1-020, V1-021 |
| V1-024 | Attacks workspace | Build grouped/filterable attempts by technique/target/host, raw/structured evidence drill-in, status, and live updates. | frontend-UX | Waypoint | P1 | V1-016, V1-021, V1-022 |
| V1-025 | Static guide content system | Add reviewed phase notes and keyed technique explainers with safe rendering/search/context links; exclude v2 command guidance/AI claims. | content-UX | Waypoint | P1 | V1-017, V1-021 |
| V1-026 | Finding promotion and revisions | Promote an attack manually with evidence auto-links, required report fields, optimistic edits, statuses, and complete actor-attributed revision history. | full-stack | Waypoint | P0 | V1-008, V1-024 |
| V1-027 | Findings workspace | Build finding list/editor/evidence trace with guide-style validation, concurrent-edit handling, and always-accessible empty state. | frontend-UX | Waypoint | P1 | V1-021, V1-026 |
| V1-028 | Frozen report snapshot and PDF | Create versioned deterministic report input and print route; render escaped PDF with scope, methodology, findings, evidence, attribution/gaps. | reporting | Waypoint | P0 | V1-026 |
| V1-029 | Bundle, manifest, verify, restore | Stream consistent DB dump/evidence/PDF/snapshot, manifest payload files, emit outer hash/signature hook, add clean-room verify/restore/regenerate test. | reporting-platform | Waypoint | P0 | V1-009, V1-028 |
| V1-030 | Summit and guarded teardown | Add export progress/failure recovery, Summit UI, verified-export receipt, destroy guard and explicit break-glass flow. | full-stack-platform | Waypoint | P0 | V1-021, V1-029 |
| V1-031 | Supported-host installer/provisioning | Build idempotent Ubuntu 22.04/24.04 installer, account provisioning path, upgrades/config validation, and clean VM test. | platform | Waypoint | P1 | V1-005, V1-007 |
| V1-032 | Performance and fault suite | Seed baseline profile; enforce query/SSE/ingest/UI/export budgets; test disk-full, restart, slow client, interrupted upload/export. | performance | Waypoint | P0 | V1-011, V1-018, V1-024, V1-029 |
| V1-033 | Security hardening suite | Run auth/IDOR/XSS/archive/SSRF/quota/parser/log/dependency/container tests and document at-rest/admin residual risks. | security | Waypoint | P0 | V1-012, V1-019, V1-029, V1-031 |
| V1-034 | Platform collection matrix | Run wrapper matrix on Windows/Linux/macOS and agent matrix on Linux/macOS, including AI attribution and flaky networks; retain artifacts. | QA-collection | Waypoint-Plugins | P0 | V1-013, V1-014, V1-015 |
| V1-035 | Full UX/accessibility dogfood | Drive all flows desktop/mobile, light/dark, keyboard/screen-reader/reduced-motion; capture and compare screenshots to references. | QA-UX | Waypoint | P0 | V1-022, V1-023, V1-025, V1-027, V1-030 |
| V1-036 | Operations and boundary docs | Document setup/config/TLS, actors/MCP, backup/export/verify/restore/destroy, supported matrix, out-of-band/no-egress/signing/admin boundaries. | docs-security | Waypoint | P1 | V1-019, V1-029, V1-031, V1-034 |
| V1-037 | Clean-host release acceptance | Execute the complete acceptance journey, reconcile traceability evidence, and prove no v2 claims/routes. | release | Waypoint | P0 | V1-030, V1-032, V1-033, V1-034, V1-035, V1-036 |

## 8. Integration rules for parallel workers

- Land contracts and fixtures before implementation. Consumers pin a contract version; breaking changes require a compatibility window or coordinated gate update.
- Database migrations are forward-only after a release artifact is published. During v1 development, down tests are allowed, but destructive changes require fixture migration tests.
- A task owns only its listed surface. Shared schema/design-token changes go through their owning task rather than being copied into feature branches.
- Every write uses the application audit transaction; workers must not create “temporary” unaudited mutation routes.
- UI fixtures must use synthetic data and exercise empty, loading, error, partial, large, and concurrent-change states.
- Cross-repo acceptance is artifact-based (schemas, fixtures, binaries, test results). Workers do not assume access to or modify a sibling repository outside their assigned worktree.

## 9. Release stop conditions

Stop and return to design if any implementation requires anonymous/shared attribution, drops raw output because parsing failed, performs an external call in manual/off egress mode, makes a phase inaccessible because it is empty, permits report evidence without provenance, exports without a verified payload manifest, or introduces a v2 graph/LLM/scan-library dependency. Stop release for any reproducible data loss, cross-engagement access, critical/high unmitigated vulnerability, failed clean-room restore, unsupported required platform, or missing light/dark dogfood evidence.
