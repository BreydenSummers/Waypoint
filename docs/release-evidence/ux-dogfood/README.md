# UX dogfood / accessibility gate evidence

## Timestamp
- 2026-08-25T20:03:21Z (prior retained audit)
- 2026-08-25T??:??:??Z (this rerun in the assigned worktree)

## Checks run in this worktree
- `go test ./...` → passed
- `make test` → passed
- `make lint` → passed
- `npm --prefix web run build` → passed
- `python3 scripts/verify-contracts.py` → passed
- `docker info` → failed: Docker daemon unavailable
- `command -v google-chrome || command -v chromium || command -v chromium-browser || command -v firefox || command -v microsoft-edge || command -v msedge || command -v safari || true` → no browser binary
- `node -e "try{console.log(require.resolve('playwright'));}catch(e){console.error('no playwright')}"` → no Playwright
- `node -e "try{console.log(require.resolve('puppeteer'));}catch(e){console.error('no puppeteer')}"` → no Puppeteer
- `node -e "try{console.log(require.resolve('selenium-webdriver'));}catch(e){console.error('no selenium')}"` → no Selenium

## Why the real-browser gate is still blocked here
- The app cannot start without a live PostgreSQL DSN.
- This sandbox has no PostgreSQL service/tooling and no Docker daemon.
- There is no browser runtime available to drive the live UI.

## Source-level accessibility notes retained
- Waypoint waypoints are real links/buttons and the active item uses `aria-current="step"`.
- Focus rings are defined for buttons, links, and tabindex targets.
- Reduced motion is honored via `@media (prefers-reduced-motion: reduce)`.
- Mobile breakpoints collapse the layout to one column.
- The trail uses an ordered waypoint list as the non-visual equivalent.

## Contrast audit from CSS tokens
- cocoa on parchment: 8.72:1
- wheat on bark: 7.87:1
- wheat on deep bark: 9.14:1
- deep bark on parchment: 12.41:1
- light text on dark chrome: 14.31:1

## EV-07 / EV-14 / EV-15 retention status
| EV | Status | Notes |
|---|---|---|
| EV-07 | Unverified | No live two-session SSE/browser recording runtime available in this sandbox. |
| EV-14 | Unverified | No live light/dark desktop/mobile screenshots could be captured. |
| EV-15 | Unverified | No live accessibility-tree, keyboard, axe, or reduced-motion browser artifact could be captured. |

## Remediation note
- Fixed the trail-map accessibility duplication: the decorative SVG map is now hidden from the accessibility tree, leaving the ordered waypoint list and live shortcut buttons as the non-visual equivalent.
- Before: the SVG announced as an image in addition to the waypoint navigation, which would have cluttered the screen-reader path.
- After: the accessible trail remains the ordered list / shortcut buttons only, matching the design spec more cleanly.

## Conclusion
The web bundle and source checks are current, but the required real-browser dogfood pass still needs a host with Docker, PostgreSQL, and a browser automation runtime.
