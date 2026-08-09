# Waypoint — Product Requirements Document (v1)

## Context

Waypoint is a self-contained, end-to-end platform for running security assessments (initial
target: schools / university & K-12 networks). It exists for two reasons that sit at its core:

1. **Capture everything** from every tool across the whole engagement.
2. Keep a **clean, attributable audit trail** so the final client report is defensible and
   traceable — the report falls *out of* the data rather than being reconstructed by hand.

Around that core, later versions add an offensive-focused LLM, a BloodHound-style node graph, and a
guided scan library. Those are explicitly **features, not the headline**. Everything lives in a
fast, "cosy," trail-based UI built for short assessment windows.

This PRD scopes **v1 = capture + audit + the three-phase model + report export**, with graph /
LLM / scan library deferred to v2+ (confirmed with the product owner). Two GitHub repos already
exist and are currently empty (LICENSE only): `BreydenSummers/Waypoint` (core app) and
`BreydenSummers/Waypoint-Plugins` (parser plugins).

---

## Decisions locked in (from bulletproofing session)

| Area | Decision | Rationale |
|---|---|---|
| **Persistence vs. disposability** | Instance is disposable. Before teardown, export a **self-contained engagement bundle** (DB dump + evidence + PDF report + SHA-256 hash manifest). Running instance holds nothing long-term. | Resolves the disposability-vs-auditability contradiction: you can wipe freely because the defensible record already left the box. |
| **Deployment / users** | **Team, shared instance** (on engagement LAN or jump box). Multiple concurrent operators. | Matches "what have *we* tried." Requires per-operator attribution, concurrency-safe writes, and near-real-time UI updates. |
| **Data store** | **PostgreSQL**, single instance, one container. Graph layer (v2) lives in the *same* store (recursive queries / Apache AGE). | Concurrent multi-writer safe; disposable in a container; BloodHound CE already moved to Postgres, so no separate Neo4j in v2. |
| **No-plugin capture** | **Raw capture always**, plus unrecognized tools are flagged `needs-plugin` to feed the (v2) AI plugin-generation loop. | Nothing is ever lost — the audit-everything promise holds even for tools with no parser. |
| **Attacker IP** | Per action, record: (a) executing-host local IP, (b) **public/NAT egress IP**, (c) optional pivot/proxy chain. | The IP that actually touched the client is often not the operator's box; the client needs the true source that hit their systems. |
| **LLM** | **Configurable per engagement**: hosted API (redacted) *or* local/offline model. v2 feature. | Client data-handling rules vary; some engagements forbid any data egress. |
| **Report output** | **PDF (human-readable) + raw machine-readable bundle**, both generated from the audit trail. | Schools need a readable report plus a records copy. |
| **Remote collection** | **Tiny agent** on the remote host: captures locally, buffers to disk when offline, syncs to Waypoint when reachable. | Engagement networks are segmented/flaky; direct push would drop data. |
| **Findings flow** | v1: **operator manually promotes** an attack → finding and fills report fields; evidence auto-links to the captured action. v2: **AI pre-fills** candidate findings for operator confirmation. | Defensibility requires a human in the loop; AI extraction accelerates but never auto-publishes. |
| **Setup / spin-up** | Support **all three**: `docker compose up` (default disposable path), an **install script** for supported hosts (handles config), and **account provisioning**. | Different environments; compose for throwaway, script for configured hosts. |
| **Operator auth & attribution** | **No shared token.** Each operator joins with their **own named token** (or a provisioned account). Every captured action carries the operator identity. | A shared link would make the audit trail anonymous — fatal to the core promise. This directly answers the owner's open question. |
| **Real-time updates** | **Server-sent events (SSE)** push new actions/findings to connected operators; writes stay normal REST. | Fits a read-heavy live feed; simple, fast, auto-reconnect. No websocket complexity unless live collaborative editing is added later. |
| **Egress IP detection** | **Startup config: `auto` \| `manual` \| `off`.** `auto` resolves the public/NAT egress IP (STUN or a Waypoint-controlled listener); `manual` uses operator-declared IP(s); `off` records none. | Auto is a usability win; `manual`/`off` respect engagements with a no-egress rule. |
| **Windows support (v1)** | The **operator terminal wrapper is cross-platform (Windows/Linux/macOS) in v1**; the offline-buffering **remote agent is Linux/macOS in v1, Windows as a fast-follow**. | Operators run Windows tooling in AD-heavy school networks, so the wrapper must; the heavier buffering agent can trail slightly. |
| **Bundle integrity** | **Hash manifest (SHA-256 of every artifact)** included in the export for tamper-evidence. Cryptographic **signing deferred** until a client requires it (hook left in place). | Integrity with zero key-management overhead; real signing can be added without reworking the export. |
| **AI agents are first-class actors** | Any AI agent used during the engagement is registered as an `actor` of kind `ai_agent` and gets its **own token**; its actions flow through the **same collection path** and are captured/attributed exactly like a human's (plus model/version and the human who authorized it). Applies to external AI tooling *and* Waypoint's own v2 offensive LLM. | Closes the audit blind spot: an AI running commands during an assessment must be as traceable as an operator, or the client-facing "what happened where" is incomplete. |

