# Findings-to-report workflow evidence

Retained checkpoints for EV-08 / EV-09 / EV-10.

- EV-08: finding promotion, evidence auto-linking, revision history, and frozen PDF/report rendering.
- EV-09: export bundle manifest, outer hash, restore/regenerate tooling, and clean-room verification.
- EV-10: guarded teardown authorization and post-wipe bundle verification.

Notes:
- The authoritative real-PostgreSQL + real-Chromium journey is gated by `WAYPOINT_TEST_PG_DSN` and `WAYPOINT_CHROMIUM`.
- Local verification here covers compilation plus bundle/tooling contract tests.
