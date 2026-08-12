# UX/accessibility dogfood evidence

## Timestamp
- 2026-08-12T05:41:22Z

## Checks run
- `go test ./...` → blocked by real-PostgreSQL gate tests (`WAYPOINT_TEST_PG_DSN` required)
- `cd web && npm test`
- `cd web && npm run build`
- `command -v google-chrome || command -v chromium || command -v chromium-browser || command -v firefox || command -v microsoft-edge || command -v msedge || command -v safari || true` → no local browser binary
- `node -e "try{console.log(require.resolve('playwright'));}catch(e){console.error('no playwright')}"` → no Playwright
- `node -e "try{console.log(require.resolve('puppeteer'));}catch(e){console.error('no puppeteer')}"` → no Puppeteer
- `node -e "try{console.log(require.resolve('selenium-webdriver'));}catch(e){console.error('no selenium')}"` → no Selenium
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

## Blocker
This sandbox does not provide a runnable browser or a live PostgreSQL service, so I could not capture authoritative screenshots, accessibility tree output, keyboard transcript, axe report, or reduced-motion playback from the running app. The visual dogfood pass therefore remains unverified here and must be completed in a browser-enabled environment.
