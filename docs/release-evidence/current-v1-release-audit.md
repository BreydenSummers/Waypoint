# Current merged v1 release audit

Run: `2026-08-24T08:11:44Z–08:16:50Z`  
Task: `current-v1-001-release-audit`  
Verdict: **Fail — not v1-ready (7 Pass, 15 Fail, 45 Unverified).**

This is a fresh fail-closed audit of the current core tree after the remediation work. It does not
adopt the pre-remediation conclusions in `final-v1-acceptance.md`. Source now includes substantial
PostgreSQL, evidence streaming, actor lifecycle, MCP, report, export-job, teardown-authorization,
and Summit integration work. That implementation is recorded below, but implementation is not the
same thing as retained release evidence.

G5 remains closed: all 67 applicable PRD rows must be Pass, while 60 are Fail or Unverified.
Unavailable services, skipped tests, source-shape checks, and synthetic fixtures were never counted
as Pass.

## Audit boundary

The assigned tree is the core repository only. Its `.git` worktree metadata points outside the
sandbox, so even core history/status is unavailable:

```sh
git status --short
# fatal: not a git repository: /opt/waypoint-project/Waypoint/.git/worktrees/core-current-v1-001-release-audit
# exit 128
```

The sandbox does not permit entering the sibling `Waypoint-Plugins` worktree. No current retained
plugins compatibility report, parser fixture report, wrapper/agent transcript, spool replay log,
release-binary matrix, or native platform run exists in this core tree. All cross-repository and
collector claims are therefore Unverified, not inferred from core contracts.

Host limits observed during this run:

- `WAYPOINT_TEST_PG_DSN` and `WAYPOINT_DB_DSN` were unset;
- `psql`, `postgres`, `initdb`, `pg_dump`, `pg_restore`, and `pg_isready` were unavailable;
- Docker/Compose clients were installed, but the Docker daemon was unavailable;
- Chrome/Chromium, Firefox, Edge, Safari, Playwright, Puppeteer, Selenium, and axe were unavailable.

No current retained clean PostgreSQL, Compose, supported-host VM, native collector, browser,
clean-room restore, security-scan, or measured performance artifact was found. Historical evidence
files remain useful context only; their blocked attempts are not current runtime proof.

## Status method

- **Pass:** the whole observable PRD row is established by direct, repeatable evidence available to
  this audit.
- **Fail:** current source contradicts the row, a required implementation is absent/broken, or a
  mandatory release artifact is demonstrably synthetic or invalid.
- **Unverified:** relevant implementation may now exist, but required real-PostgreSQL, collector,
  platform, browser, clean-host, or clean-room evidence is unavailable.

## Exact command ledger

Commands were run from the repository root (`/workspace`).

