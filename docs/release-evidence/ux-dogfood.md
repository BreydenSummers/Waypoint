# UX/accessibility dogfood evidence

## Timestamp
- 2026-08-25T20:03:21Z

## Checks run
- `go test ./...` → passed
- `cd web && npm test` → passed
- `cd web && npm run build` → passed
- `python3 scripts/verify-contracts.py` → passed
- `command -v google-chrome || command -v chromium || command -v chromium-browser || command -v firefox || command -v microsoft-edge || command -v msedge || command -v safari || true` → no local browser binary
- `node -e "try{console.log(require.resolve('playwright'));}catch(e){console.error('no playwright')}"` → no Playwright
- `node -e "try{console.log(require.resolve('puppeteer'));}catch(e){console.error('no puppeteer')}"` → no Puppeteer
- `node -e "try{console.log(require.resolve('selenium-webdriver'));}catch(e){console.error('no selenium')}"` → no Selenium
- `bash scripts/ux-dogfood-host.sh /tmp/waypoint-ux-dogfood-$$` → blocked (`Docker daemon unavailable`)
- `docker compose up -d --build` → blocked (`Cannot connect to the Docker daemon`)

## Source-level accessibility notes
- Waypoint shortcuts use real links/buttons with `aria-current="step"` on the active item.
- Map hit targets have explicit `aria-label`s.
- Focus rings are defined for `button:focus-visible`, `a:focus-visible`, and `[tabindex]:focus-visible`.
- Reduced motion is respected via `@media (prefers-reduced-motion: reduce)`.
- Mobile breakpoints collapse the layout to a single column.
- Theme switching is persisted and the app sets `data-theme` on `<html>`.

## Contrast audit from CSS tokens
- cocoa on parchment: 8.72:1
- wheat on bark: 7.87:1
- wheat on deep bark: 9.14:1
- deep bark on parchment: 12.41:1
- light text on dark chrome: 14.31:1

## Operator-flow checklist
- [ ] Desktop light: trail map, guide note, journey log
- [ ] Desktop dark: trail map, guide note, journey log
- [ ] Mobile light: stacked trail/workspace layout
- [ ] Mobile dark: stacked trail/workspace layout
- [ ] Accessibility tree
- [ ] Keyboard-only navigation
- [ ] Reduced-motion playback
- [ ] Timestamped screenshots retained

## Summit integration notes
- Runtime summit flow now binds to persisted export jobs, server receipts, verified downloads, cancel/failure states, and guarded teardown authorization.
- The source-level a11y notes below still hold, but I could not capture live screenshots or keyboard/axe output in this sandbox.

## Reproducible host runner
- `scripts/ux-dogfood-host.sh` starts the authoritative compose stack, waits for `/readyz`, and then runs the browser dogfood pass.
- `scripts/ux-dogfood-browser.mjs` captures timestamped desktop/mobile light/dark screenshots, keyboard flow, accessibility tree, contrast checks, reduced-motion evidence, SSE reconnect evidence, and optional axe output when `axe-core` is present.

## Blocker
This sandbox does not provide a runnable browser or a live PostgreSQL service, so I could not capture authoritative screenshots, accessibility tree output, keyboard transcript, axe report, or reduced-motion playback from the running app. The visual dogfood pass therefore remains unverified here and must be completed in a browser-enabled environment.

## Current attempt
- `go test ./...` → passed
- `cd web && npm test` → passed
- `cd web && npm run build` → passed
- `bash scripts/ux-dogfood-host.sh /tmp/waypoint-ux-dogfood-check` → blocked (`Docker daemon unavailable`)
- `node scripts/ux-dogfood-browser.mjs --base-url http://127.0.0.1:8080 --out-dir /tmp/waypoint-ux-browser-check` → blocked (`Playwright is unavailable on this host`)
- `curl -fsS http://127.0.0.1:8080/readyz` → no service listening

## Notes
- I could not validate the required light/dark/mobile screenshots, accessibility tree, keyboard transcript, axe report, contrast run, or reduced-motion evidence because the app could not be launched in this sandbox.
- No product defects were directly observed in a live UI session; the only confirmed blockers were environment-level.
