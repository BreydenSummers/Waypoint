# Complete v1 release acceptance revalidation

Run: `2026-08-12T01:42:15Z`  
Verdict: **Fail — v1 is not releasable.**

The release rule is fail-closed: every one of the 67 PRD traceability rows must have retained,
repeatable runtime evidence. This run found **7 Pass, 16 Fail, and 44 Unverified** rows. Any Fail or
Unverified row blocks G5.

## Evidence boundary and method

The assigned worktree contains core source and the historical core evidence records only. It does
**not** contain retained runtime artifacts from `Waypoint-Plugins` (contract report, wrapper/agent
transcripts, spool replay logs, parser fixtures report, release binaries, or platform matrix). The
sandbox also prohibits entering a sibling worktree. Cross-repository and collector rows are therefore
Unverified rather than inferred from core fixtures.

Core runtime acceptance was also constrained by the supplied host:

- Docker Compose syntax validates, but the Docker daemon is unavailable, so no clean Compose stack
  could run.
- PostgreSQL client/server tools and `WAYPOINT_TEST_PG_DSN` are unavailable. The Go suite reports 32
  passed tests and **24 skipped real-PostgreSQL tests**. Skips include migrations, immutable audit,
  live multi-actor capture, SSE, MCP, entity merge/split, finding promotion, alerts, and fault paths.
- Chromium, Playwright, and axe are unavailable, and no retained browser screenshots, accessibility
  output, or operator-flow recording exists.
- No retained performance, security, clean-host, clean-room restore, or supported-platform report is
  present.

A source implementation or synthetic fixture was used to identify a definite Fail, but never to turn
missing runtime evidence into a Pass. The only Pass rows are narrow v2 deferral checks that can be
established by the retained route/dependency/copy inventory.

## Commands rerun

```sh
python3 scripts/verify-contracts.py
python3 scripts/lint-architecture.py
go test -count=1 -v ./...
go test -count=1 -json ./...       # summarized as 32 pass / 24 skip / 0 executed-test fail
go vet ./...
npm --prefix web test
npm --prefix web run build
go build -o /tmp/waypoint-revalidation-bin/waypoint ./cmd/waypoint
docker compose -f compose.yml config --quiet
docker info --format '{{.ServerVersion}}'
```

Observed:

- Core-only contract verification passed: 8 schemas, 2 generated artifacts, 64 OpenAPI references,
  15 capture cases, 6 event cases, 4 idempotency cases, 9 cursor cases, and 5 problem cases.
- Architecture lint, Go vet, web fixture tests/build, binary build, and Compose config parsing passed.
- Starting the binary without a DSN correctly failed closed with `WAYPOINT_DB_DSN is required`; no
  database-backed production journey could be run.
- `docker info` failed with `Cannot connect to the Docker daemon`.
- Web tests remain source/fixture checks. They do not launch a browser.

These useful checks are not substitutes for the PRD's runtime acceptance artifacts.

## Reconciled PRD rows

The canonical status column is updated in [`../v1-traceability.md`](../v1-traceability.md). This table
records why each row received that status.