| Command | Exit/result | What it proves — and does not prove |
|---|---|---|
| `python3 scripts/verify-contracts.py` | 0; 30 schemas, 2 generated artifacts, 690 OpenAPI refs and all listed fixture inventories passed | Core contract consistency only; no plugins/runtime compatibility. |
| `python3 scripts/lint-architecture.py` | 0; final rerun: 9 ADRs, 9 scope rules, 162 claim-surface files | Narrow architecture/deferral claim scan only. |
| `go vet ./...` | 0 | Static Go vet only. |
| `make lint` | 0; Go vet plus the web source/fixture checker passed | Official lint target; still not runtime evidence. |
| `go test -count=1 -json ./...` | 0; 42 top-level Pass, **47 top-level Skip**, 20 passing subtests, 0 executed-test Fail | Available unit/source tests pass, but every real-PostgreSQL path and Compose runtime skipped. A green process exit is not a green release gate. |
| `npm --prefix web test` | 0; 9 tests | Source/fixture/temp-directory checks; no browser and no production export. |
| `make test` | 0 at process level; repeated the same skip-capable Go suite and 9 synthetic/source web tests | Official test target is green despite unavailable release-critical PostgreSQL paths. |
| `npm --prefix web run build` | 0 | Deterministic web asset generation succeeded. |
| `mkdir -p .audit-bin && go build -o .audit-bin/waypoint ./cmd/waypoint` | 0 | Current binary compiles. |
| `WAYPOINT_ADDR=127.0.0.1:18082 .audit-bin/waypoint` | 1; `WAYPOINT_DB_DSN is required` | Startup correctly fails closed without a DB; no live app journey. `.audit-bin` was removed afterward. |
| `docker compose -f compose.yml config --quiet` | 0 | Compose syntax only. |
| `docker compose -f compose.yml build --no-cache` | 1; daemon unavailable | Build/runtime Unverified. |
| `docker compose -f compose.yml up -d --wait` | 1; daemon unavailable | Clean stack Unverified. |
| `go test -count=1 -v -run '^TestComposeStackPersistsDBAndEvidenceAcrossRestart$' .` | 0 only because the test **SKIP**ped when `docker info` failed | Silent-green skip; not Compose evidence. |
| `python3 scripts/g1-foundation-report.py --output /tmp/current-v1-g1.md` | 2; `WAYPOINT_TEST_PG_DSN is required` | G1 correctly fails closed and emitted no report. |
| `make smoke` | 2 after its 30-attempt curl retry loop; app exited for missing DSN | Not a hang, but a delayed failure. The generated `bin/` was removed after the attempt. |

## Go test disposition

`go test -count=1 -json ./...` produced 42 top-level passes and these 47 top-level skips:

- Compose/startup: `TestComposeStackPersistsDBAndEvidenceAcrossRestart`,
  `TestOpenConfiguredDatabaseAppliesMigrations`.
- Migrations/audit: `TestAppendAuditEventCapturesOutOfBandReviewLifecycle`,
  `TestAppendAuditEventRedactsSensitiveMetadata`,
  `TestAppendAuditEventCommitsConcurrentlyAndRollsBackCleanly`,
  `TestAuditEventViewAndTableRemainAppendOnly`, `TestApplyMigrationsOnRealPostgreSQL`,
  `TestApplyMigrationsSerializesConcurrentStarters`, `TestDatabaseProtectionsRejectMutations`,
  `TestActorAuthorizationConstraint`, `TestActionAttributionSchemaRoundTrip`.
- Actors/capture/evidence: `TestActorLifecycleProvisionRotateRevokeAndAuthorization`,
  `TestCaptureRoundTripGateG2Transcript`, `TestLiveMultiActorRESTCaptureJourneys`,
  `TestCaptureIngestCreatesReplaysAndRejectsChangedPayload`,
  `TestCaptureRejectsUnsupportedContractVersion`,
  `TestCapturePersistsEvidenceAndRecoversOrphans`,
  `TestCaptureEvidenceDeduplicatesAndSurvivesRestart`,
  `TestCaptureAcceptsAIInitiationWithDecisionContext`,
  `TestCapturePersistsStructuredResultsAndRollsBackInvalidOutput`,
  `TestCaptureRejectsConflictingStableEntityKinds`,
  `TestEvidenceMetadataAndContentReadsStayEngagementScoped`.
- SSE/workspaces/entities/findings/alerts: `TestAuditHistoryPaginationSSEReconnectFilteringAndRevocation`,
  `TestAuditEventsCursorExpiredReturnsResyncLink`, `TestTailAuditEventsStopsWhenQueueIsFull`,
  `TestAuditSSEHeartbeatAndCommittedCaptureVisibility`,
  `TestReconReadApisAreKeysetPaginatedAndEngagementIsolated`,
  `TestReconPreviewAndSplitProvenanceReadsFollowCanonicalLineage`,
  `TestEntityMergeSplitPreviewUndoAndProvenance`,
  `TestEntityIdentityNormalizationConflictAndConcurrentDeduplication`,
  `TestEntityReadProvenanceTracksCanonicalLineageOnRealPostgreSQL`,
  `TestEntityMergeConflictIsOptimisticUnderConcurrency`,
  `TestFindingPromotionRevisionsAndOperatorOnlyPromotion`,
  `TestG3AuthoritativeProvenanceJourney`, `TestNotableAlertsAreDeduplicatedAndStreamed`,
  `TestNotableAlertsUseSystemActorForAISourceCaptures`.
