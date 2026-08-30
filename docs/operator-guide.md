# Waypoint operator guide

Supported v1 operator flows only. This guide covers the shipped paths and the documented limits; see the [documentation matrix](release-evidence/documentation/README.md) for coverage.

## Supported setup

### 1) Disposable Compose stack

Use this when you want a fresh engagement that is easy to tear down later.

**a. Start the stack** (builds the image, starts Postgres + Waypoint, applies migrations,
and waits until healthy on http://localhost:8080):

```sh
docker compose up -d --wait
curl -fsS http://localhost:8080/readyz    # -> {"status":"ready"}
```

**b. Provision an engagement and mint an operator token.** A fresh instance has no data and
no credentials; only a token's SHA-256 digest is ever stored. This block creates the
engagement + operator and prints the token once:

```sh
ENGAGEMENT_ID=11111111-1111-4111-8111-111111111111
ACTOR_ID=22222222-2222-4222-8222-222222222222
TOKEN=$(openssl rand -hex 24)
TOKEN_HASH=$(printf '%s' "$TOKEN" | sha256sum | cut -d' ' -f1)
docker compose exec -T postgres psql -U waypoint -d waypoint -v ON_ERROR_STOP=1 -c "
  INSERT INTO engagement (id, name, client, scope, status)
  VALUES ('$ENGAGEMENT_ID', 'Demo Expedition', 'Acme University', 'campus /16', 'active');
  INSERT INTO actor (id, engagement_id, kind, handle, token_hash, role)
  VALUES ('$ACTOR_ID', '$ENGAGEMENT_ID', 'human', 'alex.operator', '$TOKEN_HASH', 'operator');"
echo "Operator token: $TOKEN"
```

Add further operators or AI actors after this first one with the authenticated actors API
(`POST /api/v1/actors`), or use the installer's file-based `provision` (section 2) for
repeatable, host-managed provisioning.

**c. Open the UI** at http://localhost:8080/engagements/demo and paste the token into the
**Operator token** field. Tear the stack down with `docker compose down -v`.

This stack is the path used for dogfood UX runs; pair it with Playwright-driven browser checks when verifying the operator flow.

Relevant files: [`compose.yml`](../compose.yml), [`cmd/waypoint/main.go`](../cmd/waypoint/main.go), [`../README.md`](../README.md).

### 2) Supported-host install

Use the installer on supported Ubuntu hosts (22.04/24.04 x86_64).

```sh
scripts/waypoint-installer.sh validate --config /path/to/installer.env --provision /path/to/provision.json
scripts/waypoint-installer.sh install   --config /path/to/installer.env --provision /path/to/provision.json
scripts/waypoint-installer.sh provision --config /path/to/installer.env --provision /path/to/provision.json
scripts/waypoint-installer.sh diagnostics --config /path/to/installer.env --provision /path/to/provision.json
```

The installer is the supported path for account provisioning, service files, rollout, and rollback.

## TLS and secret handling

- The runtime binary reads `WAYPOINT_DB_DSN`, `WAYPOINT_ADDR`, and `WAYPOINT_EGRESS_MODE` from the environment.
- The installer records a loopback service bind (`WAYPOINT_ADDR=127.0.0.1:8080`) plus TLS paths (`WAYPOINT_TLS_CERT_FILE`, `WAYPOINT_TLS_KEY_FILE`, `WAYPOINT_TLS_CA_FILE`) and egress settings in `0600` config files.
- Plain HTTP is loopback-only. If you need a non-loopback listener, configure TLS cert/key on the app itself; a trusted reverse proxy may terminate TLS only while Waypoint stays bound to loopback.
- Tokens, DSNs, receipts, and bundle paths never belong in URLs or logs.
- Sensitive state is written with restrictive permissions where the installer manages the files; host/disk encryption remains the operator’s responsibility.

## Human and AI actors

- Human actors get one-time tokens; only the digest is stored.
- AI actors are first-class `ai_agent` actors with their own token.
- Every AI actor must name a human `authorized_by` actor, plus model and version metadata.
- No shared token exists.
- Human and AI actions use the same capture path and audit fidelity.

## REST and MCP ingestion

- REST capture: `POST /api/v1/captures`
- SSE feed: `GET /events`
- MCP: `POST /api/v1/mcp` (Streamable HTTP JSON-RPC)
- MCP tools: `waypoint_ingest_capture`, `waypoint_capture_status`
- Out-of-band review: `POST /api/v1/out-of-band-claims/review`

REST and MCP share the same auth, idempotency, validation, and audit service.

## Wrapper and agent matrix

| Component | v1 status |
|---|---|
| Operator wrapper | Supported on Windows, Linux, and macOS |
| Offline remote agent | Supported on Linux and macOS |
| Windows offline agent | Deferred to v2 / fast-follow |
| AI actor capture | Same wrapper/API/MCP path as humans |

## Egress modes

Set `WAYPOINT_EGRESS_MODE` to one of:

- `auto` — resolve the public/NAT egress IP through `WAYPOINT_EGRESS_ENDPOINT`
- `manual` — use the operator-declared `WAYPOINT_EGRESS_ADDRESS`
- `off` — send no discovery traffic and record no public egress value

```sh
WAYPOINT_EGRESS_MODE=auto WAYPOINT_EGRESS_ENDPOINT=https://egress.example/resolve
WAYPOINT_EGRESS_MODE=manual WAYPOINT_EGRESS_ADDRESS=198.51.100.10
WAYPOINT_EGRESS_MODE=off
```

`off` intentionally loses public-source attribution; only exec-host IP, target, timing, and the rest of the action record remain.

## Findings, report, export, verify, restore, and destroy

1. Promote an attack to a finding in the app; findings are human-confirmed only.
2. Read the frozen report snapshot:
   - `GET /api/v1/engagements/:engagementID/summit/report.json`
   - `GET /api/v1/engagements/:engagementID/summit/report.pdf`
3. Create and inspect export jobs:
   - `POST /api/v1/exports`
   - `GET /api/v1/exports/:exportJobID`
   - `GET /api/v1/exports/:exportJobID/bundle`
4. Verify and restore the bundle:
   - `node bundle/tools/verify-restore.mjs /path/to/bundle-root`
   - `node bundle/tools/regenerate-report.mjs /path/to/bundle-root /tmp/restored-report.html`
5. Tear down only after export verification:
   - `scripts/waypoint-installer.sh destroy --bundle /path/to/bundle-root --receipt /path/to/receipt.json`

The bundle is sensitive client data. The manifest is SHA-256 hash-only; it is tamper-evidence, not a signature.

## Troubleshooting

- `WAYPOINT_DB_DSN is required` — set the database DSN before starting `cmd/waypoint`.
- `WAYPOINT_EGRESS_ENDPOINT is required for auto mode` — set the resolver URL when `WAYPOINT_EGRESS_MODE=auto`.
- `WAYPOINT_EGRESS_ADDRESS is required for manual mode` — set the declared address when `WAYPOINT_EGRESS_MODE=manual`.
- `psql not available for provisioning` — install PostgreSQL client tools before using `provision`.
- `receipt does not match the requested teardown bundle` — the receipt and bundle hash inputs do not match; re-verify before destroy.
- `service_unavailable` on capture/report/export — usually means the DB, auth, or startup prerequisites are not ready.

## Break-glass guidance

`--force` exists for teardown emergencies only:

```sh
scripts/waypoint-installer.sh destroy --bundle /path/to/bundle-root --force
```

Use it only when you accept that the verified receipt path is unavailable. It does not make host-admin deletion safe, and it does not recover missing evidence.

## Limits and exclusions

- Wholly out-of-band human or AI execution cannot be guaranteed captured.
- `egress=off` intentionally loses public-source attribution.
- Host admins can still delete volumes or read unencrypted data.
- Disk encryption and secure transport for stored bundles are operator responsibilities.
- Windows offline agent support is deferred.
- v2 exclusions remain excluded: graph, zone map, guided scan library, offensive LLM, AI finding prefill, parser/plugin generation, and cryptographic signing.