| ID | Status | Revalidation evidence |
|---|---|---|
| PRD-CORE-001 | Unverified | Core streaming-store code exists, but the live DB journey skipped and no collector durable-spool/restart artifact from plugins is retained. |
| PRD-CORE-002 | Unverified | Append-only constraints and audit tests exist, but every real-DB immutability/concurrency test skipped. |
| PRD-CORE-003 | Fail | No retained provenance journey spans authoritative Recon, Attacks, and Findings; the source UI remains largely skeleton/fallback content. |
| PRD-CORE-004 | Fail | No operator runtime proves tried/worked/unworked state while retaining failed attempts. |
| PRD-DATA-001 | Unverified | Compose declares one PostgreSQL service, but no clean stack/topology/runtime DB inspection ran. |
| PRD-DATA-002 | Unverified | Migrations exist; all real-PostgreSQL migration and constraint checks skipped. |
| PRD-DATA-003 | Unverified | Evidence is streamed to content-addressed paths in source, but restart/dedup/quota/orphan behavior was not run against the production DB. |
| PRD-DATA-004 | Unverified | Normalization unit cases pass; DB-backed stable-key conflict/concurrency paths skipped. |
| PRD-DATA-005 | Unverified | Merge/split/undo implementation tests skipped and no operator journey is retained. |
| PRD-DATA-006 | Unverified | DB-backed SSE/concurrency tests skipped and there is no two-browser recording. |
| PRD-AUD-001 | Unverified | Contract fields exist, but no retained local/remote collector-to-database transcript proves all attribution fields. |
| PRD-AUD-002 | Unverified | Human/AI live REST test skipped and no plugin wrapper/agent transcript is retained. |
| PRD-AUD-003 | Unverified | Validation/source exists, but live AI provisioning/capture acceptance skipped. |
| PRD-AUD-004 | Unverified | Immutability/reconstruction tests require PostgreSQL and skipped. |
| PRD-CAP-001 | Unverified | No retained wrapper execution matrix from the plugins repository is present. |
| PRD-CAP-002 | Unverified | Core fixtures and skipped DB tests do not prove unknown/parser-failed collector round trips. |
| PRD-CAP-003 | Unverified | Core contract verifier passed; the required matching report from the plugins repository is absent. |
| PRD-CAP-004 | Unverified | No Windows/Linux/macOS wrapper binary/runtime artifacts are retained. |
| PRD-CAP-005 | Unverified | No Linux/macOS disconnect, process restart, reconnect, and original-timestamp replay artifacts are retained. |
| PRD-CAP-006 | Unverified | Core claims preserve the deferral, but no retained plugin-repository inventory proves the offline Windows agent is not claimed for v1 while Windows wrapper support remains. |
| PRD-CAP-007 | Unverified | REST handler exists; idempotency/live capture tests skipped and no collector transcript is retained. |
| PRD-CAP-008 | Unverified | MCP routes exist; the REST/MCP parity test skipped. |
| PRD-CAP-009 | Unverified | Review routes/source exist; the DB lifecycle tests skipped and no operator review journey is retained. |
| PRD-CAP-010 | Fail | The requirement calls for operator documentation; only PRD/planning/ADR text states the residual boundary. |
| PRD-CAP-011 | Unverified | Server retry logic is not enough; no collector spool/ack-loss/restart artifact is retained and DB retry tests skipped. |
| PRD-ID-001 | Unverified | Installer token tests are synthetic; no live provision/revoke/rotate, DB digest inspection, or two-operator journey is retained. |
| PRD-ID-002 | Unverified | AI provision schema/source exists, but real-DB authorization/revocation acceptance skipped. |
| PRD-NET-001 | Unverified | Capture validation stores a supplied address; no multi-interface collector runtime artifact exists. |
| PRD-NET-002 | Unverified | Contract/storage fields exist, but no local/remote collector round trip proves egress and ordered pivots. |
| PRD-NET-003 | Fail | No startup `auto|manual|off` resolver/config implementation or packet assertion exists; only capture-envelope validation exists. |
| PRD-RT-001 | Unverified | SSE source exists; DB cursor/revocation tests skipped and no browser reconnect/resync run is retained. |
| PRD-RT-002 | Unverified | Alert unit fixtures pass, but deduplication/SSE DB test skipped and no live browser alert is retained. |
| PRD-FIND-001 | Unverified | Promotion handler exists; PostgreSQL permission/validation/evidence test skipped and no browser flow is retained. |
| PRD-FIND-002 | Unverified | Schema/source exists; persistence and revision reconstruction were not executed against PostgreSQL. |
| PRD-REP-001 | Fail | A fixture renderer exists, but no production frozen snapshot/PDF pipeline from authoritative engagement records or generated PDF artifact is retained. |
| PRD-REP-002 | Fail | Verifier/regenerator helpers exist, but no exporter creates a DB dump/evidence/PDF bundle and no clean-room restore artifact exists. |
| PRD-REP-003 | Fail | Manifest verification is tested only on synthetic files; no production bundle manifest and outer archive hash were generated. |
| PRD-REP-004 | Fail | Fixture copy has an empty signature hook, but there is no production export/report artifact on which to verify the unsigned claim. |
| PRD-REP-005 | Fail | No live export service implements consistent snapshot, preflight, streaming progress/cancel/recovery, or concurrent capture acceptance. |
| PRD-LIFE-001 | Fail | Installer destroy trusts receipt status/path text without verifying bundle hashes, and no post-wipe self-contained reconstruction was run. |
| PRD-DEP-001 | Unverified | Compose syntax passes, but daemon unavailability prevented a clean one-step stack run. |
| PRD-DEP-002 | Unverified | Installer tests fake systemd/PostgreSQL; no clean Ubuntu 22.04/24.04 VM logs are retained. |
| PRD-DEP-003 | Unverified | Synthetic provisioning passes; no live DB provision/revoke/rotate journey is retained. |
| PRD-DEP-004 | Fail | No TLS enforcement, secure-header/origin middleware, dependency/container scan, or retained restrictive-permission/log review exists. |
| PRD-UX-001 | Unverified | Expedition shell source exists, but no running-app light/dark visual comparison is retained. |
| PRD-UX-002 | Unverified | Links appear non-linear in source; no empty-engagement keyboard/click browser run is retained. |
| PRD-UX-003 | Unverified | Embedded JS contains runtime audit fetch logic, but no audit-to-log parity/reconnect browser evidence exists. |
| PRD-UX-004 | Fail | Summit still advances export/teardown using local timers/state rather than a verified production export lifecycle. |
| PRD-UX-005 | Unverified | Static phase notes exist; required offline running-UI dogfood is absent. |
| PRD-UX-006 | Unverified | Reviewed note content exists; runtime context/search/link coverage and scope review are not retained. |
| PRD-UX-007 | Unverified | No invalid operator-flow copy/accessibility review is retained. |
| PRD-UX-008 | Unverified | Tokens and a source-level contrast note exist; required light/dark screenshots and browser contrast audit do not. |
| PRD-UX-009 | Unverified | Reduced-motion CSS exists; no browser timing/reduced-motion playback evidence is retained. |
| PRD-A11Y-001 | Unverified | Semantic source checks are not a keyboard/accessibility-tree/axe run. |
| PRD-A11Y-002 | Unverified | Responsive CSS is not computed-style, contrast, or mobile viewport dogfood evidence. |
| PRD-PERF-001 | Fail | Tests assert fixture constants/query strings; no baseline dataset, plans, or measured p95/p99 report exists. |
| PRD-PERF-002 | Fail | Streaming code exists, but no 10 GiB/concurrent ingest RSS/ack measurements are retained. |
| PRD-PERF-003 | Fail | No measured SSE latency, warm-route timing, or local-interaction report exists. |
| PRD-QUAL-001 | Fail | Twenty-four release-critical PostgreSQL tests skipped; platform, security, performance, and browser suites are absent. |
| PRD-QUAL-002 | Unverified | No timestamped desktop/mobile light/dark operator screenshots or checklist exists. |
| PRD-DEF-001 | Pass | Core route/dependency/schema/copy inventory and architecture lint show no analytical graph surface. |
| PRD-DEF-002 | Pass | Inventory shows no network-zone/firewall reachability map. |
| PRD-DEF-003 | Pass | Only reserved compatibility vocabulary exists; no guided scan catalog/trigger UI exists. |
| PRD-DEF-004 | Pass | Inventory shows no model dependency/call, recommendation service, or live-insight UI. |
| PRD-DEF-005 | Pass | Findings begin through explicit operator promotion; no AI prefill/autopublish route exists. |
| PRD-DEF-006 | Pass | No runtime plugin-generation/upload endpoint or UI exists. |
| PRD-DEF-007 | Pass | Manifest vocabulary is hash-only with an empty extension hook; no signing/key-management surface exists. |