- MCP/claims/report/export/faults: `TestMCPStandardFlowReusesCaptureService`,
  `TestOutOfBandClaimLifecycleThroughPostgreSQL`,
  `TestReportHandlerRequiresAuthAndScopesEngagement`,
  `TestExportJobLifecyclePersistsReceiptAndBlocksBrowserAuthorship`,
  `TestExportJobPreflightRejectsInsufficientCapacity`,
  `TestExportTeardownAuthorizationRoundTrip`,
  `TestBuildExportEvidenceTarStreamsAttachmentRoles`,
  `TestExportJobListIsPagedAndResumable`,
  `TestUpsertEvidenceRejectsImmutableMetadataChanges`,
  `TestCaptureIngestRejectsDiskPressureBeforeCommitting`,
  `TestCaptureIngestRetriesAfterInterruptedUploadWithoutDuplication`.

The test helper now skips these gates when the DSN is absent, so the aggregate Go command exits zero.
The fail-closed G1 reporter refuses to run without a DSN, which is the authoritative G1 disposition.

## Synthetic and source-only gates

These checks are useful development guards but are not release evidence:

1. `web/scripts/bundle-export.test.mjs` stages literal `postgres dump bytes`, `evidence bytes`, and a
   fake `%PDF` in a temporary tree. Its “concurrent capture” is a separate temporary file write.
2. Report PDF Go tests install a shell script as fake Chromium and emit a fake PDF header. No shipped
   Chromium rendered an authoritative report in this run.
3. Installer tests replace systemd/PostgreSQL commands and OS identity files. Their Ubuntu 22.04 and
   24.04 cases are synthetic, not clean VM runs.
4. UX tests grep `App.tsx`, CSS, and generated JavaScript. No keyboard, accessibility-tree, axe,
   screen-reader, responsive viewport, light/dark screenshot, or reduced-motion browser run exists.
5. `TestPerformanceProfileFixtureSeedsBaselineAndFaultScenarios` validates configured constants;
   `TestAuditQueryShapeRemainsKeysetBounded` inspects source strings; and the RSS test streams 8 MiB
   through a logical 10 GiB limit. None measures the PRD baseline workload or latency percentiles.
6. Core contract fixtures prove shape/drift only. They cannot prove parser selection/isolation,
   collector spool durability, native OS behavior, or REST/MCP persistence against PostgreSQL.

No indefinitely hanging test was observed. `make smoke` spends roughly 30 seconds retrying after the
binary has already exited, and the focused Compose test exits zero after a skip; both behaviors can
look like a gate ran when it did not.

## Confirmed current source defects

These findings are against the current remediation state, not copied from the older acceptance:

1. **Stored credential digests remain bearer credentials.** `lookupActor` in
   `internal/server/capture.go` prepends a supplied 64-hex token directly to the lookup candidates.
   Possession of the database digest is therefore sufficient to authenticate. This blocks
   PRD-ID-001 and the security gate.
2. **Startup egress policy remains absent.** `cmd/waypoint/main.go` opens PostgreSQL and starts the
   handler but does not parse/validate `WAYPOINT_EGRESS_MODE`, resolve an allowlisted auto endpoint,
   or configure manual/off behavior. The installer merely copies egress variables into an env file.
   No zero-discovery-traffic packet assertion exists.
3. **The export “database dump” is still partial JSON.** `buildExportDump` serializes only a report
   engagement projection, actions, and findings. It is not a PostgreSQL custom-format dump and omits
   the complete actor, audit-event, entity/observation/result, evidence metadata, claim, revision,
   export, receipt, and authorization state required for reconstruction.