---

## Guiding principles (constraints on every feature)

- **Performance is paramount** — every view, query, and import must feel instant. Slow is a
  dealbreaker. (Drives: indexed Postgres schema, server-side pagination, streaming ingest.)
- **Bulletproof by default** — everything is covered by tests; we do not debug on-site.
- **Dogfooded design loop** — during development, features are not "done" when tests pass. The AI
  agent must actually **run the app, use the feature as an operator would, and visually inspect it**
  (drive the real UI / screenshot it), then judge whether the flow and look make sense before
  calling it complete. Automated tests prove correctness; this loop proves it's usable and cosy.
- **Disposable & trivial to stand up** — near-instant fresh setup, no lingering state from the last
  client.
- **Trail-based & cosy** — a calm, well-designed app for hacking; not enterprise vuln-management.
- **Auditable end to end** — every action timestamped + attributed *at collection time*,
  **regardless of whether a human or an AI agent initiated it**. If an AI acts during the
  engagement, its actions are captured and attributed identically (see "AI-actor capture").
- **AI woven in, not bolted on** (v2) — insights generate on the fly as data arrives.

---

## Core data model (the three phases = the audit trail)

Every item lands in one phase and is stored with **source, operator, timestamp, target, and
result**. That storage IS the audit trail; the reporting layer reads straight off it.

1. **Recon** — everything discovered about the environment.
2. **Attacks** — everything tried, grouped into subgroups (by technique / target / host).
3. **Findings** — confirmed, reportable results (promoted from Attacks).

**Central always-answerable question:** *"What have we tried, what worked, and what's left?"* — the
UI must surface engagement state at a glance and never lose an attempt.

### Proposed schema (v1, Postgres)

- `engagement` — id, name, client, scope, created_at, status.
- `actor` — id, engagement_id, **kind (`human` | `ai_agent`)**, handle, token_hash, role; for AI
  actors also `agent_name`, `model`, `version`, and `authorized_by` (the human/actor who launched
  it). **Every action attributes to exactly one actor** — human or AI, no exceptions.
- `action` (the audit spine) — id, engagement_id, **actor_id**, **initiated_by (`manual` | `ai` |
  `scan-library`)**, phase, command, argv, cwd, **exec_host_ip**, **egress_public_ip**,
  **pivot_chain (jsonb)**, target, started_at, ended_at, exit_code, raw_stdout (ref),
  raw_stderr (ref), plugin_id (nullable), parse_status (`parsed` | `needs-plugin` | `raw`),
  source_agent_id, **decision_context (jsonb, nullable)** — for AI-initiated actions, the
  rationale / prompt reference behind the action, so the *why* is auditable too.
- `entity` (recon nodes) — id, engagement_id, kind (host/service/cred/domain/...), identity keys,
  attributes (jsonb), first_seen, last_seen. **Identity/dedup rule (v1 decision):** prefer stable
  identifiers as the entity key when available — **MAC address, AD SID, FQDN** — falling back to a
  normalized (hostname, ip) pair only when no stable key exists. New observations merge into an
  existing entity when a key matches; a manual **merge/split** control resolves the ambiguous cases
  (DHCP churn, shared hostnames). This keeps "what have we tried against host X" correct without
  waiting for the graph.
- `structured_result` — id, action_id, plugin_id, extracted (jsonb), links to entities discovered.
- `finding` — id, engagement_id, title, severity, affected_entities, evidence_action_ids,
  remediation, status, promoted_by, promoted_at.
- `evidence_blob` — content-addressed store for raw output / screenshots (kept out of the row for
  performance; referenced by hash).

---

## Collection layer (`Waypoint-Plugins` repo)

