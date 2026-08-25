# Current merged-code final fail-closed v1 acceptance

Run: `2026-08-25T20:05:43Z–20:09:16Z`  
Task: `release-fix-008-final-g5`  
Technical verdict: **Fail — not v1-ready (8 Pass, 10 Fail, 49 Unverified).**  
Technical completion recommended: **false**  
Human acceptance: **not evaluated or inferred by this technical gate.**

G5 remains closed. All 67 applicable PRD rows must pass; 59 are blocking. This rerun used only
repeatable evidence retained from the current assigned tree. A skipped test, unavailable service,
source-shape assertion, synthetic fixture, or historical artifact was never promoted to runtime Pass.

## Mandatory acceptance conditions

| Condition | Result | Fail-closed disposition |
|---|---|---|
| All 67 applicable PRD rows pass | **Fail** | 8 Pass, 10 Fail, 49 Unverified. |
| Release-critical tests have zero skips | **Fail** | Current Go run: 73 top-level Pass, 0 Fail, **50 Skip**. PostgreSQL, Compose, capture, export, and concurrency gates skipped. |
| No data-loss issue exists | **Fail** | `engagement.dump` remains a partial JSON projection, and supported destroy still checks receipt status/path rather than exact bytes and bound server authorization. |
| Security has no unresolved critical/high finding | **Fail** | No current security assessment establishes this; a stored 64-hex token digest is still accepted as a bearer credential, TLS-outside-loopback is not enforced, and dedicated scanners are unavailable. |
| Performance budgets pass | **Fail** | No measured baseline API p95/p99, 10 GiB ingest RSS/ack, SSE/UI latency, export-duration, or complete fault report exists. |
| Build/deployment are reproducible | **Fail** | Two trimmed Go builds and repeated web assets are byte-identical, and Compose syntax passes; Docker build/up and supported Ubuntu clean-host deployment did not run. |
| Browser dogfood is complete | **Fail** | The authoritative host runner failed before startup; no desktop/mobile light/dark screenshots, operator journey, accessibility tree, keyboard/axe, or reduced-motion evidence was produced. |
| v2 deferrals are explicit | **Pass** | Architecture lint and current route/dependency/copy inventory pass all seven deferral checks. |

The conditions are conjunctive. Technical completion is not recommended.

## Evidence boundary

The assigned worktree is the core repository. Its Git worktree metadata resolves outside the sandbox,
so history/status is unavailable. The plugins repository is outside the permitted boundary, and no
current plugins compatibility, parser, collector, spool replay, release-binary, or native-platform
artifact is retained here.

The host has Docker/Compose clients but no daemon, PostgreSQL tools/service/DSN, browser binary,
Playwright, axe, or dedicated security scanner. These are blockers, not waivers. Exact commands,
UTC timestamps, exit codes, full Go JSON, focused source excerpts, and inventories are retained under
[`current-v1-final-acceptance-artifacts/`](current-v1-final-acceptance-artifacts/), with a SHA-256
inventory.

### Current artifact keys

