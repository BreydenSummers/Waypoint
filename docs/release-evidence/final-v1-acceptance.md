# Final fail-closed v1 acceptance

Run: `2026-08-17T09:20:54Z`  
Verdict: **Fail — G5 is closed (7 Pass, 15 Fail, 45 Unverified).**

Waypoint is not v1-ready. G5 requires all 67 applicable PRD rows to be `Pass`; this run has 60
blocking rows. No unavailable runtime, skipped integration test, source inspection, fixture, or
synthetic host test was counted as a pass.

## Evidence boundary and acceptance method

The assigned core worktree was audited against the complete [`PRD.md`](../../PRD.md),
[`v1-execution-plan.md`](../v1-execution-plan.md), [`design-spec.md`](../design-spec.md), and
[`waypoint-mockup.html`](../waypoint-mockup.html). The sandbox contains no retained
Waypoint-Plugins compatibility report, collector transcript, spool replay log, parser report, native
release binaries, or platform matrix, and the security boundary prohibits entering a sibling
worktree.

The host has Docker/Compose clients but no daemon, PostgreSQL tools/service/DSN, browser binary, or
browser automation/accessibility runtime. These are release blockers, not waivers:

- `docker compose config --quiet` passes, but build/up and the Compose runtime test cannot run;
- `psql`, `postgres`, `initdb`, `pg_dump`, `pg_restore`, and `pg_isready` are absent, and
  `WAYPOINT_TEST_PG_DSN` is unset;
- Chrome/Chromium, Firefox, Edge, Safari, Playwright, Puppeteer, Selenium, and axe are absent;
- QEMU is present without a retained supported-host image or runnable platform evidence.

## Retained repeatable artifacts

The rerun retains exact commands, UTC start/end times, exit codes, raw test output, and focused
inventories under [`final-v1-acceptance-artifacts/`](final-v1-acceptance-artifacts/). The primary
artifact groups used by all 67 trace rows and EV-01–EV-16 are:

| Key | Retained artifact |
|---|---|
| ENV | [`environment.txt`](final-v1-acceptance-artifacts/environment.txt) |
| STATUS | [`check-status.tsv`](final-v1-acceptance-artifacts/check-status.tsv), [`traceability-check.txt`](final-v1-acceptance-artifacts/traceability-check.txt) |
| CONTRACT | [`verify-contracts.txt`](final-v1-acceptance-artifacts/verify-contracts.txt) |
| DB | [`go-test-summary.txt`](final-v1-acceptance-artifacts/go-test-summary.txt), [`go-test-json.txt`](final-v1-acceptance-artifacts/go-test-json.txt), [`g1-report.txt`](final-v1-acceptance-artifacts/g1-report.txt) |
| WEB | [`web-test.txt`](final-v1-acceptance-artifacts/web-test.txt), [`web-build.txt`](final-v1-acceptance-artifacts/web-build.txt) |
| DEPLOY | [`compose-config.txt`](final-v1-acceptance-artifacts/compose-config.txt), [`docker-info.txt`](final-v1-acceptance-artifacts/docker-info.txt), [`compose-build.txt`](final-v1-acceptance-artifacts/compose-build.txt), [`compose-up.txt`](final-v1-acceptance-artifacts/compose-up.txt), [`compose-runtime-test.txt`](final-v1-acceptance-artifacts/compose-runtime-test.txt), [`smoke.txt`](final-v1-acceptance-artifacts/smoke.txt) |
| SOURCE | [`source-boundary-inventory.txt`](final-v1-acceptance-artifacts/source-boundary-inventory.txt) |
| SCOPE | [`architecture-lint.txt`](final-v1-acceptance-artifacts/architecture-lint.txt), [`deferral-inventory.txt`](final-v1-acceptance-artifacts/deferral-inventory.txt) |

The canonical matrix links every requirement row to these groups in
[`v1-traceability.md`](../v1-traceability.md). Source and fixture checks establish only their narrow
claims; they do not replace a required runtime, browser, platform, clean-room, or sibling-repository
artifact.

## Repeatable checks rerun

```sh
python3 scripts/verify-contracts.py
python3 scripts/lint-architecture.py
go vet ./...
go test -count=1 -json ./...
npm --prefix web test
npm --prefix web run build
go build -o /tmp/waypoint-final-g5-bin/waypoint ./cmd/waypoint
docker compose -f compose.yml config --quiet
docker info
docker compose -f compose.yml build --no-cache
docker compose -f compose.yml up -d --wait
go test -count=1 -v -run '^TestComposeStackPersistsDBAndEvidenceAcrossRestart$' .
python3 scripts/g1-foundation-report.py --output /tmp/waypoint-g1-final.md
make smoke
docker compose -f compose.yml down -v --remove-orphans
```

