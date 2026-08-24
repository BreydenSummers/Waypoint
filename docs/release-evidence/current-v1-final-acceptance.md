# Current-main final fail-closed v1 acceptance

Run: `2026-08-24T08:42:57Z–08:51:10Z`  
Task: `current-v1-007-final-acceptance`  
Technical verdict: **Fail — not v1-ready (7 Pass, 15 Fail, 45 Unverified).**  
Technical readiness recommended: **false**  
Human acceptance: **false** — no explicit owner grant is retained, and technical acceptance cannot grant human acceptance.

G5 remains closed. All 67 applicable PRD rows must pass; 60 are blocking. This audit counted no
unavailable runtime, skipped test, source-shape check, contract fixture, synthetic host, or historical
artifact as runtime acceptance.

## Mandatory acceptance conditions

| Condition | Result | Fail-closed disposition |
|---|---|---|
| All 67 applicable PRD rows pass | **Fail** | 7 Pass, 15 Fail, 45 Unverified. |
| Release-critical tests have zero skips | **Fail** | Current Go run: 43 top-level Pass, 0 Fail, **48 Skip**. All real-PostgreSQL and Compose gates remain skipped. |
| No data-loss defect exists | **Fail** | The export is a partial JSON projection rather than a complete PostgreSQL dump, bundled restore tools are not self-contained, and supported destroy does not verify/authorize exact preserved bytes end to end. |
| Security has no open critical/high | **Fail** | No retained current security assessment establishes this; digest-as-bearer authentication and material runtime-control gaps remain. |
| Performance budgets pass | **Fail** | No measured baseline p95/p99, 10 GiB ingest RSS/ack, SSE/UI latency, or complete fault report exists. |
| Build/deployment are reproducible | **Fail** | Core/web builds and Compose syntax pass, but Docker build/up cannot run and no supported Ubuntu clean-host run is retained. |
| Dogfood artifacts are complete | **Fail** | No running-app desktop/mobile light/dark screenshots, browser operator journey, accessibility tree, keyboard/axe, or reduced-motion artifact exists. |

Because every condition is conjunctive, any one failure is sufficient to reject release. All seven
conditions above currently fail.

## Evidence boundary

The assigned tree is the core repository. Its `.git` metadata resolves outside the sandbox, so
history/status is unavailable. The sibling plugins worktree is outside the permitted boundary, and
this tree retains no current plugins compatibility/parser report, collector transcript, spool replay,
native wrapper/agent run, release-binary matrix, or platform report.

The host has Docker/Compose clients but no daemon, PostgreSQL service/tools/DSN, browser binary, or
browser automation/accessibility runtime. These are blockers, not waivers. Exact current environment,
commands, UTC timestamps, exit codes, full Go JSON, and inventories are retained under
[`current-v1-final-acceptance-artifacts/`](current-v1-final-acceptance-artifacts/); the directory has a
[`SHA256SUMS`](current-v1-final-acceptance-artifacts/SHA256SUMS) inventory.

### Current artifact keys