- **Terminal wrapper** — wraps any command, captures stdout/stderr/exit/timing, and tags it with
  what ran, when, from where (exec host + egress IP + pivot chain), by whom, and against what. The
  tagging is what makes the audit trail free.
- **Raw-first** — capture is never gated on having a parser. Unknown tools → stored raw + flagged
  `needs-plugin`.
- **Plugin contract (proposed, so AI-generation has a fixed target):** a plugin declares
  (1) a **match rule** (by binary name + optional argv/regex heuristics), (2) a **parse function**
  `raw_output -> structured_result + discovered entities`, (3) a **schema** for its structured
  output, and (4) **example fixtures** (sample raw output → expected parse) that double as its
  tests. Selection: match on binary name first, then argv heuristics; ties resolved by specificity.
- **Cross-OS** — must work broadly (Linux/macOS/Windows), not just Kali.
- **Large library** — value scales with coverage; treat plugin count as a first-class metric.
- **Remote agent** — tiny static binary: capture locally → buffer to disk if offline → sync to
  Waypoint when reachable. Trivial to copy onto a remote host. **Cross-OS scope (v1):** the
  operator wrapper runs on Windows/Linux/macOS; the offline-buffering agent ships Linux/macOS in v1
  with Windows as a fast-follow.
- **AI-assisted plugin generation (v2)** — the repo ships instructions written *for an AI* plus the
  `needs-plugin` queue as raw material: point the AI at a tool's output, it scaffolds a plugin +
  fixtures.

### AI-actor capture (audit-everything covers AI too)

If an AI agent runs actions during an assessment, those actions are captured and attributed with the
same rigor as a human operator's:

- **Same collection path.** AI agents execute commands through the **same terminal wrapper / remote
  agent** (or post through the same ingestion API) that humans use — so capture is automatic and
  uniform. An AI agent is issued its own `ai_agent` actor + token and cannot act un-attributed.
- **The *why* is captured too.** AI-initiated actions record `initiated_by = ai` plus optional
  `decision_context` (rationale / prompt reference), so the report can show not just what the AI did
  but why it chose to.
- **Waypoint's own AI self-audits.** When the v2 offensive LLM / scan library triggers a command, it
  is logged as an AI-actor action the same way — no first-party exemption.
- **First-class ingestion endpoint.** Waypoint ships an official ingestion path (API + MCP endpoint)
  so routing an AI agent through Waypoint is the easy default, not a chore.
- **Best-effort out-of-band detection.** Waypoint additionally attempts to detect and flag actions
  that appear to have bypassed the collection path (e.g. results/entities that surface with no
  corresponding captured action), surfacing them for operator review rather than silently trusting
  full coverage.
- **Documented boundary.** An AI that acts entirely outside the collection path still can't be
  guaranteed-captured; that limit is documented explicitly so the audit story is honest about its
  edges.

---

## Reporting, auditing & alerts (core)

- **Attacker IP always known** — captured per action at collection time. Egress-IP resolution is a
  startup config (`auto` / `manual` / `off`); exec-host IP and pivot chain are always recorded.
- **Full traceability** — every action → source (operator + host + IP), time, target, result.
- **Defensible output** — PDF report + raw bundle generated from the audit trail, with a
  **SHA-256 hash manifest** of every artifact for tamper-evidence at export. Cryptographic signing
  is deferred until a client requires it; the export flow leaves a hook so it can be added without
  rework.
- **Alerts (v1-lite)** — surface notable results as they arrive (e.g. a successful auth, a new
  reachable segment). Full AI-driven alerting is v2.

---

## Deferred to v2+ (explicitly not v1)