| Key | Retained evidence |
|---|---|
| ENV | [`environment.txt`](current-v1-final-acceptance-artifacts/environment.txt), [`run-at.txt`](current-v1-final-acceptance-artifacts/run-at.txt) |
| STATUS | [`check-status.tsv`](current-v1-final-acceptance-artifacts/check-status.tsv) |
| CONTRACT | [`verify-contracts.txt`](current-v1-final-acceptance-artifacts/verify-contracts.txt) |
| DB/TEST | [`go-test-all.json`](current-v1-final-acceptance-artifacts/go-test-all.json), [`go-test-summary.txt`](current-v1-final-acceptance-artifacts/go-test-summary.txt), [`go-test-race.txt`](current-v1-final-acceptance-artifacts/go-test-race.txt), [`g1-report.txt`](current-v1-final-acceptance-artifacts/g1-report.txt) |
| EGRESS | [`egress-policy-tests.txt`](current-v1-final-acceptance-artifacts/egress-policy-tests.txt) |
| PERF/FAULT | [`performance-fault-tests.txt`](current-v1-final-acceptance-artifacts/performance-fault-tests.txt) |
| EXPORT | [`export-tests.txt`](current-v1-final-acceptance-artifacts/export-tests.txt), [`source-boundary-inventory.txt`](current-v1-final-acceptance-artifacts/source-boundary-inventory.txt) |
| WEB/BUILD | [`web-test.txt`](current-v1-final-acceptance-artifacts/web-test.txt), [`web-build.txt`](current-v1-final-acceptance-artifacts/web-build.txt), [`go-build.txt`](current-v1-final-acceptance-artifacts/go-build.txt), [`reproducible-build.txt`](current-v1-final-acceptance-artifacts/reproducible-build.txt) |
| DEPLOY | [`compose-config.txt`](current-v1-final-acceptance-artifacts/compose-config.txt), [`docker-info.txt`](current-v1-final-acceptance-artifacts/docker-info.txt), [`compose-build.txt`](current-v1-final-acceptance-artifacts/compose-build.txt), [`compose-up.txt`](current-v1-final-acceptance-artifacts/compose-up.txt), [`compose-runtime-test.txt`](current-v1-final-acceptance-artifacts/compose-runtime-test.txt), [`smoke.txt`](current-v1-final-acceptance-artifacts/smoke.txt) |
| SECURITY | [`security-inventory.txt`](current-v1-final-acceptance-artifacts/security-inventory.txt) |
| UX | [`ux-dogfood.txt`](current-v1-final-acceptance-artifacts/ux-dogfood.txt), [`ux-dogfood-runtime/blocker.txt`](current-v1-final-acceptance-artifacts/ux-dogfood-runtime/blocker.txt) |
| SCOPE | [`architecture-lint.txt`](current-v1-final-acceptance-artifacts/architecture-lint.txt), [`deferral-inventory.txt`](current-v1-final-acceptance-artifacts/deferral-inventory.txt) |

## Checks rerun

```sh
python3 scripts/verify-contracts.py
python3 scripts/lint-architecture.py
go vet ./...
make lint
go test -count=1 -json ./...
go test -race -count=1 ./...
go test -count=1 -v ./internal/egresspolicy
go test -count=1 -v ./internal/server -run '<performance/fault gate set>'
go test -count=1 -v ./internal/server -run '<export gate set>'
npm --prefix web test
npm --prefix web run build
go build -trimpath -o /tmp/release-fix-008-final-g5-waypoint-a ./cmd/waypoint
# repeat Go/web builds and compare bytes
docker compose -f compose.yml config --quiet
docker info
docker compose -f compose.yml build --no-cache
docker compose -f compose.yml up -d --wait
go test -count=1 -v -run '^TestComposeStackPersistsDBAndEvidenceAcrossRestart$' .
python3 scripts/g1-foundation-report.py --output /tmp/release-fix-008-final-g5-g1.md
make smoke
bash scripts/ux-dogfood-host.sh docs/release-evidence/current-v1-final-acceptance-artifacts/ux-dogfood-runtime
docker compose -f compose.yml down -v --remove-orphans
make test
```

Observed:

- core contracts pass: 30 schemas, 2 generated artifacts, 690 OpenAPI references, 15 capture cases,
  6 event cases, 4 idempotency cases, 9 cursor cases, 9 problem cases, and current remediation fixtures;
- architecture lint passes 9 scope rules over 171 claim-surface files;
- Go vet, race/unit boundaries, official lint/test, web tests/build, Go build, and Compose syntax pass;
- two `-trimpath` Go binaries have the same SHA-256, and a repeated web build has identical assets;
- startup egress auto/manual/off tests pass, including no-discovery packet traps for manual/off;
- the full Go suite exits zero but reports **50 release-critical skips**; focused export/fault and
  Compose commands also exit zero only because their runtime cases skip;