Observed:

- core contracts pass: 30 schemas, 2 generated artifacts, 690 OpenAPI references, 15 capture cases,
  6 event cases, 4 idempotency cases, 9 cursor cases, 9 problem cases, and the actor, claim, MCP,
  export, and invalid-remediation fixture inventories;
- architecture lint, Go vet, deterministic web build, Go build, and Compose syntax pass;
- web tests report 9 passes, but the bundle E2E stages literal fake dump/evidence/PDF bytes and is
  not production or clean-room evidence;
- Go reports **58 test passes, 39 failures, and 5 skips**. The failures are real-PostgreSQL gates
  failing closed because the DSN is absent; the skips are four export/report DB tests and one
  Compose runtime test. The focused Compose command exits zero only because that gate explicitly
  skips; it is recorded as Unverified, never Pass;
- the production binary exits with `WAYPOINT_DB_DSN is required`, and with an unreachable DSN exits
  on PostgreSQL ping rather than claiming readiness;
- Docker build/up fail because the daemon is unavailable; `make smoke` cannot start without a DSN;
- installer tests pass only against stubbed host services and synthetic OS identities, so they do
  not establish Ubuntu 22.04/24.04 support.

## Complete clean-host journey disposition

| Step | Status | Retained result |
|---|---|---|
| 1. Compose up; provision two humans and an authorized AI | Unverified | Compose syntax only. Daemon and PostgreSQL unavailable; no clean account journey ran. |
| 2. Known/unknown wrapper capture and live Journey log/SSE | Unverified | Core DB tests failed closed and no plugins collector transcript exists. |
| 3. Linux/macOS offline restart and exactly-once replay | Unverified | No wrapper/agent binaries, native hosts, or spool-replay artifact exists. |
| 4. Two-browser non-linear phases, merge/split, alerts | Unverified | No live DB or browser runtime; no two-browser recording exists. |
| 5. Promote, export, verify, clean-room restore/regenerate | Fail | Production export is not a restorable PostgreSQL bundle; no generated bundle/PDF/restore artifact exists. |
| 6. Guarded teardown and post-wipe reconstruction | Fail | Destroy validates receipt text/path only and does not verify bundle bytes or consume server authorization. |
| 7. Security/performance/accessibility and light/dark dogfood | Fail | Required suites/artifacts are absent; browser checks could not run and material security controls are missing. |

## Release-stop defects confirmed by source inspection

1. **Credential digest is still a bearer credential.** `lookupActor` in
   `internal/server/capture.go` deliberately tries a supplied 64-hex digest directly before the
   digest of the presented token. This violates one-time, non-reversible token storage.
2. **Startup egress policy does not exist.** `cmd/waypoint/main.go` does not parse, validate, or
   resolve `WAYPOINT_EGRESS_MODE`; there is no auto resolver/cache or packet assertion proving zero
   discovery traffic in manual/off.
3. **The report/export is not self-contained or restorable.** `buildExportDump` emits partial JSON
   rather than a PostgreSQL custom-format dump; it omits the full actor/audit/entity/result/evidence
   database. No archive is emitted, the evidence tar is assembled in memory and named `.tar.zst`
   without zstd, and bundled tool scripts import repository files that are not included. Wiping the
   source would lose required audit state.
4. **Export preflight/cancellation is not release-safe.** Preflight only checks for a non-empty
   engagement ID; it does not check capacity. Artifact creation buffers dump/evidence and does not
   implement the measured 10 GiB live-safe path.
5. **Supported teardown is not cryptographically guarded.** `scripts/waypoint-installer.sh` accepts
   receipt status and matching path text without hashing manifest entries or the supplied bundle,
   and has no short-lived receipt-bound server authorization.
6. **Summit remains browser-authoritative.** `web/src/App.tsx` and the served runtime fetch report
   JSON/PDF, mint a browser-local receipt, and mark teardown locally instead of consuming persisted
   export jobs/receipts and external teardown authorization.
