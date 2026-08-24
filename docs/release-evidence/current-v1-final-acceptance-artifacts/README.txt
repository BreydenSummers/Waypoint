Waypoint current-main fail-closed final v1 acceptance artifacts
Run started: 2026-08-24T08:42:57Z
Run completed: 2026-08-24T08:51:10Z
Verdict: FAIL — G5 closed; technical readiness not recommended; human acceptance false
PRD rows: 7 Pass, 15 Fail, 45 Unverified (67 total)
Evidence rows: EV-01–EV-16 reconciled; only EV-16 Pass
Go gate: 43 top-level Pass, 0 Fail, 48 Skip; release-critical zero-skip condition failed

These files retain current core contract/scope reruns, command status, environment limitations,
full Go JSON output, build/test/deployment attempts, and focused current source inventories. A zero
exit caused by skip is Unverified, not Pass. Core/source/synthetic checks cannot replace plugins,
PostgreSQL, browser, native-platform, security, measured-performance, clean-room, or post-wipe
runtime evidence.

Canonical decision: docs/release-evidence/current-v1-final-acceptance.md
Canonical matrix: docs/v1-traceability.md