| Key | Retained evidence |
|---|---|
| ENV | [`environment.txt`](current-v1-final-acceptance-artifacts/environment.txt) |
| STATUS | [`check-status.tsv`](current-v1-final-acceptance-artifacts/check-status.tsv), [`traceability-precheck.txt`](current-v1-final-acceptance-artifacts/traceability-precheck.txt) |
| CONTRACT | [`verify-contracts.txt`](current-v1-final-acceptance-artifacts/verify-contracts.txt) |
| DB | [`go-test-all.json`](current-v1-final-acceptance-artifacts/go-test-all.json), [`go-test-summary.txt`](current-v1-final-acceptance-artifacts/go-test-summary.txt), [`g1-report.txt`](current-v1-final-acceptance-artifacts/g1-report.txt) |
| WEB | [`web-test.txt`](current-v1-final-acceptance-artifacts/web-test.txt), [`web-build.txt`](current-v1-final-acceptance-artifacts/web-build.txt) |
| BUILD | [`go-build.txt`](current-v1-final-acceptance-artifacts/go-build.txt), [`go-vet.txt`](current-v1-final-acceptance-artifacts/go-vet.txt), [`make-lint.txt`](current-v1-final-acceptance-artifacts/make-lint.txt), [`make-test.txt`](current-v1-final-acceptance-artifacts/make-test.txt) |
| DEPLOY | [`compose-config.txt`](current-v1-final-acceptance-artifacts/compose-config.txt), [`docker-info.txt`](current-v1-final-acceptance-artifacts/docker-info.txt), [`compose-build.txt`](current-v1-final-acceptance-artifacts/compose-build.txt), [`compose-up.txt`](current-v1-final-acceptance-artifacts/compose-up.txt), [`compose-runtime-test.txt`](current-v1-final-acceptance-artifacts/compose-runtime-test.txt), [`smoke.txt`](current-v1-final-acceptance-artifacts/smoke.txt) |
| SOURCE | [`source-boundary-inventory.txt`](current-v1-final-acceptance-artifacts/source-boundary-inventory.txt) |
| SCOPE | [`architecture-lint.txt`](current-v1-final-acceptance-artifacts/architecture-lint.txt), [`deferral-inventory.txt`](current-v1-final-acceptance-artifacts/deferral-inventory.txt) |
| RUNTIME | [`current-v1-runtime-gates.md`](current-v1-runtime-gates.md) and its retained focused performance/export artifacts |
| UX | [`ux-dogfood.md`](ux-dogfood.md), [`browser-support-matrix-g5.md`](browser-support-matrix-g5.md) |

## Checks rerun

```sh
python3 scripts/verify-contracts.py
python3 scripts/lint-architecture.py
go vet ./...
make lint
go test -count=1 -json ./...
npm --prefix web test
npm --prefix web run build
go build -o /tmp/current-v1-final-waypoint ./cmd/waypoint
docker compose -f compose.yml config --quiet
docker info
docker compose -f compose.yml build --no-cache
docker compose -f compose.yml up -d --wait
go test -count=1 -v -run '^TestComposeStackPersistsDBAndEvidenceAcrossRestart$' .
python3 scripts/g1-foundation-report.py --output /tmp/current-v1-final-g1.md
make smoke
docker compose -f compose.yml down -v --remove-orphans
make test
```

Observed:

- core contracts pass: 30 schemas, 2 generated artifacts, 690 OpenAPI references, 15 capture cases,
  6 event cases, 4 idempotency cases, 9 cursor cases, 9 problem cases, and the current action, actor,
  claim, MCP, export, and invalid-remediation fixture inventories;
- final architecture lint passes 9 scope rules over 166 claim-surface files;
- Go vet, official lint/test targets, web tests/build, Go build, and Compose syntax pass within their
  narrow boundaries;
- `go test` exits zero but reports **48 release-critical skips**. The focused Compose test also exits
  zero only because it skips when the daemon is unavailable;
- the G1 reporter fails closed because `WAYPOINT_TEST_PG_DSN` is absent;
- Docker build/up/down fail because the daemon is unavailable; `make smoke` fails because the app
  requires `WAYPOINT_DB_DSN`;
- web/export, installer, PDF, UX, and performance source/fixture tests remain synthetic or
  source-shape checks and do not replace their required runtime artifacts.

## Current release-stop defects

1. **Credential digests authenticate directly.** `lookupActor` accepts both `sha256(token)` and a
   presented 64-hex value itself. A stolen stored digest is therefore a bearer credential.
2. **Startup egress policy is absent.** `cmd/waypoint/main.go` does not parse or enforce
   `WAYPOINT_EGRESS_MODE`, resolve auto mode, configure manual/off semantics, or establish zero
   discovery traffic for manual/off.
3. **Export is not a complete restorable database bundle.** `buildExportDump` serializes only the
   report engagement, actions, and findings. It is not a PostgreSQL custom-format dump and omits
   authoritative actor, audit, entity/observation/result, evidence metadata, claim, revision, export,
   receipt, and authorization state.
