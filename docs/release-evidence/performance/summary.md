# performance summary

Sandbox verdict: blocked.

- The performance profile fixture is retained unchanged.
- Read-path indexes were added for audit pagination and export-job list scans.
- A real PostgreSQL-backed benchmark was not runnable in this environment, so no live p95/p99, RSS, SSE, warm-route, local-interaction, or export-duration samples were captured.
- The retained artifact set is ready for a repeatable rerun under `WAYPOINT_TEST_PG_DSN`.
