# Clean-host journey G5 gate — execution attempt

## Verdict

**Blocked in this sandbox.** I could not execute the full clean-host journey because the environment lacks a running Docker daemon, a PostgreSQL service, and any browser/automation runtime. I still ran the available checks and retained the evidence below.

## Checks run

- `cd /workspace && go test ./...`
- `cd /workspace/web && npm test`
- `cd /workspace/web && npm run build`
- `cd /workspace && docker compose config`
- `cd /workspace && make smoke`
- `docker info`
- `command -v google-chrome || command -v chromium || command -v chromium-browser || command -v firefox || command -v microsoft-edge || command -v msedge || command -v safari || true`

## Results

### Passed

- `go test ./...` passed for:
  - `waypoint`
  - `waypoint/scripts`
- `web` tests passed.
- `web` build passed.
- `docker compose config` rendered the stack successfully.

### Blocked / failed

- Real-PostgreSQL gate tests failed because `WAYPOINT_TEST_PG_DSN` was not set.
- `make smoke` failed because the app refuses to start without `WAYPOINT_DB_DSN`.
- `docker info` reported: `Cannot connect to the Docker daemon at unix:///var/run/docker.sock`.
- No browser binary or browser automation runtime was available.

## Reproducible failure excerpts

```text
WAYPOINT_TEST_PG_DSN is required for real-PostgreSQL gate tests
WAYPOINT_DB_DSN is required
ERROR: Cannot connect to the Docker daemon at unix:///var/run/docker.sock. Is the docker daemon running?
```

## EV retention snapshot

| Evidence | Status | Notes |
|---|---|---|
| EV-01 | Unverified | No sibling plugins-repo runtime evidence in this sandbox. |
| EV-02 | Unverified | Real-DB migrations/auth gates require a live PostgreSQL DSN. |
| EV-03 | Unverified | No live capture transcript available here. |
| EV-04 | Unverified | Offline replay/platform matrix not executable here. |
| EV-05 | Fail | Egress resolution/packet evidence absent in this sandbox. |
| EV-06 | Unverified | Entity merge/split journey not executable here. |
| EV-07 | Unverified | No two-browser/SSE recording runtime available. |
| EV-08 | Fail | No authoritative generated report/PDF artifact retained from this sandbox. |
| EV-09 | Fail | No export/restore runtime artifact retained from this sandbox. |
| EV-10 | Fail | No guarded teardown/post-wipe reconstruction run retained. |
| EV-11 | Unverified | Compose/installer runtime unavailable without Docker daemon. |
| EV-12 | Fail | No measured performance/fault report retained. |
| EV-13 | Fail | No runtime security test/scan report retained. |
| EV-14 | Unverified | No browser screenshots available. |
| EV-15 | Unverified | No accessibility-tree/keyboard/axe/reduced-motion report available. |

## Notes

This worktree retains the existing source-level checks and evidence documents, but the full clean-host acceptance journey still needs a browser-enabled host with a live PostgreSQL/Docker runtime.
