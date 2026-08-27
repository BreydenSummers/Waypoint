Waypoint v1 completion reconciliation artifacts
Run: 2026-08-27T22:13:26Z
Verdict: FAIL (9 Pass, 8 Fail, 50 Unverified PRD rows)

release-test.txt records the trusted release-mode gate failing closed because no browser binary is available.
unit-gate/ retains the executable unit-mode subset: 81 passed, 0 failed, 56 skipped, including 48 release-critical skips.
environment.txt records the unavailable Docker daemon, PostgreSQL tooling/DSN, and browser runtime.
Other files retain focused lint, web, Compose syntax, and reconciliation test output.

See ../v1-completion-reconciliation.md for qualifications and EV-01 through EV-16 disposition.
