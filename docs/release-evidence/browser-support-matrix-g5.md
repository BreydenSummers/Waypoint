# Browser support matrix — G5 validation attempt

## Verdict

**Unverified / blocked in this sandbox.**

The PRD requires current and previous major Chrome, Firefox, Edge, and Safari (`docs/v1-execution-plan.md:78`), with operator flows covering navigation, forms, SSE reconnect, report/export, responsive layouts, and secure session behavior. In this worktree I could not run authoritative browser flows because:

- no browser binaries are installed (`chrome`, `chromium`, `firefox`, `msedge`, `safari` all absent);
- no browser automation tooling is installed (`playwright`, `puppeteer`, `selenium` all absent);
- the app cannot be smoke-started here without `WAYPOINT_DB_DSN`, and no database service is available.

## Reproducible checks

```sh
command -v google-chrome || command -v chromium || command -v chromium-browser || true
command -v firefox || true
command -v microsoft-edge || command -v msedge || true
command -v safari || true
node -e "try{require('playwright'); console.log('playwright present')}catch(e){console.error('no playwright')}"
node -e "try{require('puppeteer'); console.log('puppeteer present')}catch(e){console.error('no puppeteer')}"
node -e "try{require('selenium-webdriver'); console.log('selenium present')}catch(e){console.error('no selenium')}"
make smoke
```

Observed in this gate run:

- browser/automation commands are unavailable;
- `make smoke` fails because the app cannot reach PostgreSQL here (`WAYPOINT_DB_DSN` missing / no DB service);
- `cd web && npm test` and `cd web && npm run build` both pass, so the front-end bundle is current even though no browser could be driven.

## Compatibility matrix

| Browser family | Current major | Previous major | Navigation/forms | SSE reconnect | Report/export | Responsive layouts | Secure session |
|---|---|---:|---|---|---|---|---|
| Chrome | unverified | unverified | not run | not run | not run | not run | not run |
| Firefox | unverified | unverified | not run | not run | not run | not run | not run |
| Edge | unverified | unverified | not run | not run | not run | not run | not run |
| Safari | unverified | unverified | not run | not run | not run | not run | not run |

## Notes

The existing web test suite only validates source/build artifacts; it does not drive a real browser. This matrix therefore remains pending until a browser-enabled environment with a runnable app/database stack is available.
