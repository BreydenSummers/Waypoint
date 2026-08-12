# restart-003 hostile PRD release gap audit

Run: `2026-08-12T13:36:24Z`  
Verdict: **Fail — v1 is not releasable (7 Pass, 25 Fail, 35 Unverified).**

This re-audit treats source, fixtures, mocked executables, synthetic platform identities, and a
successfully parsed Compose file as implementation clues, not release acceptance. A requirement is
`Pass` only when the complete observable behavior has retained, repeatable evidence. Missing runtime
access remains `Unverified`; a definite implementation omission or contradiction is `Fail`.

## Evidence boundary and host inspection

The assigned worktree contains core source and historical core evidence only. It contains **no
retained `Waypoint-Plugins` runtime report**, wrapper/agent transcript, parser fixture report,
platform binary matrix, or spool-replay log. The trusted sandbox does not permit entering a sibling
repository, so collector and cross-repository claims remain Unverified rather than being inferred
from core contract fixtures.

Host capabilities were inspected without crossing the worktree boundary:

- Docker CLI/Compose are installed (`26.1.5`, Compose `2.26.1-4`), but the daemon socket is
  unavailable. `docker compose config` passes; build/up/restart/down and container inspection cannot
  run.
- `psql`, `pg_dump`, and `pg_restore` are absent. No `WAYPOINT_TEST_PG_DSN` is available.
- QEMU is installed (`10.0.11`), but no VM image exists in the worktree and libvirt has no reachable
  socket. Synthetic installer tests cannot be upgraded into Ubuntu 22.04/24.04 host evidence here.
- Chromium, Firefox, Playwright, Puppeteer, Selenium, and axe are absent. There is no retained
  browser screenshot or accessibility artifact.

The historical `restart-host-runtime-validation.md` accurately records the same Docker/PostgreSQL
blocker; despite “host validation” in its title, it did **not** run the stack. Tool availability is
never converted into a Pass.

## Checks rerun

```sh
python3 scripts/verify-contracts.py
python3 scripts/lint-architecture.py
go vet ./...
npm --prefix web test
npm --prefix web run build
go build ./cmd/waypoint
docker compose -f compose.yml config --quiet
go test -count=1 -json ./...
go test -count=1 -run TestComposeStackPersistsDBAndEvidenceAcrossRestart -v ./...
```

Results:

- Core contract verification passed: 8 schemas, 2 generated artifacts, 64 OpenAPI references, 15
  capture cases, 6 event cases, 4 idempotency cases, 9 cursor cases, and 5 problem cases.
- Architecture lint, `go vet`, web checks, binary build, and Compose syntax passed.
- Web tests report 8 passes, but export tests stage literal `postgres dump bytes`, `evidence bytes`,
  and a fake `%PDF` file. The checks are not production export evidence.
- Go reports **38 passed, 31 failed, and 1 skipped test**. All 31 failures are release-critical
  PostgreSQL gates that fail closed because `WAYPOINT_TEST_PG_DSN` is unavailable. The skipped test
  is the Compose runtime test because the Docker daemon is unavailable.
- The production binary correctly exits when `WAYPOINT_DB_DSN` is missing. A database-backed start
  could not be run.

These are useful development checks. They do not satisfy the missing real-database, collector,
platform, browser, security, performance, export, or teardown gates.

## Hostile findings

### 1. Export and teardown are still simulated/local-only

A production report snapshot and PDF route now exists in `internal/server/report.go`; that is a real
improvement over the older audit. It is **not** the required export lifecycle:

- there is no export job/API/CLI that freezes a PostgreSQL snapshot, runs `pg_dump`, copies all
  evidence, writes metadata/tools, creates an archive, emits a complete manifest and outer hash, or
  persists a verified receipt;
- `web/scripts/bundle-export.test.mjs` manufactures dump/evidence/PDF bytes in a temp directory;
  “concurrent capture” is an unrelated file write, and “wipe” removes that temp fixture;
- Summit fetches only report JSON and PDF, hashes those two responses, and then spreads IDs,
  manifest hash, and notes from the static `reportSnapshotFallback` fixture into a browser-local
  receipt;
- the browser destroy button only sets `state.teardownState = 'destroyed'`;
- installer destroy parses receipt JSON and compares a path, but never verifies manifest entries,
  the bundle bytes, or the outer archive hash before deleting roots.

Consequently PRD-REP-002–005, PRD-LIFE-001, and PRD-UX-004 remain Fail. PRD-REP-001 also remains
Fail: no authoritative generated PDF is retained, its only Chromium test uses a fake shell script,
and the report routes do not authenticate or scope the caller. The container runs Chromium as root
without retained proof that the selected command line works.

