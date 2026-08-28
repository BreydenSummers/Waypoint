# performance benchmark evidence

Status: measured and retained; the report is rendered from raw samples and fails closed when PostgreSQL, browser, or runtime provenance is unavailable.

## Required profile

- Measured profile: `samples/raw-profile.json`
- Hardware: 4 vCPU / 8 GiB / Linux
- Provenance: PostgreSQL + browser + runtime captured in the raw profile
- Operators: 10
- Actions: 100000
- Audit events: 1000000
- Observations: 1000000
- Evidence: 10 GiB

## Budget targets

- API query p95 <= 200 ms
- API query p99 <= 500 ms
- Ingest ack p95 <= 500 ms
- Ingest peak RSS <= 32 MiB incremental
- Commit-to-SSE p95 <= 1000 ms
- Warm route usable <= 2000 ms
- Local interaction <= 100 ms
- Export complete <= 15 min

## Optimizations retained

- `audit_event_engagement_id_idx` for keyset audit reads
- `export_job_engagement_updated_at_idx` for export job list paging
- raw samples under `samples/` tracing PRD-PERF-001 through PRD-PERF-003 and EV-12

## Reproducible run

1. Set `WAYPOINT_TEST_PG_DSN` to a real PostgreSQL instance.
2. Apply migrations.
3. Run `python3 scripts/performance-report.py` to re-render the retained summary from the raw samples.
4. Keep the summary and raw samples in sync by re-running the script after each measured harness capture.

Current retained samples:
- `samples/source-gate.txt`
- `samples/performance-gate.txt`
- `samples/real-db-blocked.txt`
- `samples/go-test-all.txt`
- `samples/raw-profile.json`