4. **The production bundle is internally inconsistent and not self-contained.** The archive is
   created before its hash sidecar, while later verification expects all files then present in the
   bundle directory. Generated restore scripts import `../../web/scripts/bundle-tools.mjs`, which is
   not included in the manifest payloads. No clean-room restore/regeneration exists.
5. **Supported teardown is not preservation-safe.** The installer checks receipt status and matching
   path text, but does not hash the supplied archive/manifest or consume the server's short-lived
   receipt-bound authorization before deleting roots.
6. **Report attribution remains lossy.** The report action query omits egress mode/status/observation,
   pivot chain, source-agent detail, execution failure semantics, and complete evidence metadata;
   null egress is collapsed to `not recorded`. No authoritative real-Chromium PDF is retained.
7. **Runtime security controls and evidence are incomplete.** TLS-outside-loopback enforcement,
   complete role enforcement, global secure headers/origin/rate controls, operator boundary guidance,
   and retained IDOR/XSS/SSRF/archive/dependency/container/secret scans are absent.
8. **Measured performance and operator/browser evidence are absent.** Fixture constants, source
   checks, and bounded unit streams do not establish the PRD production budgets or dogfood gate.

Defects 3–5 form a current **data-preservation/data-loss defect**: after supported wipe, the complete
authoritative audit record cannot be reconstructed from the emitted bundle alone.

## All 67 PRD rows

