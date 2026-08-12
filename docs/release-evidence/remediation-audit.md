# v1 release remediation audit

## Verdict

**Fail — the merged core tree is not a runnable v1 product.** It contains useful contracts, schema
work, API handler implementations, a static UI shell, and test scaffolding, but the production
binary starts without PostgreSQL, reports ready anyway, and returns `503` for capture and audit.
There is no Dockerfile or Compose file. Raw evidence bytes are read into memory and discarded. The
browser is fixture-driven and does not call the API or open SSE. Export, PDF, bundle, restore, and
teardown are simulated/static rather than operational.

This audit applies the PRD verification rule literally: fixtures, static state, source-shape tests,
and prose are not runtime acceptance evidence. A row that cites an unavailable tool, missing sibling
repository, proxy, or other boundary is not passed.

## Method and status meanings

- **Pass:** direct source/runtime evidence demonstrates the complete row with a reproducible command.
- **Fail:** a required artifact/path is absent, runtime behavior contradicts the requirement, or the
  claimed evidence is only a fixture/proxy.
- **Unverified:** implementation may exist outside the available boundary, but the required runtime,
  platform, browser, or sibling-repository evidence was not available. Unverified is release-blocking.

The assigned tree's `.git` file points to
`/opt/waypoint-project/Waypoint/.git/worktrees/core-v1-release-remediation-audit`, which is not
available in this sandbox. `git status --short` therefore fails. File inventory and ignore rules were
used for artifact review; no claim below depends on hidden Git history.

## Host/tool observations

Run from the core repository root:

```sh
pwd
for x in git docker psql pg_dump pg_restore go node npm chromium playwright axe; do
  printf '%-12s' "$x"; command -v "$x" || true
done
find . -maxdepth 2 -type f \( -iname '*compose*' -o -iname 'Dockerfile*' \) -print
git status --short
```

Observed: working path `/workspace`; Docker CLI, Go, Node, and npm exist; `psql`, `pg_dump`,
`pg_restore`, Chromium, Playwright, and axe do not; there are no Compose/Dockerfile results; Git
fails because the worktree metadata target is unavailable. Tool unavailability is recorded as
**Unverified**, never converted into a pass.

## Hostile findings and PRD traceability

