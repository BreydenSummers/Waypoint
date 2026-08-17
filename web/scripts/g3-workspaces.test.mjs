import assert from 'node:assert/strict';
import { mkdtemp, readFile, rm } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import test from 'node:test';
import { buildWebAssets, embeddedDistRoot } from './web-assets.mjs';

const webRoot = dirname(fileURLToPath(import.meta.url));
const repoRoot = resolve(webRoot, '..', '..');

const readText = (relativePath) => readFile(resolve(repoRoot, relativePath), 'utf8');

await test('G3 shell keeps recon/attacks/findings non-linear and accessible', async () => {
  const app = await readText('web/src/App.tsx');
  const distBundle = await readText('internal/webassets/dist/assets/waypoint.js');

  for (const phase of ['recon', 'attacks', 'findings', 'summit']) {
    assert.match(app, new RegExp(phase));
  }

  assert.match(app, /routeFromPath\(pathname: string\)/);
  assert.match(app, /aria-current=\{waypoint\.id === activeId \? 'step' : undefined\}/);
  assert.match(app, /All phases stay accessible; fog means no data discovered yet\./);
  assert.match(app, /Guide's note/);
  assert.match(app, /Journey log/);
  assert.match(app, /No reviewed notes match this search\./);
  assert.match(app, /navigateToPhase\(engagementId, guidePhase\)/);

  assert.match(distBundle, /Journey log/);
  assert.match(distBundle, /guide-note-list/);
  assert.match(distBundle, /guide-note-empty/);
  assert.match(distBundle, /Notable alerts/);
  assert.match(distBundle, /Alerts arrive from the live SSE stream/);
  assert.match(distBundle, /No notable alerts yet/);
  assert.match(distBundle, /waypoint-hitbox/);
  assert.match(distBundle, /aria-current="step"/);
  assert.match(distBundle, /Reviewed guide notes/);
});

await test('G3 styling retains artifact surfaces in light, dark, desktop, and mobile variants', async () => {
  const styles = await readText('web/src/styles.css');

  assert.match(styles, /color-scheme: light dark;/);
  assert.match(styles, /html\[data-theme='dark'\]/);
  assert.match(styles, /@media \(prefers-color-scheme: dark\)/);
  assert.match(styles, /@media \(max-width: 980px\)/);
  assert.match(styles, /@media \(max-width: 720px\)/);
  assert.match(styles, /button:focus-visible,\s*a:focus-visible,\s*\[tabindex\]:focus-visible/);
  assert.match(styles, /\.artifact/);
  assert.match(styles, /\.guide-panel/);
  assert.match(styles, /\.log-panel/);
  assert.match(styles, /\.route-nav/);
  assert.match(styles, /\.workspace-grid/);
  assert.match(styles, /\.route-link/);
  assert.match(styles, /\.waypoint-hitbox/);
  assert.match(styles, /\.primary-button,\s*\.secondary-link/);
});

await test('G3 performance fixture keeps the 100k-action budget focused and bounded', async () => {
  const profile = JSON.parse(await readFile(resolve(repoRoot, 'contracts/v1/fixtures/performance-profile.json'), 'utf8'));
  const audit = await readText('internal/server/audit.go');

  assert.equal(profile.baseline.actions, 100000);
  assert.equal(profile.budgets.queryP95Ms, 200);
  assert.equal(profile.budgets.queryP99Ms, 500);
  assert.equal(profile.budgets.localInteractionMs, 100);
  assert.equal(profile.budgets.warmRouteUsableMs, 2000);
  assert.equal(profile.budgets.sseVisibleP95Ms, 1000);
  assert.equal(profile.budgets.ingestPeakRSSMiB, 32);

  assert.match(audit, /limit = 100/);
  assert.match(audit, /limit must be between 1 and 500/);
  assert.match(audit, /fetchLimit := limit \+ 1/);
  assert.match(audit, /hasMore := len\(items\) > limit/);
});

await test('web build output is reproducible and matches the embedded assets', async () => {
  const tempRoot = await mkdtemp(resolve(tmpdir(), 'waypoint-web-build-'));

  try {
    await buildWebAssets(tempRoot);

    for (const asset of ['index.html', 'assets/waypoint.css', 'assets/waypoint.js']) {
      const generated = await readFile(resolve(tempRoot, asset), 'utf8');
      const embedded = await readFile(resolve(embeddedDistRoot, asset), 'utf8');

      assert.equal(generated, embedded, `${asset} drifted from the compiled embedded asset`);
    }
  } finally {
    await rm(tempRoot, { recursive: true, force: true });
  }
});