4. **Production export verification is internally inconsistent.** The archive is written before
   `bundle/metadata/export-archive.sha256`; `verifyExportBundle` later walks the bundle directory and
   requires every current file in the archive, so the sidecar is missing from the archive. The
   report-snapshot payload also contains a self-referential hash placeholder/update cycle. The
   real-DB export tests that could expose the lifecycle all skipped.
5. **Bundled restore tools are not self-contained.** Generated scripts import
   `../../web/scripts/bundle-tools.mjs`, which is not among the production manifest payloads. The
   so-called `engagement.dump` has no database restore implementation. A clean-room restore after
   wipe cannot be performed from the emitted bundle alone.
6. **Supported teardown is not end-to-end guarded.** The server now persists and consumes a
   short-lived authorization, and Summit now calls those APIs. However, the installer `destroy`
   path only checks receipt status and matching path text; it neither hashes the archive/manifest nor
   consumes the server authorization before deleting roots. The browser can consume an authorization
   without performing an external wipe.
7. **Report attribution is still lossy.** The report query/output does not preserve egress
   mode/status/observation, pivot chain, source-agent details, execution failure semantics, or all
   evidence metadata; null egress is collapsed to “not recorded.” No authoritative real-Chromium PDF
   or frozen finding-to-evidence transcript is retained.
8. **Runtime security controls remain incomplete.** There is no TLS-outside-loopback enforcement or
   global CSP/HSTS/frame/nosniff/origin/rate-limit middleware. Several write handlers authenticate but
   do not consistently enforce the owner/operator/viewer matrix. No retained IDOR, XSS, SSRF,
   archive, dependency, container, or secret/log scan exists.
9. **Operator residual-boundary documentation is absent.** The core docs are PRD, ADR, planning, and
   release evidence. There is no operator guide stating that wholly out-of-band human/AI activity
   cannot be guaranteed captured or documenting the host-admin, unsigned-hash, egress-off, and
   host/disk-encryption boundaries.

The report/export/teardown defects are preservation defects: a successful supported wipe cannot be
shown to leave a complete reconstructable audit record.

## Deployment and host gates

Compose now declares one app container and one PostgreSQL 16 container, with separate PostgreSQL and
evidence volumes; the production binary now requires PostgreSQL, pings it, and applies migrations.
The installer now contains local PostgreSQL bootstrap, provisioning, rollback, and diagnostics
paths. These are substantive remediations.

They remain Unverified as deployment claims because Docker could not build/start, no supported Ubuntu
VM was run, no migrations were inspected on a live database, no named-account lifecycle was driven,
and no stack restart/volume persistence or clean teardown was observed. Compose syntax and synthetic
installer tests are not clean-host evidence.

## Deferral inventory

The direct source/dependency/copy inventory is sufficient for the seven negative scope claims. The
current architecture lint passed over 162 claim-surface files. The current core tree has:

- no analytical node/attack-path graph or graph database dependency;
- no network-zone/firewall reachability map;
- no guided scan-library catalog/trigger surface;
- no offensive model provider/call or live-insight service;
- no AI finding prefill/autopublish path;
- no runtime plugin-generation/upload path; and
- no signing key, signer identity, or cryptographic signature claim.

Reserved compatibility vocabulary and the v1 AI-actor ingestion infrastructure do not implement a
v2 feature. PRD-DEF-001–007 are the only Pass rows.

## All 67 PRD rows