- Docker build/up/down, G1, smoke, and UX host-runner gates fail because required services are absent.

## Current release-stop defects

1. **Credential digests authenticate directly.** `lookupActor` tries a presented 64-hex value as a
   stored digest before trying `sha256(token)`. Exfiltrated credential storage is therefore usable as
   bearer material.
2. **Export is not a complete database bundle.** `buildExportDump` serializes only engagement,
   report actions, and findings as JSON. It is not a PostgreSQL custom-format dump and omits actor,
   audit, entity/observation/result, evidence metadata, claims, revisions, jobs, receipts, and
   authorization state.
3. **Supported teardown is not preservation-safe.** The installer checks receipt status and path,
   but does not hash the supplied archive/manifest or consume the short-lived server authorization
   before deleting state.
4. **Sensitive deployment controls are incomplete.** The server uses plain `ListenAndServe`; default
   Compose publishes HTTP on all interfaces. No retained security scan establishes no critical/high
   finding.
5. **Operator boundary documentation is absent.** PRD/ADR/contract/planning prose is not a supported
   operator guide covering wholly out-of-band execution and deployment/export residual boundaries.
6. **Measured runtime and browser evidence is absent.** Performance budgets, PostgreSQL/Compose
   journeys, native collectors, security scans, and browser dogfood remain unexecuted.

Defects 2–3 are an open data-preservation/data-loss issue: after supported wipe, the complete
authoritative record cannot be reconstructed from the emitted bundle alone.

## All 67 PRD rows