### 2. Production API inventory is incomplete or mislabeled

Implemented server surfaces include REST capture, audit history/SSE, findings, entity merge/split,
out-of-band *resolution*, and report JSON/PDF. Missing or incomplete v1 surfaces include:

- no actor/account bootstrap, list, revoke, or rotate API;
- no authoritative paginated actions/Attacks API, entity/Recon list and provenance API, or evidence
  retrieval API for the workspaces;
- no export/preflight/progress/cancel/resume/receipt API and no teardown authorization API;
- no out-of-band detector, claim creation path, pending queue, or list API — only an endpoint that
  accepts a caller-invented claim ID and resolves it;
- `/api/v1/mcp/*` are ordinary custom HTTP handlers. There is no MCP initialize, tools/list,
  tools/call, JSON-RPC, or standard transport implementation. Naming a REST alias “MCP” is not an
  official MCP endpoint.

PRD-CAP-008 and PRD-CAP-009 are therefore Fail, not Unverified.

### 3. Audit schema drops required attribution semantics

The capture envelope validates more detail than the authoritative schema retains. `action` stores a
source agent UUID, exec IP, optional egress IP, and pivots, but drops source-agent kind/name/version/
OS/architecture, exec-host method/interface/confidence, egress mode/status/observed time, execution
status/signal/failure code, and explicit clock-skew metadata. The audit event keeps only some of this
and cannot reconstruct every action field promised by the PRD. Reports collapse disabled and failed
egress into “not recorded.”

PRD-DATA-002, PRD-AUD-001, and PRD-NET-001/002 are definite Fail. Human/AI parity and collector
fidelity remain Unverified because no collector-to-database transcript exists.

### 4. Egress startup policy does not exist

There is no `WAYPOINT_EGRESS_*` startup config, resolver, cache, allowlisted endpoint, mode-specific
collector configuration, or packet assertion. Capture merely trusts an envelope that claims
`auto|manual|off`. No evidence proves manual/off make zero discovery traffic. PRD-NET-003 and EV-05
remain Fail.

### 5. Web/runtime evidence is stale, split, and synthetic

`web/src/App.tsx` and the served `internal/webassets/dist/assets/waypoint.js` are different
implementations. `npm run build` copies HTML/CSS, checks that the already-committed JS exists, and
never compiles TypeScript. Source edits can therefore pass while stale handwritten/prebuilt JS is
served.

The embedded JS does fetch audit/findings and stream SSE using a bearer token, but Recon and Attacks
remain derived/fallback cards rather than authoritative phase APIs, and fallback journey/report data
can appear as engagement facts. No browser run proves empty/populated/error/conflict/reconnect
behavior. The required desktop/mobile light/dark screenshots, keyboard transcript, accessibility
tree, axe report, screen-reader result, contrast computation, and reduced-motion playback are all
absent. PRD-CORE-003/004 remain Fail; UX/A11Y rows remain Unverified except the simulated Summit
flow, which is Fail.

### 6. Performance remains unmeasured

`TestPerformanceProfileFixtureSeedsBaselineAndFaultScenarios` checks configured constants and
`TestAuditQueryShapeRemainsKeysetBounded` searches source text. No 4-vCPU/8-GiB baseline with 10
operators, 100k actions, 1m events/observations, and 10 GiB evidence ran. There are no query plans,
percentiles, heap/RSS profiles, SSE/browser timings, export duration, fault injection, soak, or
capacity report. The API also caps each stdout/stderr part at 32 MiB, so the documented 10 GiB
streaming scenario has no demonstrated upload/chunking path. PRD-PERF-001–003 remain Fail.

### 7. Security controls are materially incomplete

Definite source findings include:

- report JSON/PDF endpoints accept an engagement ID without authentication or engagement checks;
- `lookupActor` deliberately accepts the stored 64-hex token digest itself as a bearer credential;
- capture, entity merge/split, and out-of-band review do not enforce the owner/operator/viewer role
  matrix after authentication;
- the app exposes plain HTTP on all interfaces in default Compose, with no TLS-outside-loopback
  startup rejection;
- no CSP, HSTS, frame policy, nosniff header, strict origin policy, global request/rate limit, or
  retained IDOR/XSS/SSRF/archive/dependency/container/secret scan exists;
- multipart envelope bytes are read without an explicit envelope limit before decoding;
- no operator documentation states the out-of-band, host-admin, `egress=off`, unsigned-manifest,
  and host/disk-encryption residual boundaries.

PRD-ID-001, PRD-DEP-004, PRD-CAP-010, and EV-13 remain Fail. A security gate must not wait until the
end: report IDOR and digest-as-token acceptance are release-stop defects.

### 8. Deployment and OS claims are unsupported

