# performance-fault-gate

Run: `2026-08-27T07:39:34Z`
Status: **Measured / Retained**

## Retained performance profile

Source: [`docs/release-evidence/performance/samples/raw-profile.json`](./performance/samples/raw-profile.json)

### Baseline

- Hardware: 4 vCPU / 8 GiB / Linux
- Operators: 10
- Actions: 100000
- Audit events: 1000000
- Observations: 1000000
- Evidence: 10 GiB

### Budgets / percentiles

- API query p95: 200 ms
- API query p99: 500 ms
- Ingest ack p95: 500 ms
- Ingest peak RSS: 32 MiB
- SSE visible p95: 1000 ms
- Warm route usable: 2000 ms
- Local interaction: 100 ms
- Export complete: 15 min

### Fault plan

- disk-full
- restart
- postgresql-interruption
- slow-client
- interrupted-upload
- interrupted-export

## Commands run

### Fixture / contract gate

```sh
go test -v ./internal/server -run TestPerformanceProfileFixtureSeedsBaselineAndFaultScenarios
```

Result: **PASS**

### Real-PostgreSQL fault gate

```sh
go test -v ./internal/server -run 'TestUpsertEvidenceRejectsImmutableMetadataChanges|TestCaptureIngestRejectsDiskPressureBeforeCommitting|TestCaptureIngestRetriesAfterInterruptedUploadWithoutDuplication'
```

Result: **FAIL CLOSED**

Representative failure:

```text
WAYPOINT_TEST_PG_DSN is required for real-PostgreSQL gate tests
```

## Notes

- The retained fixture still documents the required plans, percentiles, profile, and fault coverage.
- EV-12 remains **Fail** until a real measured performance/fault report is produced.
- No code changes were made.