| ID | Status | Current disposition |
|---|---|---|
| PRD-CORE-001 | Unverified | No collector durable-spool/parser-crash/restart round trip or live PostgreSQL capture run is retained. |
| PRD-CORE-002 | Unverified | Append-only/actor source exists; real-DB immutability and concurrency gates skipped. |
| PRD-CORE-003 | Unverified | Phase APIs/workspaces exist; no retained Recon → Attacks → Findings provenance journey exists. |
| PRD-CORE-004 | Unverified | Source models tried/result/empty states; no operator runtime proves failed attempts remain visible. |
| PRD-DATA-001 | Unverified | Compose declares one PostgreSQL service and no graph/object store; topology/startup did not run. |
| PRD-DATA-002 | Unverified | Migrations represent planned fields; migration/constraint/round-trip tests skipped. |
| PRD-DATA-003 | Unverified | Content-addressed evidence source exists; DB isolation/dedup/restart/fault tests skipped. |
| PRD-DATA-004 | Unverified | Stable-key units pass; real-DB conflict/concurrency tests skipped. |
| PRD-DATA-005 | Unverified | Merge/split/preview/undo source exists; DB and operator evidence is unavailable. |
| PRD-DATA-006 | Unverified | Keyset/version/SSE source exists; DB concurrency tests skipped and no two-browser run exists. |
| PRD-AUD-001 | Unverified | Authoritative action/report fields exist, but no collector → PostgreSQL → report transcript exists. |
| PRD-AUD-002 | Unverified | Human/AI validation exists; parity DB tests and collector transcript are unavailable. |
| PRD-AUD-003 | Unverified | AI authorization/context validation exists; live provision/capture acceptance skipped. |
| PRD-AUD-004 | Unverified | Protections/revisions exist; immutable reconstruction tests skipped. |
| PRD-CAP-001 | Unverified | No retained wrapper execution matrix exists. |
| PRD-CAP-002 | Unverified | Core fallback source exists; no unknown/parser-crash collector-to-DB round trip exists. |
| PRD-CAP-003 | Unverified | Core contracts pass; plugins matching/isolation/fixture compatibility evidence is absent. |
| PRD-CAP-004 | Unverified | No Windows/Linux/macOS wrapper binaries or native runs are retained. |
| PRD-CAP-005 | Unverified | No Linux/macOS disconnect/restart/reconnect agent run exists. |
| PRD-CAP-006 | Unverified | Core makes no Windows-agent v1 claim, but plugins inventory and Windows-wrapper evidence are absent. |
| PRD-CAP-007 | Unverified | REST source exists; live DB/collector conformance tests skipped. |
| PRD-CAP-008 | Unverified | Standard MCP source exists; PostgreSQL parity/client transcript is absent. |
| PRD-CAP-009 | Unverified | Claim lifecycle source exists; PostgreSQL test and operator review journey are absent. |
| PRD-CAP-010 | Fail | No supported operator guide states that wholly out-of-band execution cannot be guaranteed captured. |
| PRD-CAP-011 | Unverified | Server idempotency exists; no collector ack-loss/spool/restart replay artifact exists. |
| PRD-ID-001 | Fail | Stored 64-hex digest remains directly accepted as bearer input; no clean two-operator journey exists. |
| PRD-ID-002 | Unverified | AI actor constraints/lifecycle source exists; real-DB authorization/revocation tests skipped. |
| PRD-NET-001 | Unverified | Core stores exec-host detail; no multi-interface collector selection run exists. |
| PRD-NET-002 | Unverified | Core stores egress/pivots; no collector round trip proves them. |
| PRD-NET-003 | Pass | Startup config/resolve units pass auto/manual/off, and retained packet traps prove manual/off send no discovery traffic. |
| PRD-RT-001 | Unverified | Cursor SSE source exists; DB reconnect/revocation tests skipped and no browser run exists. |
| PRD-RT-002 | Unverified | Alert units pass; DB dedup/SSE test skipped and no live alert was observed. |
| PRD-FIND-001 | Unverified | Manual promotion source exists; DB permission/evidence test skipped and no browser flow exists. |
| PRD-FIND-002 | Unverified | Finding/revision schema exists; PostgreSQL history reconstruction did not run. |
| PRD-REP-001 | Unverified | Report projection now carries richer attribution, but no PostgreSQL-to-real-Chromium frozen PDF is retained. |
| PRD-REP-002 | Fail | `engagement.dump` is partial JSON rather than a complete PostgreSQL dump; no complete clean-room restore exists. |
| PRD-REP-003 | Unverified | Manifest/archive source and synthetic tests exist; production export tests skipped and no valid live bundle is retained. |
| PRD-REP-004 | Unverified | Empty signature hook/hash-only wording exists; no valid production export establishes it end to end. |
| PRD-REP-005 | Unverified | Persisted job source exists; real-DB live-safe/fault/recovery tests skipped. |
| PRD-LIFE-001 | Fail | Installer destroy verifies status/path, not exact bytes or bound authorization; no post-wipe reconstruction exists. |
| PRD-DEP-001 | Unverified | Compose syntax passes; clean build/up/readiness/restart/teardown did not run. |
| PRD-DEP-002 | Unverified | Installer source/tests exist; no supported Ubuntu VM run is retained. |
| PRD-DEP-003 | Unverified | Provisioning source exists; no live provision/rotate/revoke journey is retained. |
| PRD-DEP-004 | Fail | Digest-as-bearer and TLS/runtime-control gaps plus absent scans block sensitive deployment. |
| PRD-UX-001 | Unverified | Expedition shell source exists; no running-app visual comparison exists. |
| PRD-UX-002 | Unverified | Links are non-linear in source; empty-state keyboard/click navigation did not run. |
| PRD-UX-003 | Unverified | Journey-log API/SSE source exists; no audit parity/reconnect browser artifact exists. |
| PRD-UX-004 | Fail | Summit uses server lifecycle APIs, but export completeness and supported external wipe are not preservation-safe. |
| PRD-UX-005 | Unverified | Static notes exist; offline running-UI dogfood is absent. |
| PRD-UX-006 | Unverified | Technique content exists; browser context/search/link evidence is absent. |
| PRD-UX-007 | Unverified | Guide-style copy exists; invalid-flow browser/accessibility review is absent. |
| PRD-UX-008 | Unverified | Tokens/source checks exist; browser contrast and light/dark screenshots are absent. |
| PRD-UX-009 | Unverified | Reduced-motion CSS exists; browser timing/playback evidence is absent. |
| PRD-A11Y-001 | Unverified | Semantic source checks are not keyboard/accessibility-tree/axe evidence. |
| PRD-A11Y-002 | Unverified | Responsive CSS is not computed-style/contrast/mobile dogfood evidence. |
| PRD-PERF-001 | Fail | No baseline dataset, query plans, or measured API p95/p99 report exists. |
| PRD-PERF-002 | Fail | Streaming/source units exist; no 10 GiB concurrent ingest RSS/ack measurement exists. |
| PRD-PERF-003 | Fail | No measured SSE visibility, warm-route, or local-interaction report exists. |
| PRD-QUAL-001 | Fail | Fifty release-critical tests skipped; platform/security/performance/browser/plugins gates are absent. |
| PRD-QUAL-002 | Unverified | No timestamped desktop/mobile light/dark screenshots or completed operator checklist exists. |
| PRD-DEF-001 | Pass | Current lint/inventory shows no analytical graph surface/dependency. |
| PRD-DEF-002 | Pass | No network-zone/firewall reachability-map surface exists. |
| PRD-DEF-003 | Pass | Only rejected reserved compatibility vocabulary exists; no guided scan catalog/trigger exists. |
| PRD-DEF-004 | Pass | No model provider/call, offensive recommendation, or live-insight UI exists. |
| PRD-DEF-005 | Pass | Findings start through explicit operator promotion; no AI prefill/autopublish exists. |
| PRD-DEF-006 | Pass | No runtime plugin-generation/upload endpoint or UI exists. |
| PRD-DEF-007 | Pass | Hash-only wording/empty hook exists; no key management or signer-identity claim exists. |

