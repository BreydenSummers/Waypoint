# UX/accessibility dogfood evidence

## Checks run
- `cd web && npm test`
- `cd web && npm run build`
- `go test ./...`

## Source-level accessibility notes
- Waypoint shortcuts use real links/buttons with `aria-current="step"` on the active item.
- Map hit targets have explicit `aria-label`s.
- Focus rings are defined for `button:focus-visible`, `a:focus-visible`, and `[tabindex]:focus-visible`.
- Reduced motion is respected via `@media (prefers-reduced-motion: reduce)`.
- Mobile breakpoints collapse the layout to a single column.

## Contrast audit from CSS tokens
- cocoa on parchment: 8.72:1
- wheat on bark: 7.87:1
- wheat on deep bark: 9.14:1
- deep bark on parchment: 12.41:1
- light text on dark chrome: 14.31:1

## Blocker
This environment does not provide a runnable browser binary or a live PostgreSQL instance, so I could not capture real-browser light/dark/mobile screenshots, accessibility tree output, keyboard transcript, axe report, or reduced-motion playback from the running app. The app and tests are green, but browser dogfood evidence remains unavailable here.
