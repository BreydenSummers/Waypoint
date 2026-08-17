Waypoint final fail-closed v1 acceptance artifacts
Run started: 2026-08-17T09:20:54Z
Verdict: FAIL — G5 closed
PRD rows: 7 Pass, 15 Fail, 45 Unverified (67 total)
Evidence rows: EV-01–EV-16 reconciled; EV-16 Pass, all others release-blocking

This directory retains exact commands, UTC timestamps, exit codes, raw test output, host/tool
availability, focused source inventory, and the deferral inventory. check-status.tsv is the command
ledger. go-test-summary.txt is derived from go-test-json.txt. SHA256SUMS hashes every retained file
except SHA256SUMS itself.

Interpretation is fail-closed: a zero exit caused by a skipped gate (notably the Compose runtime
test) is Unverified, not Pass. Source, fixture, contract, and syntax checks prove only their narrow
claims and do not replace required PostgreSQL, collector, native-platform, browser, security,
performance, clean-room restore, or post-wipe evidence.

Canonical reconciliation:
- docs/release-evidence/final-v1-acceptance.md
- docs/v1-traceability.md