| ID | Status | Final current-main disposition |
|---|---|---|
| PRD-CORE-001 | Unverified | Core raw evidence paths exist, but no collector spool/parser-crash/restart round trip or live DB run is retained. |
| PRD-CORE-002 | Unverified | Append-only and actor constraints exist; all real-DB immutability/concurrency gates skipped. |
| PRD-CORE-003 | Unverified | Phase APIs/workspaces exist; no retained Recon → Attacks → Findings provenance journey exists. |
| PRD-CORE-004 | Unverified | Source models tried/result/empty states; no operator runtime proves failed attempts remain visible. |
| PRD-DATA-001 | Unverified | Compose declares one PostgreSQL service and no graph/object-store; topology/startup did not run. |
| PRD-DATA-002 | Unverified | Migrations represent the planned schema; migration/constraint/round-trip tests skipped. |
| PRD-DATA-003 | Unverified | Content-addressed evidence source exists; DB dedup/restart/orphan/isolation/fault tests skipped. |
| PRD-DATA-004 | Unverified | Stable-key normalization units pass; real-DB conflict/concurrency tests skipped. |
| PRD-DATA-005 | Unverified | Merge/split/preview/undo source exists; DB and operator evidence is unavailable. |
| PRD-DATA-006 | Unverified | Keyset/version/SSE source exists; DB concurrency tests skipped and no two-browser run exists. |
| PRD-AUD-001 | Unverified | Authoritative reads expose accepted attribution, but no collector → PostgreSQL → report transcript exists and report projection is lossy. |
| PRD-AUD-002 | Unverified | Human/AI validation exists; parity DB tests skipped and no collector transcript exists. |
| PRD-AUD-003 | Unverified | AI authorization/context validation exists; live provision/capture acceptance skipped. |
| PRD-AUD-004 | Unverified | Protections/revisions exist in source; immutable reconstruction tests skipped. |
| PRD-CAP-001 | Unverified | No retained wrapper execution matrix exists. |
| PRD-CAP-002 | Unverified | Core fallback paths exist; no unknown/parser-crash collector-to-DB round trip exists. |
| PRD-CAP-003 | Unverified | Core contracts pass; plugins matching/isolation/fixture compatibility evidence is absent. |
| PRD-CAP-004 | Unverified | No Windows/Linux/macOS wrapper binaries or native runs are retained. |
| PRD-CAP-005 | Unverified | No Linux/macOS disconnect/restart/reconnect timestamp-preserving agent run exists. |
| PRD-CAP-006 | Unverified | Core makes no Windows-agent v1 claim, but plugins inventory and Windows-wrapper evidence are absent. |
| PRD-CAP-007 | Unverified | Versioned idempotent REST source exists; live DB/collector conformance tests skipped. |
| PRD-CAP-008 | Unverified | Standard JSON-RPC MCP source exists; PostgreSQL parity test skipped and no client transcript exists. |
| PRD-CAP-009 | Unverified | Claim lifecycle source exists; PostgreSQL test skipped and no operator review journey exists. |
| PRD-CAP-010 | Fail | No operator-facing residual-boundary guide states that wholly out-of-band activity cannot be guaranteed captured. |
| PRD-CAP-011 | Unverified | Server idempotency exists; no collector ack-loss/spool/restart replay artifact exists. |
| PRD-ID-001 | Fail | A stored 64-hex digest is accepted directly as bearer input; no clean two-operator journey exists. |
| PRD-ID-002 | Unverified | AI actor constraints/lifecycle source exists; real-DB authorization/revocation tests skipped. |
| PRD-NET-001 | Unverified | Core stores exec-host detail; no multi-interface collector source-selection run exists. |
| PRD-NET-002 | Unverified | Core stores egress/pivots; no collector round trip proves them. |
| PRD-NET-003 | Fail | No startup auto/manual/off implementation or manual/off packet assertion exists. |
| PRD-RT-001 | Unverified | Cursor SSE source exists; DB reconnect/revocation tests skipped and no browser run exists. |
| PRD-RT-002 | Unverified | Alert units pass; DB dedup/SSE tests skipped and no live alert was observed. |
| PRD-FIND-001 | Unverified | Manual promotion source exists; DB permission/evidence test skipped and no browser flow exists. |
| PRD-FIND-002 | Unverified | Finding/revision schema exists; persistence/history reconstruction did not run on PostgreSQL. |
| PRD-REP-001 | Fail | Report attribution is lossy and no authoritative PostgreSQL-to-real-Chromium frozen PDF is retained. |
| PRD-REP-002 | Fail | `engagement.dump` is partial JSON, tools are not self-contained, and no complete clean-room restore exists. |
| PRD-REP-003 | Fail | Production archive/sidecar verification is inconsistent and no valid production manifest/archive artifact exists. |
| PRD-REP-004 | Fail | Empty signature hook/hash-only wording exists, but no valid production export establishes it end to end. |
| PRD-REP-005 | Fail | Job source exists, but verification/live-safe behavior is unproven and real-DB export/fault tests skipped. |
| PRD-LIFE-001 | Fail | Installer destroy neither hashes exact bundle bytes nor consumes server authorization; no post-wipe reconstruction exists. |
| PRD-DEP-001 | Unverified | Compose syntax passes; daemon absence prevented clean build/up/readiness/restart/teardown. |
| PRD-DEP-002 | Unverified | Installer source exists; only synthetic host tests, not supported Ubuntu VM runs, are retained. |
| PRD-DEP-003 | Unverified | Provisioning source exists; no live provision/rotate/revoke journey is retained. |
| PRD-DEP-004 | Fail | Digest-as-bearer, TLS/middleware/role/doc gaps, and absent scans block sensitive deployment. |
| PRD-UX-001 | Unverified | Expedition shell source exists; no running-app visual comparison exists. |
| PRD-UX-002 | Unverified | Links are non-linear in source; empty-state keyboard/click navigation did not run. |
| PRD-UX-003 | Unverified | Journey log API/SSE source exists; no audit-row parity/reconnect browser artifact exists. |
| PRD-UX-004 | Fail | Summit calls server lifecycle APIs, but export verification and supported external wipe are not preservation-safe. |
| PRD-UX-005 | Unverified | Static phase notes exist; offline running-UI dogfood is absent. |
| PRD-UX-006 | Unverified | Technique content exists; browser context/search/link evidence is absent. |
| PRD-UX-007 | Unverified | Guide-style copy exists; invalid-flow browser/accessibility review is absent. |
| PRD-UX-008 | Unverified | Tokens/source contrast checks exist; browser contrast and light/dark screenshots are absent. |
| PRD-UX-009 | Unverified | Reduced-motion CSS exists; browser timing/playback evidence is absent. |
| PRD-A11Y-001 | Unverified | Semantic source checks are not keyboard/accessibility-tree/axe evidence. |
| PRD-A11Y-002 | Unverified | Responsive CSS is not computed-style/contrast/mobile dogfood evidence. |
| PRD-PERF-001 | Fail | No baseline dataset, query plans, or measured API p95/p99 report exists. |
| PRD-PERF-002 | Fail | Streaming source/unit checks exist; no 10 GiB concurrent ingest RSS/ack measurement exists. |
| PRD-PERF-003 | Fail | No measured SSE visibility, warm-route, or local-interaction report exists. |
| PRD-QUAL-001 | Fail | Forty-eight release-critical tests skipped; platform/security/performance/browser/plugins gates are absent. |
| PRD-QUAL-002 | Unverified | No timestamped desktop/mobile light/dark screenshots or completed operator checklist exists. |
| PRD-DEF-001 | Pass | Current lint/source/dependency inventory shows no analytical graph surface/dependency. |
| PRD-DEF-002 | Pass | No network-zone/firewall reachability-map surface exists. |
| PRD-DEF-003 | Pass | Only rejected reserved compatibility vocabulary exists; no guided scan catalog/trigger exists. |
| PRD-DEF-004 | Pass | No model provider/call, offensive recommendation, or live-insight UI exists. |
| PRD-DEF-005 | Pass | Findings start through explicit operator promotion; no AI prefill/autopublish exists. |
| PRD-DEF-006 | Pass | No runtime plugin-generation/upload endpoint or UI exists. |
| PRD-DEF-007 | Pass | Hash-only wording/empty hook exists; no key management or signer-identity claim exists. |

