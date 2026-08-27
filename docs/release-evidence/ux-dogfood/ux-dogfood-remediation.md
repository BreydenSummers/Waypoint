# UX dogfood remediation note

## Change made
- Hid the decorative trail-map SVG from the accessibility tree (`aria-hidden="true" focusable="false"`).
- Kept the ordered waypoint list and the live shortcut buttons as the non-visual equivalent.

## Before / after
- Before: screen readers would encounter the SVG as an additional image landmark on top of the waypoint navigation.
- After: the trail map is visual chrome only; the accessible navigation path is the ordered list / buttons.

## Checks run
- `go test ./...` → passed
- `npm -C web test` → passed
- `npm -C web run build` → passed
- `make build` → passed
- `bash scripts/ux-dogfood-host.sh /tmp/waypoint-ux-dogfood-check` → blocked: Docker daemon unavailable
- `node scripts/ux-dogfood-browser.mjs --base-url http://127.0.0.1:8080 --out-dir /tmp/waypoint-ux-browser-check` → blocked: Playwright unavailable

## Light/dark/accessibility evidence
- Source-level contrast audit remains unchanged:
  - cocoa on parchment: 8.72:1
  - wheat on bark: 7.87:1
  - wheat on deep bark: 9.14:1
- Keyboard/focus/reduced-motion hooks remain present in source and the new trail-map fix reduces a11y noise in the map area.

## Note
A live browser capture could not be completed in this sandbox, so screenshots and accessibility-tree output remain pending on a host with Docker, PostgreSQL, and Playwright.