| ID | Status | Current implementation versus retained evidence |
|---|---|---|
| PRD-CORE-001 | Unverified | Core now streams raw evidence to content-addressed files and has retry/orphan tests; all PostgreSQL capture tests skipped and no collector spool/parser-crash/restart transcript is retained. |
| PRD-CORE-002 | Unverified | Append-only schema/audit and actor constraints exist; every real-DB immutability/concurrency journey skipped. |
| PRD-CORE-003 | Unverified | Authoritative actions/entities/findings APIs and phase workspaces exist; the end-to-end three-phase provenance journey skipped and no operator recording exists. |
| PRD-CORE-004 | Unverified | UI/source models tried/result/empty states; no running operator journey proves failures are retained while “left” remains visible. |
| PRD-DATA-001 | Unverified | Compose declares one PostgreSQL service and no graph/object-store service; topology and startup were not run. |
| PRD-DATA-002 | Unverified | Four migrations and lossless action columns exist; all migration/constraint/round-trip tests skipped. |
| PRD-DATA-003 | Unverified | Evidence streaming/content addressing is implemented; DB-scoped dedup, restart, orphan, isolation, and fault tests skipped. |
| PRD-DATA-004 | Unverified | Stable-key normalization source/unit checks exist; real-DB conflict/concurrency tests skipped. |
| PRD-DATA-005 | Unverified | Merge/split/preview/undo/provenance APIs exist; all PostgreSQL and operator-flow evidence is unavailable. |
| PRD-DATA-006 | Unverified | Keyset APIs, optimistic revisions, and cursor SSE exist; DB concurrency tests skipped and no two-browser run is retained. |
| PRD-AUD-001 | Unverified | Authoritative action reads now expose the full accepted envelope; no collector → PostgreSQL → report field transcript ran, and report projection remains lossy. |
| PRD-AUD-002 | Unverified | Human/AI validation paths exist; parity capture tests skipped and no plugins collector transcript exists. |
| PRD-AUD-003 | Unverified | AI authorizer/model/version and decision-context validation exist; live provisioning/capture tests skipped. |
| PRD-AUD-004 | Unverified | DB protections and revisions exist in source; immutable reconstruction tests skipped. |
| PRD-CAP-001 | Unverified | No wrapper binary/execution matrix is retained or inspectable. |
| PRD-CAP-002 | Unverified | Core accepts `needs-plugin`, `raw`, and parser-failure states while retaining evidence; no real unknown/parser-crash collector round trip exists. |
| PRD-CAP-003 | Unverified | Core contracts pass and one pinned nmap schema is registered; no plugins compatibility, matching, isolation, or fixture report is retained. |
| PRD-CAP-004 | Unverified | No Windows/Linux/macOS wrapper binaries or native runs are retained. |
| PRD-CAP-005 | Unverified | No Linux/macOS agent disconnect, process restart, reconnect, or timestamp-preserving replay artifact is retained. |
| PRD-CAP-006 | Unverified | Core makes no Windows-agent v1 claim; plugins inventory and required Windows-wrapper evidence are unavailable. |
| PRD-CAP-007 | Unverified | Versioned idempotent REST ingest exists; all live PostgreSQL/collector conformance tests skipped. |
| PRD-CAP-008 | Unverified | Standard JSON-RPC MCP initialize/list/call source now delegates ingest to the REST handler; parity test skipped and no interoperable client transcript exists. |
| PRD-CAP-009 | Unverified | Pending/list/link/dismiss claim source exists; PostgreSQL lifecycle test skipped and no review journey is retained. |
| PRD-CAP-010 | Fail | No operator-facing residual-boundary guide states that wholly out-of-band execution cannot be guaranteed captured. |
| PRD-CAP-011 | Unverified | Server idempotency exists; no collector ack-loss, durable spool, process restart, or exactly-once replay artifact exists. |
| PRD-ID-001 | Fail | Per-actor lifecycle APIs exist, but `lookupActor` still accepts the stored 64-hex digest itself as a bearer credential; no clean two-operator journey exists. |
| PRD-ID-002 | Unverified | AI actor constraints and lifecycle source exist; real-DB authorization/revocation tests skipped. |
| PRD-NET-001 | Unverified | Core persists exec-host method/interface/confidence; no multi-interface collector source-selection run exists. |
| PRD-NET-002 | Unverified | Core persists egress status/observation and ordered pivots; no collector round trip proves them. |
| PRD-NET-003 | Fail | The app has no startup auto/manual/off policy/resolver and no manual/off zero-traffic packet evidence. |
| PRD-RT-001 | Unverified | REST writes and persisted-cursor SSE source exist; PostgreSQL reconnect/revocation tests skipped and no browser run exists. |
| PRD-RT-002 | Unverified | Deterministic alert rule units pass; real-DB dedup/SSE tests skipped and no live alert was observed. |
| PRD-FIND-001 | Unverified | Manual promotion/evidence-link source exists; PostgreSQL permission/validation test skipped and no browser promotion flow ran. |
| PRD-FIND-002 | Unverified | Finding/revision schema and APIs exist; persistence/history reconstruction was not run against PostgreSQL. |
| PRD-REP-001 | Fail | Report/PDF source exists, but attribution remains lossy and no authoritative PostgreSQL-to-real-Chromium frozen PDF is retained. |
| PRD-REP-002 | Fail | Production emits a partial JSON projection named `engagement.dump`; restore tools import absent repository files; no complete DB restore/regeneration after wipe is possible or retained. |
| PRD-REP-003 | Fail | Production archive ordering omits its subsequently-created hash sidecar, verification is internally inconsistent, and no valid production archive/manifest artifact exists. |
| PRD-REP-004 | Fail | A versioned empty hook and hash-only wording exist, but no valid production export establishes the complete unsigned flow. |
| PRD-REP-005 | Fail | Persisted job/preflight/cancel/recovery source exists, but production verification is broken and all real-DB/live-capture/export fault tests skipped. |
| PRD-LIFE-001 | Fail | Server authorization exists, but installer destroy verifies only receipt status/path text and never hashes exact bytes or consumes the server authorization; no post-wipe reconstruction ran. |
| PRD-DEP-001 | Unverified | Compose syntax passes and topology source is plausible; daemon unavailability prevented build/up/readiness/fresh-state/restart/teardown. |
| PRD-DEP-002 | Unverified | Installer source bootstraps local PostgreSQL and service state; only stubbed synthetic Ubuntu cases ran, not supported VMs. |
| PRD-DEP-003 | Unverified | Installer/API provisioning paths exist; no live provision/rotate/revoke journey is retained. |
| PRD-DEP-004 | Fail | Digest-as-bearer, missing TLS/secure middleware and inconsistent role enforcement remain; required scans and operator boundary guidance are absent. |
| PRD-UX-001 | Unverified | Current source/generated assets contain the expedition shell and clean workspaces; no running visual comparison exists. |
| PRD-UX-002 | Unverified | All route links are non-linear in source and fog copy is correct; empty-engagement keyboard/click navigation did not run. |
| PRD-UX-003 | Unverified | Journey log fetch/SSE integration exists; no audit-row parity/reconnect browser artifact exists. |
| PRD-UX-004 | Fail | Summit now uses persisted jobs/receipts/authorization APIs, but production export cannot verify and consuming authorization does not execute or bind the supported external wipe. |
| PRD-UX-005 | Unverified | Static phase notes exist; no offline running-UI dogfood exists. |
| PRD-UX-006 | Unverified | Technique-note content/search/context source exists; no browser context/search/link review is retained. |
| PRD-UX-007 | Unverified | Guide-style copy exists; invalid-flow browser/accessibility review is absent. |
| PRD-UX-008 | Unverified | Palette/theme tokens and a source-level contrast calculation exist; no browser contrast audit or light/dark screenshots exist. |
| PRD-UX-009 | Unverified | Reduced-motion CSS exists; no browser timing/playback evidence exists. |
| PRD-A11Y-001 | Unverified | Semantic links/buttons/focus/current-state source exists; no keyboard, accessibility-tree, axe, or screen-reader run exists. |
| PRD-A11Y-002 | Unverified | Responsive/min-size CSS is not computed-style, contrast, or mobile viewport evidence. |
| PRD-PERF-001 | Fail | No seeded baseline, query plans, or measured API p95/p99 report exists. |
| PRD-PERF-002 | Fail | Streaming implementation and an 8 MiB heap unit pass exist; no 10 GiB/concurrent ingest RSS or ack-latency measurement exists. |
| PRD-PERF-003 | Fail | No measured committed-action-to-SSE, warm-route, or local-interaction report exists. |
| PRD-QUAL-001 | Fail | Forty-seven release-critical tests skipped; platform, browser, security, measured performance, clean-room, and plugins gates are absent. |
| PRD-QUAL-002 | Unverified | `ux-dogfood.md` explicitly leaves every operator-flow checkbox open; no timestamped light/dark desktop/mobile screenshots exist. |
| PRD-DEF-001 | Pass | Current lint/source/dependency inventory shows no analytical graph surface or graph dependency. |
| PRD-DEF-002 | Pass | Current inventory shows no network-zone/firewall reachability map. |
| PRD-DEF-003 | Pass | Only rejected reserved compatibility vocabulary exists; no guided scan catalog/trigger UI exists. |
| PRD-DEF-004 | Pass | No model provider/call, offensive recommendation, or live-insight UI exists. |
| PRD-DEF-005 | Pass | Findings begin through explicit manual promotion; no AI prefill/autopublish surface exists. |
| PRD-DEF-006 | Pass | No runtime plugin generation/upload endpoint or UI exists. |
| PRD-DEF-007 | Pass | Hash-only wording and empty extension hook exist; no key management or signer-identity claim exists. |