## EV-01–EV-16

| Evidence | Status | Final current-main disposition |
|---|---|---|
| EV-01 | Unverified | Core contracts pass; no retained plugins compatibility/parser/runtime report exists. |
| EV-02 | Fail | Real-DB migration/auth tests skipped and digest-as-bearer remains a definite auth defect. |
| EV-03 | Unverified | Core capture source exists, but no human/AI/unknown-tool collector-to-PostgreSQL transcript exists. |
| EV-04 | Unverified | No offline replay or native wrapper/agent platform matrix exists. |
| EV-05 | Fail | Startup egress policy/resolver and packet assertions are absent. |
| EV-06 | Unverified | Entity PostgreSQL tests skipped and no merge/split operator provenance journey exists. |
| EV-07 | Unverified | SSE/concurrency PostgreSQL tests skipped and no two-browser recording exists. |
| EV-08 | Fail | No authoritative real-Chromium PDF trace exists and report attribution remains incomplete. |
| EV-09 | Fail | Dump/archive/tools are not self-contained/consistent and no clean-room restore exists. |
| EV-10 | Fail | Installer destroy does not verify exact bytes or consume bound authorization; no post-wipe reconstruction exists. |
| EV-11 | Unverified | Definitions exist; no daemon-backed stack or supported Ubuntu VM run is retained. |
| EV-12 | Fail | No measured baseline performance/fault report exists. |
| EV-13 | Fail | Material auth/TLS/middleware/role/doc gaps remain and no retained security report exists. |
| EV-14 | Unverified | No running-app light/dark desktop/mobile screenshot/checklist set exists. |
| EV-15 | Unverified | No accessibility-tree, keyboard, axe/screen-reader, or reduced-motion browser artifact exists. |
| EV-16 | Pass | Current architecture lint and focused inventory prove the seven narrow v2 deferrals. |

## Contract and v2-deferral inventories

The contract rerun passes core consistency only. Cross-repository compatibility remains Unverified.
The v2 inventory rerun finds no graph/AGE/Neo4j, no network-zone map, no guided scan catalog, no
offensive model provider/call, no AI finding prefill, no runtime plugin generation/upload, and no
cryptographic signing identity surface. `scan-library` remains a rejected reserved enum value. Accordingly only
PRD-DEF-001–007 and EV-16 pass.

## Final decision

G5 **fails closed**. Waypoint must not be labeled v1-ready, and the current export → teardown flow
must not be represented as preservation-safe. Technical readiness is not recommended on this
evidence. Human acceptance remains **false** and may only change after an explicit owner grant;
technical automation must never infer or grant it.