- **Node viewer & graph** — cluster by node; surface pivot points (or AI *ideas*); pull in
  BloodHound data / build native AD attack paths; **firewall/network-zone map** ("what can reach
  what" — the novel piece). Lives in the same Postgres store.
- **Scan library** — BloodHound-style guided "exact command to run" for each step; copy → run via
  the terminal plugin → results flow back.
- **Offensive LLM** — operator-minded model that reads current state and says where to go next;
  on-the-fly insights; ties into graph + scan library; AI pre-fill of findings.

---

## Design & aesthetics — the "expedition" design language

Waypoint's UI is themed as a **guided journey through the woods**: a calm, cosy expedition the
operator follows through the target environment, with the tool acting as a friendly guide. The
full reference is `docs/design-spec.md` + `docs/waypoint-mockup.html`. The visual atmosphere lives
in the chrome, map, and transitions — **workspaces where real work happens stay clean and legible
and never compete with the woodland styling.**

### Palette (fixed "artifact" surfaces + themed chrome)

Woody, warm tokens (subset of the full spec):

```
--deep-bark:#3B2617  --bark:#4A2F1B  --walnut:#5C4033  --saddle:#6B4423 (primary btn)
--trail:#8B5E34 (dotted trail)  --harvest:#BA7517 (accent)  --lantern:#EF9F27 (current)
--wheat:#FAC775 (ring/accent-on-dark)  --parchment:#FAEEDA (light panels)
--map-cream:#E8DCC3 (terrain)  --contour:#D4C4A0  --fern:#97C459 / --forest:#639922 (done)
--stone:#B4A78C (locked)  --dark-cocoa:#633806 (headings)  --cocoa:#854F0B (body)
```

Rules: text on parchment is always cocoa/dark-cocoa, never black/gray. **Dark mode:** the map and
parchment panels stay fixed-color "artifact" surfaces (they read as a physical map in both modes);
surrounding chrome uses theme variables. Approved contrast pairs: cocoa-on-parchment,
wheat-on-bark.

### The two different "maps" (do not conflate)

1. **Expedition shell (v1)** — the illustrated trail/home view + guide + journey log. This is the
   navigation and atmosphere layer, present from v1.
2. **Analytical graph (v2)** — the BloodHound-style node / attack-path / network-zone graph. A
   separate, data-dense feature; different visual treatment, deferred.

### Core concept applied to Waypoint

- **Home = an illustrated trail map** of the engagement, not a menu or stepper. Clicking a place on
  the trail opens that area's clean workspace.
- **The tool speaks as a guide** — proactive briefings, never robotic system language. The guide has
  three layers (see "Guide content model" below): an always-on static note, contextual how-to
  notes, and (v2) optional live AI insights.
- Motion: trail "draws" between points on progress (~800ms), current point has a subtle campfire
  flicker; all animation respects `prefers-reduced-motion` and stays <1s (performance is paramount).

### Guide content model (v1)

The guide's help is layered, so it's useful from day one and gets smarter in v2:

1. **Always-on static note (v1).** Every phase/waypoint has a hand-written, *bulletproofed* briefing
   in the cosy guide voice — what this phase is for, what's already in your pack, typical next moves.
   Always present, never depends on the LLM.
2. **Contextual how-to notes (v1).** A library of static, technique-level explainers surfaced in
   context — BloodHound-style ("how to use DNS recon", "what SMB signing off means and how to abuse
   it"): what it is, when to use it, and (later, tied to the v2 scan library) the exact command. This
   is a first-class v1 knowledge layer, keyed to entities/techniques so it appears where relevant.
3. **Live AI insights (v2, optional).** When the offensive LLM is enabled, its on-the-fly guidance
   layers *on top of* the static note — it augments, never replaces, the bulletproofed baseline.

### KEY ADAPTATION — linear wizard → non-linear assessment (bulletproofing note)

The reference mockup is a **linear, locked-stage wizard** (each waypoint gated until the previous is
done). Waypoint's Recon / Attacks / Findings phases are worked **continuously and in parallel**, so
the gating mechanic is re-mapped rather than copied:

- Waypoints represent the **engagement's phases and key milestones** (and, in the v2 scan library, a
  *recommended* path of scans) — they are **always accessible**, not permission-gated.
- The **"fog / locked"** state is re-read as **"nothing discovered here yet"** (e.g. Findings is
  faint until the first result is promoted), not "you haven't earned this yet."
- **Summit = engagement complete → the trip-summary report** (the export-then-wipe bundle, with its
  hash manifest).

### Vocabulary map (generic → Waypoint's security-assessment domain)

Adapt the words/symbols so they make sense for operators (per owner instruction):

| Expedition term | Waypoint meaning |
|---|---|
| Journey log | **The audit trail / activity feed** — one entry per captured action ("Captured `nmap` vs 10.0.0.5 — 12 hosts packed"). Auto-written, timestamped, attributed. This *is* the auditability layer, surfaced cosily. |
| Guide's note | Next-step briefing; in v2, powered by the offensive LLM (pivot + scan + exact command). |
| Your pack / provisions | Collected recon data, credentials, and decisions carried across phases. |
| Break camp / Make camp | Start work / pause-and-persist a workspace (resume later). |
| Fog on the trail | Area with no data yet (not a permission lock). |
| Errors as guide advice | "Can't promote this yet — a finding needs a severity in your pack," never "Error: field required." |
| Reach the summit | Finalize engagement → generate PDF + raw bundle (with hash manifest). |

### Accessibility (carried from the spec)

- Non-visual equivalent for the map: an ordered/labeled waypoint list with state; waypoints are real
  buttons/links with focus rings (harvest) and `aria-current` on the active one.
- Min text 12px on map labels / 13px elsewhere; approved contrast pairs only.
- Mobile: trail runs top-to-bottom (vertical scroll), guide's note docks below, workspace goes
  full-screen with a compact progress pill.

### Design work to start now (in parallel with the build)

- Commit the `waypoint-mockup.html` reference and codify the palette/type as design tokens.
- Continue collecting concrete visual references (Firewatch-style parallax was called out) as a
  living collection.
- Performance and aesthetics are both first-class; neither is sacrificed for the other.

---

## Resolved decisions (bulletproofing rounds)

All previously-open items are now decided:

1. **Public egress IP detection.** Startup config `auto` / `manual` / `off`. `auto` resolves the
   egress IP (STUN or a Waypoint-controlled listener) for usability; `manual` uses operator-declared
   IP(s); `off` records none — respecting no-egress engagements. Exec-host IP + pivot chain are
   always captured regardless.
2. **Real-time team updates.** Server-sent events (SSE); REST for writes.
3. **Entity dedup.** Prefer stable keys (MAC / AD SID / FQDN) when available, fall back to
   (hostname, ip); manual merge/split for ambiguous cases.
4. **Bundle integrity / signing.** SHA-256 hash manifest of every artifact at export for
   tamper-evidence; cryptographic signing deferred until a client requires it (hook left in place).
5. **Windows support (v1).** Operator wrapper is cross-platform (Windows/Linux/macOS); the
   offline-buffering remote agent is Linux/macOS in v1, Windows as a fast-follow.
6. **Trail flow.** Non-linear: all waypoints always accessible; "fog" means "no data discovered here
   yet," not a permission gate.
7. **Guide content.** Three layers — always-on static bulletproofed note + contextual technique
   how-to notes (v1) + optional live AI insights layered on top (v2). See "Guide content model."
8. **AI outside the collection path.** Ship a first-class ingestion endpoint (API + MCP) as the easy
   default, add best-effort detection/flagging of apparently out-of-band actions, and document the
   residual boundary honestly.

---

## Verification (how we prove v1 works)

- **Setup:** `docker compose up` on a clean host stands up the full stack in one step; a fresh
  engagement is reachable and empty. Install script does the same on a supported host.
- **Capture round-trip:** run a known tool (e.g. `nmap`) through the wrapper on the Waypoint host
  and on a remote host via the agent; confirm the action appears with correct operator, exec-host
  IP, egress IP, timestamp, target, and structured parse.
- **Raw fallback:** run an unknown tool; confirm it is captured raw and flagged `needs-plugin`.
- **Attribution:** two operators with distinct tokens run commands; confirm each action is
  attributed to the correct operator, and no shared/anonymous actions exist.
- **AI-actor attribution:** register an AI agent and have it run a command through the wrapper/agent;
  confirm the action is captured with `actor.kind = ai_agent`, `initiated_by = ai`, the authorizing
  human, and a `decision_context` — identical fidelity to a human action, none of it anonymous.
- **Offline buffering:** run a command on a remote host with Waypoint unreachable; restore
  connectivity; confirm buffered captures sync with original timestamps intact.
- **Findings → report:** promote an attack to a finding, fill fields, export; confirm the PDF +
  raw bundle contain the finding with linked evidence and a valid **hash manifest** (every artifact
  hash verifies), and that the trail is complete and consistent.
- **Teardown:** wipe the instance; confirm the exported bundle is fully self-contained (report
  reconstructs from it with nothing left on the box).
- **Tests:** every plugin ships fixtures that run as tests; core capture/attribution paths have
  automated coverage (bulletproof-by-default principle).
- **Dogfooded UX pass (required per feature):** the AI agent launches the running app, drives the
  real UI as an operator would (via browser automation / the app itself), captures screenshots, and
  compares the result against the design spec / `docs/waypoint-mockup.html` — checking that the flow
  makes sense and the expedition styling reads correctly in **both light and dark mode** — before a
  feature is considered done. Passing tests alone does not close a feature.