7. **Runtime security is incomplete.** Capture, entity mutation, claim review, and export handlers
   authenticate but do not enforce the owner/operator/viewer write matrix. There is also no
   TLS-outside-loopback startup enforcement, global CSP/HSTS/frame/nosniff/origin/rate-limit
   middleware, retained IDOR/XSS/SSRF/archive/dependency/container/secret scan, or operator boundary
   guide.
8. **The frozen report is incomplete.** It is rebuilt on request rather than loaded from a persisted
   immutable snapshot, collapses egress status to “not recorded,” and does not inventory pending and
   dismissed out-of-band claims.

The partial export is a release-stop **data-preservation/data-loss defect**: after supported wipe it
cannot reconstruct the complete authoritative audit record.

## All 67 PRD rows reconciled

Every row below is linked to retained artifact groups in the canonical
[`v1-traceability.md`](../v1-traceability.md); statuses are unchanged by unavailable tooling or
skipped gates.

| ID | Status | Final evidence disposition |
|---|---|---|
| PRD-CORE-001 | Unverified | Core durable evidence code exists, but no collector spool/parser-crash/restart round trip or live DB run is retained. |
| PRD-CORE-002 | Unverified | Append-only and actor constraints are implemented in source; all real-DB immutability/concurrency gates failed closed. |
| PRD-CORE-003 | Unverified | Authoritative phase APIs/workspaces now exist, but no retained end-to-end provenance journey spans Recon, Attacks, and Findings. |
| PRD-CORE-004 | Unverified | Source has attempted/result/empty views; no operator runtime proves tried/worked/left state while preserving failures. |
| PRD-DATA-001 | Unverified | Compose declares one PostgreSQL service and no graph/object-store service; topology/startup were not run. |
| PRD-DATA-002 | Unverified | Attribution migrations now cover the prior omissions; 9 migration/constraint tests failed closed without PostgreSQL. |
| PRD-DATA-003 | Unverified | Content-addressed evidence source exists; DB-backed dedup/restart/orphan/quota behavior did not run. |
| PRD-DATA-004 | Unverified | Normalization unit cases pass; stable-key DB conflict/concurrency tests failed closed. |
| PRD-DATA-005 | Unverified | Merge/split/undo source exists; all DB and operator journey evidence is unavailable. |
| PRD-DATA-006 | Unverified | Concurrency/SSE code exists; DB tests failed closed and no two-browser recording exists. |
| PRD-AUD-001 | Unverified | Contract/schema/action reads retain complete attribution fields, but no collector → PostgreSQL → report transcript ran. |
| PRD-AUD-002 | Unverified | Human/AI parity tests exist but failed closed; no plugins collector transcript exists. |
| PRD-AUD-003 | Unverified | AI metadata/context validation exists; live provisioning/capture acceptance did not run. |
| PRD-AUD-004 | Unverified | Immutability/reconstruction tests require PostgreSQL and failed closed. |
| PRD-CAP-001 | Unverified | No retained wrapper execution matrix from plugins exists. |
| PRD-CAP-002 | Unverified | Core fallback tests require PostgreSQL and no unknown/parser-crash collector transcript exists. |
| PRD-CAP-003 | Unverified | Core contract verifier passes; the required plugins compatibility/parser report is absent. |
| PRD-CAP-004 | Unverified | No Windows/Linux/macOS wrapper runtime or release artifact exists. |
| PRD-CAP-005 | Unverified | No Linux/macOS disconnect, process restart, reconnect, or original-timestamp artifact exists. |
| PRD-CAP-006 | Unverified | Core makes no Windows-agent v1 claim, but plugins inventory and Windows-wrapper evidence are absent. |
| PRD-CAP-007 | Unverified | REST ingest source exists; live idempotency and collector conformance failed closed/unavailable. |
| PRD-CAP-008 | Unverified | Standard JSON-RPC MCP initialize/tools flow now exists; REST/MCP parity failed closed without PostgreSQL. |
| PRD-CAP-009 | Unverified | Server-assigned claim/list/link/dismiss source now exists; lifecycle tests failed closed and no operator journey ran. |
| PRD-CAP-010 | Fail | No operator-facing documentation states that wholly out-of-band execution cannot be guaranteed captured. |
| PRD-CAP-011 | Unverified | Server idempotency exists; no collector ack-loss/spool/restart evidence exists. |
| PRD-ID-001 | Fail | A stored 64-hex token digest is still accepted directly as bearer input; no clean bootstrap/two-operator runtime exists. |
| PRD-ID-002 | Unverified | AI provisioning constraints exist; real-DB authorization/revocation tests failed closed. |
| PRD-NET-001 | Unverified | Full exec-host semantics are now stored/read; no multi-interface collector runtime proves source selection. |
| PRD-NET-002 | Unverified | Egress status/observation and ordered pivots are now stored; no collector round trip proves them. |
| PRD-NET-003 | Fail | No startup auto/manual/off resolver policy or zero-traffic packet assertion exists. |
| PRD-RT-001 | Unverified | Cursor/SSE source exists; PostgreSQL reconnect/revocation tests failed closed and no browser run exists. |
| PRD-RT-002 | Unverified | Deterministic rule units pass; DB dedup/SSE tests failed closed and no browser alert was observed. |
| PRD-FIND-001 | Unverified | Manual promotion source exists; permission/validation/evidence test failed closed and no browser flow ran. |
| PRD-FIND-002 | Unverified | Finding/revision schema exists; persistence and reconstruction did not run on PostgreSQL. |
| PRD-REP-001 | Fail | No retained authoritative PDF exists; snapshot is request-time, omits claim gaps, and collapses required network status semantics. |
| PRD-REP-002 | Fail | Production “dump” is partial JSON, no outer archive is created, bundled tools are not self-contained, and no clean-room restore ran. |
| PRD-REP-003 | Fail | No production archive/manifest artifact exists; current outer hash is for a synthetic concatenation, not emitted exact archive bytes. |
| PRD-REP-004 | Fail | An empty hook exists in source/fixtures, but no valid production export artifact establishes the unsigned claim end to end. |
| PRD-REP-005 | Fail | Persisted job source exists, but capacity preflight and bounded 10 GiB live-safe export are absent and DB job tests skipped. |
| PRD-LIFE-001 | Fail | Destroy trusts receipt status/path text and neither hashes bundle bytes nor consumes a short-lived server authorization. |
| PRD-DEP-001 | Unverified | Compose syntax passes; daemon unavailability prevented a clean stack, migration, readiness, and empty engagement run. |
| PRD-DEP-002 | Unverified | Installer now has local DB bootstrap source; only stubbed synthetic-host tests exist, not Ubuntu VM logs. |
| PRD-DEP-003 | Unverified | API/installer provisioning paths exist; no live provision/rotate/revoke journey is retained. |
| PRD-DEP-004 | Fail | Write-role enforcement, TLS, global secure headers/origin controls, scans, and complete operator boundary docs are absent. |
| PRD-UX-001 | Unverified | Expedition shell source exists; no running-app visual comparison is retained. |
| PRD-UX-002 | Unverified | Links are non-linear in source; empty-engagement keyboard/click navigation did not run. |
| PRD-UX-003 | Unverified | Browser fetch/SSE integration exists; no audit-to-log parity/reconnect browser artifact exists. |
| PRD-UX-004 | Fail | Summit mints a browser-local receipt and local teardown state instead of using persisted export jobs and guarded teardown. |
| PRD-UX-005 | Unverified | Static phase notes exist; required offline running-UI dogfood is absent. |
| PRD-UX-006 | Unverified | Technique content exists; runtime context/search/link coverage and scope review are not retained. |
| PRD-UX-007 | Unverified | Guide copy exists; invalid-flow browser/accessibility review is absent. |
| PRD-UX-008 | Unverified | Palette/theme tokens exist; required browser contrast audit and light/dark screenshots are absent. |
| PRD-UX-009 | Unverified | Reduced-motion CSS exists; no browser timing/playback evidence exists. |
| PRD-A11Y-001 | Unverified | Semantic source checks are not a keyboard/accessibility-tree/axe run. |
| PRD-A11Y-002 | Unverified | Responsive CSS is not computed-style, contrast, or mobile viewport dogfood evidence. |
| PRD-PERF-001 | Fail | No baseline dataset, query plans, or measured API p95/p99 report exists. |
| PRD-PERF-002 | Fail | No 10 GiB/concurrent ingest RSS/ack measurement exists; evidence paths remain size-limited and export buffers bytes. |
| PRD-PERF-003 | Fail | No measured SSE latency, warm-route timing, or local-interaction report exists. |
| PRD-QUAL-001 | Fail | The suite has 39 release-critical DB failures plus absent platform/security/performance/browser gates. |
| PRD-QUAL-002 | Unverified | No timestamped desktop/mobile light/dark screenshots or completed operator checklist exists. |
| PRD-DEF-001 | Pass | Architecture lint and route/dependency/copy inventory show no analytical graph surface or graph dependency. |
| PRD-DEF-002 | Pass | No network-zone/firewall reachability map surface is present. |
| PRD-DEF-003 | Pass | `scan-library` is rejected reserved vocabulary; no guided catalog or command-trigger UI exists. |
| PRD-DEF-004 | Pass | No model dependency/call, offensive recommendation, or live-insight UI exists. |
| PRD-DEF-005 | Pass | Findings start through explicit operator promotion; no AI prefill/autopublish surface exists. |
| PRD-DEF-006 | Pass | No runtime parser/plugin generation or upload endpoint/UI exists. |
| PRD-DEF-007 | Pass | Export vocabulary is hash-only with an empty hook; no key management or signer-identity claim exists. |

