# Waypoint

Waypoint is a self-contained platform for running security assessments (built for
school / university networks). Its two jobs: **capture everything** from every tool an
operator or AI agent runs, and keep a **clean, attributable audit trail** so client
reports are defensible. A single Postgres holds all state; evidence is stored raw-first
on disk; every actor — human or AI — is a first-class attributed identity with its own
token. The UI is a cosy "expedition / trail" design (Recon → Attacks → Findings → Summit).

For the full product definition see [`PRD.md`](PRD.md); for supported operator flows and
limits see [`docs/operator-guide.md`](docs/operator-guide.md).

---

## Prerequisites

- **Docker** + the Docker Compose plugin (`docker compose`) — for the disposable stack.
- **openssl** and **sha256sum** (standard on Linux/macOS) — to mint an operator token.
- To build or test from source: **Go 1.22+** and **Node 22+** (the container build handles
  this for you; you only need them for local `go test` / web builds).

Everything runs on `localhost`; no external services are required.

---

## Quick start (disposable Compose stack)

This is the fastest way to a running instance you can tear down cleanly afterward.

### 1. Start the stack

```sh
docker compose up -d --wait
```

This builds the image, starts Postgres and Waypoint, applies all database migrations, and
waits until the app is healthy. The API and UI are then served on **http://localhost:8080**.

Check it:

```sh
curl -fsS http://localhost:8080/readyz    # -> {"status":"ready"}
```

### 2. Grab the one-time setup code

A fresh instance has no data and no credentials, so it prints a **setup code** in a bordered
`WAYPOINT — FIRST-TIME SETUP` banner at the end of startup. The code lives only in memory and
stops working the moment setup completes:

```sh
docker compose logs waypoint      # look for the yellow-bordered banner + setup code
```

### 3. Complete first-time setup in the browser

Browse to **http://localhost:8080/** and follow the setup wizard: paste the setup code, name
the engagement (name / client / scope), and pick your owner handle. Waypoint mints your
**owner token**, shows it once (only its SHA-256 digest is stored — save it), and drops you
straight into the trail already signed in. The trail map, notable-alerts feed, and phase
workspaces come to life; captures ingested via REST/MCP show up live over the SSE feed.

Returning operators sign in by pasting their own token into the **Operator token** field. Add
more operators or AI actors from the in-app provisioning workspace or the actors API
(`POST /api/v1/actors`); every AI actor must name a human `authorized_by` actor.

> **No interactive step?** For automated deployments, set the first engagement and owner via
> `WAYPOINT_BOOTSTRAP_ENGAGEMENT_NAME` / `_CLIENT` / `_SCOPE` and `WAYPOINT_BOOTSTRAP_OWNER_HANDLE`
> (optionally `WAYPOINT_BOOTSTRAP_OWNER_TOKEN`). Waypoint then provisions at startup and skips
> the wizard. To provision by SQL instead, set `WAYPOINT_DISABLE_SETUP_WIZARD=1` — see
> [`docs/operator-guide.md`](docs/operator-guide.md).

### 4. Tear it down

```sh
docker compose down -v          # stops containers and removes the data + evidence volumes
```

---

## Supported host install

For a persistent, supported deployment on Ubuntu 22.04 / 24.04 (x86_64), use the installer,
which manages the database, service files, TLS, rollout/rollback, and account provisioning
from a JSON file (it mints one token file per actor under `0600` perms):

```sh
scripts/waypoint-installer.sh validate   --config /path/to/installer.env --provision /path/to/provision.json
scripts/waypoint-installer.sh install    --config /path/to/installer.env --provision /path/to/provision.json
scripts/waypoint-installer.sh provision   --config /path/to/installer.env --provision /path/to/provision.json
scripts/waypoint-installer.sh diagnostics --config /path/to/installer.env --provision /path/to/provision.json
```

The `provision.json` holds one `engagement` object and a non-empty `actors` array (each with
`id` (UUID), `kind` (`human` or `ai_agent`), `handle`, `role`; AI actors also need
`agent_name`, `model`, `version`, and a human `authorized_by`). See
[`docs/operator-guide.md`](docs/operator-guide.md) for the full matrix and TLS/secret handling.

