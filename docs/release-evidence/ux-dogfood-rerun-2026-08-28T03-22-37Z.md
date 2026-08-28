# UX dogfood rerun evidence

## Timestamp
- 2026-08-28T03:22:37Z

## Result
**Blocked from completing the real-browser UX gate in this sandbox.**

## Checks run here
- `go test ./...` → passed
- `npm --prefix web run test` → passed
- `npm --prefix web run build` → passed
- `make build` → passed
- `bash scripts/ux-dogfood-host.sh /tmp/waypoint-ux-dogfood-check` → failed: Docker daemon unavailable
- `node scripts/ux-dogfood-browser.mjs --base-url http://127.0.0.1:8080 --out-dir /tmp/waypoint-ux-browser-check` → failed: Playwright unavailable

## Remediation applied
- Fixed the browser runner's mobile context URL handling so the dogfood script now uses the provided `baseUrl` for both desktop and mobile captures.

## Environment blockers
- Docker daemon unavailable, so the production compose stack could not be started.
- Browser automation runtime unavailable, so screenshots, accessibility tree, axe, keyboard-flow, and reduced-motion captures could not be produced.

## Notes
- The source already carries the required accessibility mechanics for PRD-QUAL-002 / EV-14 / EV-15:
  - waypoints expose `aria-current="step"`
  - reduced-motion is honored in CSS
  - mobile layout is responsive
  - the trail has a non-visual ordered-list / shortcut equivalent
- The live browser gate still needs a host with Docker, PostgreSQL, and Playwright/Chromium.

## Conclusion
This rerun is a documented blocker, not an approval.