## EV-01–EV-16 disposition

Artifact keys refer to the retained table above.

| Evidence | Status | Final disposition | Artifact groups |
|---|---|---|---|
| EV-01 | Unverified | Core contracts and MCP fixtures pass; no retained plugins compatibility/parser report exists. | CONTRACT, ENV |
| EV-02 | Fail | 39 PostgreSQL tests fail closed and digest-as-token acceptance remains a definite auth defect. | DB, SOURCE |
| EV-03 | Unverified | Core attribution schema is now lossless, but no human/AI/unknown-tool collector-to-DB transcript exists. | CONTRACT, DB, ENV |
| EV-04 | Unverified | No offline replay or native wrapper/agent platform artifacts exist. | ENV, SOURCE |
| EV-05 | Fail | Startup egress resolver/config and manual/off packet assertions are absent. | ENV, DB, SOURCE |
| EV-06 | Unverified | Entity DB tests fail closed and no merge/split operator provenance journey exists. | DB, WEB |
| EV-07 | Unverified | DB concurrency/SSE tests fail closed and no two-browser recording exists. | DB, WEB, ENV |
| EV-08 | Fail | No authoritative retained PDF/trace artifact exists and report snapshot semantics are incomplete. | DB, WEB, SOURCE |
| EV-09 | Fail | Production bundle is not a PostgreSQL dump/archive or self-contained clean-room restore; fixture bytes are synthetic. | DB, WEB, SOURCE |
| EV-10 | Fail | Teardown does not hash exact bundle bytes or consume bound authorization; no post-wipe reconstruction ran. | DEPLOY, SOURCE |
| EV-11 | Unverified | Compose/installer definitions exist, but no Docker stack or supported Ubuntu VM run is retained. | ENV, DEPLOY, DB |
| EV-12 | Fail | No measured baseline performance/fault report exists. | DB, SOURCE |
| EV-13 | Fail | No retained security report; auth/role, TLS/header/origin, rate-limit, scan, and boundary-doc defects remain. | DB, SOURCE, ENV |
| EV-14 | Unverified | No real-browser light/dark desktop/mobile screenshots or completed UX checklist exists. | ENV, WEB, DEPLOY |
| EV-15 | Unverified | No accessibility tree, keyboard, axe, screen-reader, or reduced-motion browser artifact exists. | ENV, WEB, DEPLOY |
| EV-16 | Pass | Architecture lint scans 159 claim-surface files and the route/dependency/copy inventory contains no functioning v2 surface. | SCOPE, SOURCE |

## Deferral inventory rerun

```sh
python3 scripts/lint-architecture.py
grep -RhoE '"/api/v1/[^" ]+' internal/server/*.go | sort -u
grep -Ei 'apache age|neo4j|openai|anthropic|ollama' go.mod web/package-lock.json || true # no deferred dependencies expected
```

Architecture lint passes all 9 scope rules over 159 claim-surface files. The runtime route inventory
contains capture, action, evidence, actor, entity, finding, claim, audit, MCP, report, and export
lifecycle surfaces only. There is no graph/AGE/Neo4j or network-zone map;
no guided scan catalog; no model provider/call or AI finding prefill; no runtime plugin
creation/upload; and no signing key or signer-identity surface. `scan-library` remains a rejected
reserved compatibility enum.

## Gate decision

G0's core side passes, but cross-repository G0 is Unverified. G1–G4 cannot close without the missing
runtime evidence and contain the definite defects above. Therefore G5 **fails closed**. The release
must not be labeled v1-ready and teardown must not be used as a preservation-safe workflow.