---

## Configuration

The runtime binary reads these environment variables (the Compose file sets sensible
defaults):

| Variable | Purpose | Default (Compose) |
|---|---|---|
| `WAYPOINT_ADDR` | Listen address | `:8080` |
| `WAYPOINT_DB_DSN` | Postgres DSN (required) | `postgres://waypoint:waypoint@postgres:5432/waypoint?sslmode=disable` |
| `WAYPOINT_EVIDENCE_DIR` | Raw evidence storage path | `/var/lib/waypoint/evidence` |
| `WAYPOINT_EGRESS_MODE` | Egress policy (`auto` / `manual` / `off`) | unset |
| `WAYPOINT_TLS_CERT_FILE` / `_KEY_FILE` / `_CA_FILE` | Serve HTTPS directly | unset |
| `WAYPOINT_BOOTSTRAP_ENGAGEMENT_NAME` / `_CLIENT` / `_SCOPE` | Auto-provision the first engagement at startup (automated deploys) | unset |
| `WAYPOINT_BOOTSTRAP_OWNER_HANDLE` / `_OWNER_TOKEN` | First owner's handle (required for auto-provision) and optional token (generated + printed if omitted) | unset |
| `WAYPOINT_DISABLE_SETUP_WIZARD` | Disable the web setup wizard (provision via SQL/installer instead) | unset |

Plain HTTP is loopback-only. For a non-loopback listener, configure TLS on the app itself,
or terminate TLS at a trusted reverse proxy while Waypoint stays bound to loopback. Tokens,
DSNs, and receipts never belong in URLs or logs.

---

## Key endpoints

| Endpoint | Purpose |
|---|---|
| `GET /healthz`, `GET /readyz` | Liveness / readiness (readiness pings the DB) |
| `POST /api/v1/captures` | REST capture ingest (idempotent, multipart raw evidence) |
| `GET /events` | Authorized resumable SSE audit/alert feed |
| `POST /api/v1/mcp` | MCP ingest (Streamable HTTP JSON-RPC) |
| `POST /api/v1/actors` | Provision additional actors (operator-authenticated) |
| `GET /api/v1/audit-events` | Paginated audit history |
| `GET /engagements/{slug}` | Operator SPA |

All API requests send `Authorization: Bearer <token>` and `Waypoint-Contract-Version: 1.0.0`.

---

## Building and testing from source

```sh
# Fast, no external services (skips the Docker/Postgres integration tests):
go test -skip 'Compose' ./...

# Full real-Postgres suite: point it at a throwaway database, then run serially.
docker run -d --name wp-testpg -e POSTGRES_DB=waypoint -e POSTGRES_USER=waypoint \
  -e POSTGRES_PASSWORD=waypoint -p 5599:5432 postgres:16-bookworm
export WAYPOINT_TEST_PG_DSN="postgres://waypoint:waypoint@127.0.0.1:5599/waypoint?sslmode=disable"
go test -count=1 -p 1 -parallel 1 ./internal/server/ ./internal/db/     # tests reset the schema; run serially
docker rm -f wp-testpg

# Compose integration tests (need Docker; each builds the image with --no-cache, ~5-10 min):
go test -run 'Compose' -timeout 45m ./
```

Provision the `WAYPOINT_TEST_PG_DSN` database for a full run — without it, the ~50
real-Postgres gate tests skip.

---

## Documentation map

| Doc | What it covers |
|---|---|
| [`PRD.md`](PRD.md) | v1 product requirements and locked decisions |
| [`docs/operator-guide.md`](docs/operator-guide.md) | Supported operator flows, TLS/secret handling, actor model |
| [`docs/design-spec.md`](docs/design-spec.md) | Architecture and the expedition design language |
| [`AGENTS.md`](AGENTS.md) | Conventions for automated contributors |

---

## Teardown & disposability

Waypoint is disposable by design: a Compose stack is destroyed with `docker compose down -v`,
and the installer supports a receipt-verified `destroy`. Before teardown, export a
signed engagement bundle so the audit trail stays defensible after the instance is gone.