| Area | Status | PRD IDs | Runtime/source evidence and release consequence |
|---|---|---|---|
| PostgreSQL wiring and migrations | **Fail** | PRD-DATA-001/002, PRD-CORE-002 | `cmd/waypoint/main.go` calls `server.Handler()` with a nil DB. Production code has no `sql.Open`, `ApplyMigrations`, DB ping, migration ledger, or DB-aware readiness. `HandlerWithDB` is test-only. Real-Postgres tests skip when `WAYPOINT_TEST_PG_DSN` is absent. A live binary reports ready while all data paths are unavailable. |
| One-step Compose | **Fail** | PRD-DEP-001 | No Dockerfile or Compose file exists. `docker compose config` returns `no configuration file provided: not found`. There is no app image, PostgreSQL service, evidence volume, health dependency, config validation, or fresh-engagement bootstrap. |
| Supported-host installer | **Fail** | PRD-DEP-002/003 | The installer copies a supplied binary and emits systemd/SQL files; it does not install/start PostgreSQL, migrate the database, install/enable/start the service, or validate a live app. Its only test sets `WAYPOINT_INSTALLER_SKIP_DB=1` and uses a shell-script fake binary. If `psql` is absent, provisioning logs and continues. |
| Authenticated multi-actor capture | **Fail** | PRD-ID-001/002, PRD-AUD-001/002/003, PRD-CAP-001/002/007 | API handler code has token and actor logic, but the production runtime cannot use it and live capture returns `503`. No wrapper exists in core and no real two-human/AI transcript exists. Role enforcement is incomplete: actor lookup checks revocation but capture/entity reads and writes do not enforce the declared owner/operator/viewer matrix. |
| Raw evidence durability | **Fail** | PRD-CORE-001, PRD-DATA-003, PRD-CAP-002/011, PRD-PERF-002 | `readCaptureRequest` uses `io.ReadAll` with 32 MiB per-part limits. `upsertEvidence` writes only DB metadata and a synthetic `storage_key`; stdout/stderr bytes are never written to a filesystem or other durable store. Successful handler-level ingest would therefore lose the evidence it claims to retain. |
| Offline replay and collectors | **Unverified** | PRD-CAP-004/005/011 | No wrapper, remote agent, spool, reconnect harness, release binaries, or platform artifacts exist in this tree. The sibling plugin repository is not available to this task. Core idempotency unit/integration source cannot substitute for disconnect/process-restart replay on Linux/macOS. |
| Cross-platform plugins | **Unverified** | PRD-CAP-003/004/005 | Core contracts validate synthetic fixture shapes, but there is no evidence from Windows/Linux/macOS wrapper runs or Linux/macOS remote-agent runs, and no sibling-repository plugin fixtures/binaries were inspectable. |
| Egress modes | **Fail** | PRD-NET-001/002/003 | Capture validation accepts mode/status fields, but there is no startup `auto|manual|off` resolver/config implementation and no packet/network assertion. Client-supplied attribution is not proof that prohibited discovery traffic is absent. |
| MCP and out-of-band review | **Fail** | PRD-CAP-008/009/010 | No MCP endpoint/adapter or out-of-band claim/review route is implemented. MCP appears only in contracts, fixtures, enums, and prose. |
| Browser SSE/concurrency | **Fail** | PRD-DATA-006, PRD-RT-001/002, PRD-UX-003 | Server SSE handler source exists, but the browser has no `EventSource`, `fetch`, or `/api/v1/` usage. Journey-log rows are constants. DB-backed SSE tests skip and no two-browser recording exists. Thus browser concurrency, reconnect, resync, revocation, and live alerts are not demonstrated. |
| Recon/Attacks/Findings runtime UX | **Fail** | PRD-CORE-003/004, PRD-UX-001–007, PRD-FIND-001/002 | Workspaces are descriptive cards over static demo state, not views of authoritative engagement records. There is no UI capture journey, evidence drill-in, entity merge/split operation, finding promotion/editor, optimistic conflict flow, or real audit-to-log parity. |
| PDF/report | **Fail** | PRD-REP-001, PRD-FIND-001/002 | The report route is hard-coded `reportSnapshot` data. `Print PDF` calls `window.print()`. The optional script assumes an externally supplied Chromium that is unavailable and not pinned/shipped. No report is generated from PostgreSQL/audit records. |
| Bundle/export/restore | **Fail** | PRD-REP-002/003/004/005 | Manifest paths, sizes, hashes, receipt, malicious paths, and restore tool names are fixtures. No export API/CLI, `pg_dump`, archive writer, evidence copier, hash verifier, restore tool, progress/cancel recovery, or clean-room restore exists. Fixture checks only assert strings and JSON shape. |
| Teardown | **Fail** | PRD-LIFE-001, PRD-UX-004 | Summit uses timers to advance local React state and a button only changes `teardownState` to `destroyed`. No verified bundle receipt is persisted and no app/DB/evidence volumes are removed. |
| Accessibility and visual evidence | **Unverified** | PRD-UX-008/009, PRD-A11Y-001/002, PRD-QUAL-002 | Source includes semantic links/buttons, focus CSS, breakpoints, themes, and reduced-motion CSS, but there are no retained light/dark desktop/mobile screenshots, browser accessibility tree, keyboard transcript, screen-reader result, axe report, or contrast measurements. Static source checks do not pass the dogfood gate. |
| Performance/fault tolerance | **Fail** | PRD-PERF-001/002/003, PRD-QUAL-001 | `TestPerformanceProfileFixtureSeedsBaselineAndFaultScenarios` checks configured constants; `TestAuditQueryShapeRemainsKeysetBounded` checks source strings. No seeded 100k/1m/10 GiB run, latency percentiles, RSS profile, query plans, browser timings, export duration, fault injection, or soak evidence exists. Current ingest buffers up to 64 MiB across stdout/stderr and cannot accept the specified 10 GiB evidence case. |
| Security | **Fail** | PRD-DEP-004, PRD-QUAL-001, PRD-CAP-009/010 | No TLS enforcement, secure-header middleware, origin policy, rate limits, IDOR suite, dependency/container scan, archive tests, or secret/log scan report exists. Live HTML lacks CSP, HSTS, X-Content-Type-Options, and related headers. `lookupActor` accepts a 64-hex stored token digest directly as a bearer credential. Installer state writes a DB DSN to files without explicit restrictive modes. Architecture lint is not a security test. |
| Explicit v2 deferrals | **Pass** | PRD-DEF-001–007 | Source/route inventory and architecture lint show no functioning graph, zone-map, scan-library, offensive-LLM, AI finding-prefill, plugin-generation, or cryptographic-signing product surface. Reserved enums and clearly labeled fixtures are not functioning features. |
| Clean-host reproducibility | **Fail** | PRD-DEP-001/002, PRD-QUAL-001 | There is no clean-host deployment definition or retained clean-host transcript. The web “build” copies only `index.html` and merely checks that prebuilt JS/CSS already exist; it does not compile `web/src`, so a source checkout depends on committed generated assets and can silently serve stale code. |