Compose now declares one app and one PostgreSQL container with named volumes, but it was never built
or started on this host. PRD-DEP-001 remains Unverified.

The installer advertises Ubuntu 22.04/24.04 x86_64, while every retained test replaces `systemctl`,
`psql`, and `pg_isready` with stubs and substitutes synthetic `/etc/os-release`/machine values. More
importantly, a fresh local install installs PostgreSQL and waits on the configured Waypoint DSN but
never creates the Waypoint database, role, or password first. It has no live revoke/rotate journey.
PRD-DEP-002/003 and EV-11 are Fail. QEMU's presence without an image or hypervisor is not host
support evidence.

Compose support on Linux/macOS/Windows, wrapper support on Windows/Linux/macOS, agent support on
Linux/macOS, and current/previous Chrome/Firefox/Edge/Safari remain unsupported claims until the
retained native/VM/browser matrices run. No offline Windows-agent claim was found in core, but the
plugins repository inventory is unavailable, so PRD-CAP-006 remains Unverified.

### 9. Retained plugin evidence is absent

Core's synthetic contracts do not prove plugin matching specificity, parser isolation, raw fallback
through a real collector, durable spool/ack loss, original timestamps after replay, native release
binaries, or supported OS behavior. EV-01 is Unverified (both-repository report absent) and EV-04 is
Unverified. This is an evidence boundary, not a waiver.

## Traceability reconciliation

`docs/v1-traceability.md` is updated to this audit. The 67 rows now resolve to:

- **Pass: 7** — only narrow v2 deferral inventories;
- **Fail: 25** — definite implementation or acceptance contradictions;
- **Unverified: 35** — plausible implementation exists, but required runtime/platform/browser or
  sibling-repository evidence is absent.

Rows changed from Unverified to Fail in this audit:

- PRD-DATA-002 and PRD-AUD-001: authoritative schema loses required attribution fields;
- PRD-CAP-008/009: no MCP protocol server and no out-of-band detection/queue;
- PRD-ID-001: stored digest is a usable bearer credential and role controls are incomplete;
- PRD-NET-001/002: exec/egress observation semantics are not retained;
- PRD-DEP-002/003: supported-host install/provisioning is synthetic and a fresh local DB is not
  created.

No row was promoted to Pass.

## Evidence index disposition

| Evidence | Status | Disposition |
|---|---|---|
| EV-01 | Unverified | Core contract verifier passes; plugins report absent and custom HTTP aliases are not MCP protocol evidence. |
| EV-02 | Fail | 31 PostgreSQL tests fail closed; schema/auth defects are definite. |
| EV-03 | Fail | No collector transcript and required source/egress semantics are dropped. |
| EV-04 | Unverified | No retained offline replay or native platform matrix. |
| EV-05 | Fail | No startup egress policy or packet assertions. |
| EV-06 | Unverified | DB tests did not run and no merge/split operator journey exists. |
| EV-07 | Unverified | DB tests did not run and no two-browser SSE recording exists. |
| EV-08 | Fail | Report route exists but no authenticated authoritative PDF artifact/journey exists. |
| EV-09 | Fail | Fake staged bytes and helper scripts are not a production bundle/restore. |
| EV-10 | Fail | Browser local state and receipt-path parsing are not verified teardown. |
| EV-11 | Fail | Compose runtime absent; installer is stub-tested and cannot initialize its fresh local DB. |
| EV-12 | Fail | No measured performance/fault report. |
| EV-13 | Fail | Material security controls and operator boundary docs are absent. |
| EV-14 | Unverified | No browser screenshots or completed UX checklist. |
| EV-15 | Unverified | No accessibility/browser artifact. |
| EV-16 | Pass | Narrow core inventory still proves v2 deferrals only. |

## Remediation architecture and sequencing

The machine-readable task graph is returned in `AUTOPILOT_OUTCOME`. Integration decisions for that
graph are:

1. Freeze corrected contracts/schema and a deterministic web build first. Do not build UX or plugin
   releases against mislabeled MCP or lossy attribution contracts.
2. Close core identity, authorization, egress, read-API, MCP, out-of-band, and deployment gaps before
   accepting collector/UI integration.
3. Implement export as a server-owned auditable job with persisted state; the browser is a client,
   never the receipt authority. Teardown verifies the actual bundle independently.
4. Require independent core and plugins implementation reviews before security and platform gates.
5. Run real PostgreSQL, native collector, clean VM/Compose, clean-room export, security,
   performance/fault, and browser UX/accessibility gates independently.
6. Final review remains fail-closed until all 67 trace rows link to retained artifacts.

No owner question is required: the remaining issues are implementation and evidence gaps under
already locked PRD decisions, not unresolved product policy.