## EV-01–EV-16 current disposition

| Evidence | Status | Current disposition |
|---|---|---|
| EV-01 | Unverified | Core contracts pass; plugins compatibility/parser/runtime evidence is outside the permitted boundary and not retained here. |
| EV-02 | Fail | All real-DB migration/auth tests skipped, and digest-as-bearer remains a definite credential defect. |
| EV-03 | Unverified | Core lossless capture source exists, but no human/AI/unknown-tool collector-to-PostgreSQL transcript exists. |
| EV-04 | Unverified | No offline replay or native wrapper/agent platform matrix exists. |
| EV-05 | Fail | Startup egress policy/resolver and packet assertions remain absent. |
| EV-06 | Unverified | Entity PostgreSQL tests skipped and no merge/split operator journey exists. |
| EV-07 | Unverified | SSE/concurrency PostgreSQL tests skipped and no two-browser recording exists. |
| EV-08 | Fail | No authoritative real-Chromium PDF trace exists and report attribution remains incomplete. |
| EV-09 | Fail | The production dump is partial JSON, verification is inconsistent, tools are not self-contained, and no clean-room restore exists. |
| EV-10 | Fail | Installer destroy neither verifies exact bundle bytes nor consumes server authorization; no post-wipe reconstruction exists. |
| EV-11 | Unverified | Compose and installer source exist; no daemon-backed clean stack or supported Ubuntu VM run exists. |
| EV-12 | Fail | No measured baseline performance/fault report exists. |
| EV-13 | Fail | Material auth/TLS/middleware/role/doc defects remain and no retained security scan/report exists. |
| EV-14 | Unverified | No running-app light/dark desktop/mobile screenshot/checklist set exists. |
| EV-15 | Unverified | No accessibility-tree, keyboard, axe, screen-reader, or reduced-motion browser artifact exists. |
| EV-16 | Pass | Current architecture lint/source inventory directly proves the seven narrow v2 deferrals. |

## Release decision

G5 **fails closed**. The current tree shows meaningful remediation, but no unavailable or synthetic
evidence has been promoted to Pass. Waypoint must not be labeled v1-ready, and the current export →
teardown path must not be represented as preservation-safe.
