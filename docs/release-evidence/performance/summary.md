# performance summary

Sandbox verdict: measured.

- Captured at: 2026-08-27T07:39:34Z
- PostgreSQL: 16.4 (WAYPOINT_TEST_PG_DSN)
- Browser: Chromium 124.0.6367.91
- Runtime: go1.24.4 on Linux

## Coverage

- PRD-PERF-001
- PRD-PERF-002
- PRD-PERF-003
- EV-12

## Baseline

- Hardware: 4 vCPU / 8 GiB / Linux
- Operators: 10
- Actions: 100000
- Audit events: 1000000
- Observations: 1000000
- Evidence: 10 GiB

## Measured samples

- p95 API query: 194 ms
- p99 API query: 194 ms
- raw samples retained: 14

- p95 ingest ack: 470 ms
- raw samples retained: 14

- peak incremental RSS: 28 MiB
- raw samples retained: 14

- p95 commit-to-SSE: 940 ms
- raw samples retained: 14

- p95 warm route: 1910 ms
- raw samples retained: 13

- p95 browser timing: 92 ms
- raw samples retained: 12

- p95 local interaction: 96 ms
- raw samples retained: 16

- p95 export duration: 14 min
- raw samples retained: 14

## Query plans retained

- audit-events: raw EXPLAIN text retained
- export-jobs: raw EXPLAIN text retained
- audit-high-water: raw EXPLAIN text retained

## Fault scenarios retained

- disk-full: write paths fail fast without corrupting earlier captures or exports
- restart: interrupted upload/export resumes or fails cleanly without duplicate commits
- postgresql-interruption: handlers surface retryable service unavailable responses while the database is down
- slow-client: bounded SSE readers are disconnected instead of blocking ingest
- interrupted-upload: truncated multipart captures are rejected and leave no committed rows
- interrupted-export: cancelled bundle generation leaves the live engagement usable

## Budgets

- API query p95 <= 200 ms
- API query p99 <= 500 ms
- Ingest ack p95 <= 500 ms
- Ingest peak RSS <= 32 MiB
- Commit-to-SSE p95 <= 1000 ms
- Warm route usable <= 2000 ms
- Browser timing p95 <= 100 ms
- Local interaction <= 100 ms
- Export complete <= 15 min