## Evidence index disposition

| Evidence | Status | Revalidation result |
|---|---|---|
| EV-01 | Unverified | Core contracts pass; no retained plugins-repository compatibility report. |
| EV-02 | Unverified | Production DB wiring now exists, but migration/constraint/auth runtime tests all skipped. |
| EV-03 | Unverified | Live capture test exists but skipped; no wrapper/agent transcript. |
| EV-04 | Unverified | No retained offline replay or required platform matrix. |
| EV-05 | Fail | Startup egress resolver/config and packet evidence are absent. |
| EV-06 | Unverified | Entity DB tests skipped and no operator provenance journey exists. |
| EV-07 | Unverified | Server/browser integration source exists, but no real DB/two-browser recording. |
| EV-08 | Fail | Finding DB test skipped and no authoritative report/PDF artifact exists. |
| EV-09 | Fail | No production bundle creation or clean-room restore log exists. |
| EV-10 | Fail | Destroy does not cryptographically verify the supplied bundle before wipe; no post-wipe reconstruction exists. |
| EV-11 | Unverified | Compose/installer definitions exist; clean daemon/VM logs do not. |
| EV-12 | Fail | No measured performance/fault report exists. |
| EV-13 | Fail | No security test/scan report and material runtime controls remain absent. |
| EV-14 | Unverified | No running-app light/dark desktop/mobile screenshots. |
| EV-15 | Unverified | No accessibility tree, keyboard, axe, screen-reader, or reduced-motion report. |
| EV-16 | Pass | Narrow core route/dependency/copy inventory proves v2 feature deferrals. |

## Release decision

G5 **fails**. The release must not be described as v1-ready while any of the 60 blocking rows remain
Fail or Unverified. Historical evidence records remain useful context but do not override this run.
