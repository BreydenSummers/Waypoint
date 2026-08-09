# AGENTS.md — Waypoint (core app)

> Context file for coding agents (PI, and Claude/`CLAUDE.md`-compatible). Read this first, then the
> canonical docs below before writing code.

## What Waypoint is

A self-contained, disposable platform for running **security assessments** (initial target: school /
university networks). Two core reasons it exists:

1. **Capture everything** from every tool across an engagement.
2. Keep a **clean, attributable audit trail** so the client report is *defensible* and falls out of
   the data rather than being reconstructed by hand.

Graph, offensive LLM, and scan library are **v2+ features, not the headline.**

## Read these first (canonical, in this repo)

- `PRD.md` — the full v1 Product Requirements Document. **Source of truth.**
- `docs/design-spec.md` — the "expedition/trail" visual design language.
- `docs/waypoint-mockup.html` — reference mockup (open it to see the intended look).

Sibling repo: `../Waypoint-Plugins` — terminal-output parser plugins (see its own `AGENTS.md`).

## v1 scope (what to build)

Capture + audit + the three-phase model (**Recon / Attacks / Findings**) + signed-less report export.
Deferred to v2+: node/attack-path graph, offensive LLM, guided scan library, network-zone map.

## Guiding principles (constraints on every feature)

- **Performance is paramount** — every view/query/import must feel instant.
- **Bulletproof by default** — everything covered by tests; no debugging on-site.
- **Disposable** — a fresh engagement stands up in ~one step; teardown wipes cleanly *after* the
  engagement bundle is exported.
- **Trail-based & cosy** — calm, well-designed; not enterprise vuln-management. See the design spec.
- **Auditable end to end** — every action is timestamped + attributed at collection time,
  **whether a human or an AI agent initiated it**.

## Key locked decisions (full rationale in PRD.md)

- **Store:** PostgreSQL, single container (graph lives in the same store in v2).
- **Deployment:** team / shared instance, multiple concurrent operators; **per-actor tokens, no
  shared token**. AI agents are first-class attributed `actor`s (kind `ai_agent`).
- **Real-time:** server-sent events (SSE); REST for writes.
- **Capture:** raw-first — every command captured even with no parser; unknown tools flagged
  `needs-plugin`.
- **Attribution per action:** exec-host IP + public/NAT egress IP (+ pivot chain). Egress detection
  is a startup config `auto | manual | off`.
- **Entity dedup:** prefer stable keys (MAC / AD SID / FQDN), fall back to (hostname, ip); manual
  merge/split for ambiguity.
- **Export:** self-contained engagement bundle (DB + evidence + PDF) + **SHA-256 hash manifest**;
  cryptographic signing deferred (hook left in place).
- **Trail UX is non-linear:** all waypoints always accessible; "fog" = no data discovered yet, not a
  permission gate.
- **Guide content:** always-on static bulletproofed note + contextual technique how-to notes (v1) +
  optional live AI insights layered on top (v2).
- **AI outside the collection path:** ship a first-class ingestion endpoint (API + MCP) + best-effort
  detection of out-of-band actions + document the residual boundary.

## How to work in this repo (working agreements)

- **Bulletproof the thinking.** Before implementing, surface contradictions, gaps, and edge cases —
  don't just transcribe the request. Propose concrete decisions with rationale.
- **Dogfooded-UX gate.** A feature is NOT done when tests pass. You must also **run the app, use the
  feature as an operator would, screenshot it, and compare against `docs/design-spec.md` /
  `docs/waypoint-mockup.html` in BOTH light and dark mode.** Passing tests alone does not close a
  feature. (See the `dogfood-ux` skill.)
- **Adapt the design vocabulary to the security domain** — e.g. the "Journey log" IS the audit
  trail; "Summit" = finalize + export. Words and symbols must make sense to an operator.
- Match existing code style; keep the woodland styling in the chrome, never in the work surfaces.

## Skills available in this repo

- `dogfood-ux` (`.pi/skills/dogfood-ux/`) — the required run-it-and-eyeball-it verification pass.
- Plugin authoring lives in the sibling repo: `../Waypoint-Plugins/.pi/skills/waypoint-plugin-authoring/`.
