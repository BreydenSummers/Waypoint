# compose-startup evidence

Retained diagnostics for the deterministic Compose startup pass.

Current timings:
- Postgres: start_period 20s, interval 3s, retries 30
- Waypoint: start_period 45s, interval 5s, retries 36
- Compose readiness polling: 5m
- Compose test timeout: 15m
- Make smoke readiness loop: 120s

Capture the following here when runtime validation is available:
- compose-config.txt
- compose-build.txt
- compose-up.txt
- compose-down.txt
- compose-runtime-test.txt
- docker-info.txt
- restart-cycle diagnostics