## EV-01–EV-16

| Evidence | Status | Current disposition |
|---|---|---|
| EV-01 | Unverified | Core contracts pass; no retained plugins compatibility/parser/runtime report exists. |
| EV-02 | Fail | Real-DB migration/auth tests skipped and digest-as-bearer remains a definite auth defect. |
| EV-03 | Unverified | No human/AI/unknown-tool collector-to-PostgreSQL transcript exists. |
| EV-04 | Unverified | No offline replay or native wrapper/agent platform matrix exists. |
| EV-05 | Pass | Retained startup resolver tests cover auto/manual/off and packet traps prove no manual/off discovery traffic. |
| EV-06 | Unverified | Entity PostgreSQL tests skipped and no merge/split operator journey exists. |
| EV-07 | Unverified | SSE/concurrency PostgreSQL tests skipped and no two-browser recording exists. |
| EV-08 | Unverified | Finding/report tests skipped and no authoritative real-Chromium PDF trace exists. |
| EV-09 | Fail | Dump is incomplete and no production clean-room restore exists. |
| EV-10 | Fail | Installer destroy does not verify exact bytes or consume bound authorization; no post-wipe reconstruction exists. |
| EV-11 | Unverified | Definitions/build syntax exist; no daemon-backed stack or supported Ubuntu VM run is retained. |
| EV-12 | Fail | No measured baseline performance/fault report exists. |
| EV-13 | Fail | Material auth/TLS/control gaps remain and no retained security assessment exists. |
| EV-14 | Unverified | No running-app light/dark desktop/mobile screenshot/checklist set exists. |
| EV-15 | Unverified | No accessibility-tree, keyboard, axe/screen-reader, or reduced-motion browser artifact exists. |
| EV-16 | Pass | Current architecture lint and focused inventory prove all seven narrow v2 deferrals. |

## Final decision

G5 **fails closed**. Waypoint must not be labeled technically complete or v1-ready, and the current
export → teardown flow must not be represented as preservation-safe. Human acceptance is outside
this technical gate and is neither set nor inferred here.