## Reproducible runtime checks

### Production binary is ready without its database

```sh
mkdir -p bin
go build -o bin/audit-waypoint ./cmd/waypoint
WAYPOINT_ADDR=127.0.0.1:18081 \
WAYPOINT_DB_DSN='postgres://waypoint:waypoint@127.0.0.1:5432/waypoint?sslmode=disable' \
  bin/audit-waypoint >bin/audit-runtime.log 2>&1 &
pid=$!
for i in $(seq 1 30); do curl -fsS http://127.0.0.1:18081/readyz >/dev/null && break; sleep .1; done
curl -i http://127.0.0.1:18081/readyz
curl -i -X POST -H 'Authorization: Bearer fake' \
  -H 'Waypoint-Contract-Version: 1.0.0' \
  http://127.0.0.1:18081/api/v1/captures
curl -i -H 'Authorization: Bearer fake' \
  -H 'Waypoint-Contract-Version: 1.0.0' \
  http://127.0.0.1:18081/api/v1/audit-events
kill "$pid"
```

Observed: readiness is `200 {"status":"ready"}`; capture and audit are both `503
service_unavailable`. Setting `WAYPOINT_DB_DSN` has no effect because the executable never reads it.

### PostgreSQL tests are skipped, not passed

```sh
unset WAYPOINT_TEST_PG_DSN
go test -v ./...
```

Observed: migration, append-only DB, auth, capture, SSE, entity, finding, and notable-alert
integration tests report `SKIP: WAYPOINT_TEST_PG_DSN not set`; the aggregate command still exits 0.
This invalidates the prior real-database evidence claim for this run.

### Compose is absent

```sh
find . -maxdepth 2 -type f \( -iname '*compose*' -o -iname 'Dockerfile*' \) -print
docker compose version
docker compose config
```

Observed: no files; Compose v2 is installed; config exits 14 with `no configuration file provided:
not found`. Docker daemon availability does not change the missing-product-artifact failure.

### UI is disconnected from runtime data and lacks security headers

```sh
grep -nE 'EventSource|fetch\(|WebSocket|/api/v1/' web/src/App.tsx || true
curl -sSI http://127.0.0.1:18081/engagements/demo/attacks
```

Observed: the grep has no results. The HTML response has basic content headers only; no CSP, HSTS,
X-Content-Type-Options, or framing policy is present.

### Export/collector implementation inventory

```sh
find . -maxdepth 4 -type f | \
  grep -Ei '(wrapper|agent|spool|offline|export|bundle|restore|destroy|teardown|pdf|mcp|compose|docker)' || true
command -v psql pg_dump pg_restore chromium playwright axe || true
```

Observed: only export-semantics prose matched in core; required operational tools are absent from the
host. More importantly, no core implementation invokes them or ships equivalents.

## Release-evidence disposition

The former `docs/release-evidence/v1-037.md` labels all rows as passed, but repeatedly substitutes
unit/static proxies and explicitly admits missing boundaries. Those labels are unsupported.
`docs/v1-traceability.md` is corrected as follows:

| Evidence | Status | Reason |
|---|---|---|
| EV-01 contracts in both repos | **Unverified** | Core contract verifier passed, but no sibling-repo report/runtime was available; MCP runtime is absent. |
| EV-02 migrations/auth isolation | **Fail** | Real-DB tests skipped and production has no DB wiring/migration call. |
| EV-03 capture transcript | **Fail** | No live human/AI/wrapper transcript; live core returns 503; evidence bytes are discarded. |
| EV-04 offline/platform | **Unverified** | Missing sibling repo and retained platform runs. |
| EV-05 egress packets | **Fail** | No resolver/config implementation or packet assertion. |
| EV-06 entity provenance | **Unverified** | Relevant DB tests skipped; no live operator journey. |
| EV-07 browser SSE | **Fail** | Browser has no SSE/API integration; no recording. |
| EV-08 finding/report trace | **Fail** | Report and UI are static fixtures, not runtime data. |
| EV-09 bundle/restore | **Fail** | No bundle or restore implementation/log. |
| EV-10 teardown/post-wipe | **Fail** | UI state simulation is not teardown. |
| EV-11 Compose/installer | **Fail** | Compose absent; installer uses fake binary and skips DB. |
| EV-12 performance/fault | **Fail** | Constants and query-source checks are not benchmarks/fault runs. |
| EV-13 security | **Fail** | Architecture lint is not security evidence and material controls are absent. |
| EV-14 screenshots/UX | **Unverified** | Browser screenshot tooling/evidence absent. |
| EV-15 accessibility | **Unverified** | Browser accessibility/axe evidence absent. |
| EV-16 deferrals | **Pass** | Direct source/route/dependency inventory supports the narrow anti-scope claim. |

No release may be declared while any row is Fail or Unverified.

## Generated binary and artifact hygiene

```sh
file waypoint-bin internal/webassets/dist/assets/waypoint.js \
  contracts/v1/generated/contract.schema.json
wc -c waypoint-bin internal/webassets/dist/assets/waypoint.js \
  internal/webassets/dist/assets/waypoint.css contracts/v1/generated/*.json
sha256sum waypoint-bin
grep -n '^waypoint-bin$' .gitignore || true
```

Repository-tree artifacts identified:

- **`waypoint-bin` — remove.** A 9,925,427-byte, unstripped x86-64 ELF with debug information
  (`sha256 51397c6f643c456c8cdc3816b0ef20c04c942680cdc06d93cf6aa3e0baef1f4d`). It is a generated
  host-specific binary at repository root and is not ignored by `.gitignore`; it is inappropriate in
  the merged source tree.
- **`internal/webassets/dist/**` — generated, intentionally embedded but reproducibility-blocking in
  its current form.** The committed-tree JS/CSS are required by `//go:embed`, while `npm run build`
  does not compile TypeScript/CSS and only checks those files already exist. Either generate them
  deterministically from `web/src` in build/CI or document and verify generated-source policy; do not
  treat their presence as proof of a web build.
- **`contracts/v1/generated/*.json` — generated contract artifacts.** These are explicitly marked
  generated and verified by `scripts/verify-contracts.py`; retaining them can be valid for cross-repo
  compatibility, provided CI regenerates and rejects drift.

No other binary/temp/log artifact was present in the supplied tree inventory. Audit-created `bin/`
outputs were removed after runtime checks.

## Checks that do pass (limited scope)

These are useful development checks but do not change the release verdict:

```sh
python3 scripts/verify-contracts.py
python3 scripts/lint-architecture.py
npm --prefix web test
npm --prefix web run build
go test ./...
```

Observed: all exit 0. Qualifications: core-only contracts; architecture/claim lint only; static
fixture/source web checks; a no-op-style web build over prebuilt assets; and Go results with all
PostgreSQL integration paths skipped.

## Proposed remediation graph

The graph is ordered to make each task independently demonstrable. The exact machine-readable graph
is returned in `AUTOPILOT_OUTCOME`.

1. Wire production PostgreSQL startup/migrations/readiness before any feature acceptance.
2. Add deterministic app/PostgreSQL Compose and real provisioning.
3. Make evidence durable and streaming; then validate live REST/MCP capture.
4. Implement collectors in the plugin repository and retain the required platform matrix.
5. Connect browser workspaces/SSE to authoritative APIs.
6. Implement real report/PDF/export/bundle/restore/teardown lifecycle.
7. Independently run deployment, security, performance, UX/accessibility, and final clean-host gates.
