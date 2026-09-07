#!/usr/bin/env node
/*
 * Waypoint UI dogfooding harness.
 *
 * Systematically drives every view and interactive control in the running web
 * app with a headless browser, and reports bugs: uncaught page errors, console
 * errors, failed API requests, runaway request storms (e.g. a polling loop),
 * DOM garbage (undefined / NaN / [object Object] leaking into the UI), and a
 * set of per-view invariants. Exits non-zero when any bug is found, so it can
 * gate a release.
 *
 * It drives a REAL running server, so it is not part of `make test` (which has
 * no server). Run it against a seeded instance:
 *
 *   # 1. start the app (see AGENTS.md / compose.yml) and note its URL
 *   # 2. bootstrap a demo engagement and grab the owner token + engagement id
 *   #    from the response — the setup code is printed in the server startup banner:
 *   #    curl -X POST $BASE/api/v1/bootstrap \
 *   #      -H 'Content-Type: application/json' -H 'Waypoint-Contract-Version: 1.0.0' \
 *   #      -d '{"demo":true,"setupCode":"<from the banner>",
 *   #           "engagement":{"name":"Dogfood"},"owner":{"handle":"dogfood"}}'
 *   # 3. point the harness at it:
 *   DOGFOOD_BASE=http://127.0.0.1:8080 \
 *   DOGFOOD_TOKEN=<owner-token> \
 *   DOGFOOD_ENGAGEMENT=<engagement-id> \
 *   node web/scripts/dogfood.mjs
 *
 * Optional env:
 *   CHROMIUM        path to a Chromium/Chrome binary (auto-detected otherwise)
 *   DOGFOOD_JSON    set to any value to print the full report as JSON
 *   DOGFOOD_STORM   per-endpoint request cap while idling on a view (default 12)
 */

import { existsSync } from 'node:fs';
import { createRequire } from 'node:module';

const require = createRequire(import.meta.url);

const BASE = (process.env.DOGFOOD_BASE || 'http://127.0.0.1:8080').replace(/\/$/, '');
const TOKEN = process.env.DOGFOOD_TOKEN || '';
const ENGAGEMENT = process.env.DOGFOOD_ENGAGEMENT || '';
const STORM_CAP = Number(process.env.DOGFOOD_STORM || 12);

if (!TOKEN || !ENGAGEMENT) {
  console.error('dogfood: DOGFOOD_TOKEN and DOGFOOD_ENGAGEMENT are required (see the header of this file).');
  process.exit(2);
}

function resolveChromium() {
  const candidates = [
    process.env.CHROMIUM,
    '/usr/bin/chromium',
    '/usr/bin/chromium-browser',
    '/usr/bin/google-chrome',
    '/usr/bin/google-chrome-stable',
    '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome',
  ].filter(Boolean);
  for (const c of candidates) if (existsSync(c)) return c;
  return null;
}

let puppeteer;
try { puppeteer = require('puppeteer-core'); } catch { console.error('dogfood: puppeteer-core is not installed. Run `npm --prefix web install` first.'); process.exit(2); }

const bugs = [];
const notes = [];
// One entry per distinct defect: a repeating failure (e.g. the same endpoint
// failing hundreds of times) collapses into a single finding with a count,
// so a runaway loop cannot flood the report.
const bugCounts = new Map();
function bug(severity, area, message, detail) {
  const key = `${severity}|${area}|${message}|${detail || ''}`;
  const seen = bugCounts.get(key);
  if (seen) { seen.count += 1; return; }
  const entry = { severity, area, message, detail: detail || '', count: 1 };
  bugCounts.set(key, entry);
  bugs.push(entry);
}

const eng = (path) => `${BASE}/engagements/${ENGAGEMENT}${path}`;
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
const api = (path, init = {}) => fetch(`${BASE}${path}`, { ...init, headers: { Authorization: `Bearer ${TOKEN}`, 'Waypoint-Contract-Version': '1.0.0', ...(init.headers || {}) } });

// A dogfood verdict is only as good as the build it ran against: a stale
// deployment produces false failures (bugs already fixed) AND false passes
// (bugs not yet deployed). Compare the served bundle's embedded sourceHash to
// the hash of the local sources and refuse to run on a mismatch.
async function verifyDeployedBundle() {
  const { createHash } = await import('node:crypto');
  const { readFileSync } = await import('node:fs');
  const { fileURLToPath } = await import('node:url');
  const { resolve, dirname } = await import('node:path');
  const webRoot = resolve(dirname(fileURLToPath(import.meta.url)), '..');
  let expected;
  try {
    const app = readFileSync(resolve(webRoot, 'src/App.tsx'), 'utf8');
    const styles = readFileSync(resolve(webRoot, 'src/styles.css'), 'utf8');
    const runtime = readFileSync(resolve(webRoot, 'runtime/waypoint-runtime.js'), 'utf8');
    expected = createHash('sha256').update(`${app}\n${styles}\n${runtime}`).digest('hex');
  } catch {
    notes.push('skipped the deployed-bundle staleness check (local web sources not readable)');
    return;
  }
  const res = await fetch(`${BASE}/assets/waypoint.js`);
  if (!res.ok) { console.error(`dogfood: could not fetch ${BASE}/assets/waypoint.js (${res.status}).`); process.exit(2); }
  const served = (await res.text()).match(/const sourceHash = "([0-9a-f]{64})"/)?.[1];
  if (!served) { notes.push('served bundle carries no sourceHash; staleness check skipped'); return; }
  if (served !== expected) {
    console.error('dogfood: the target is serving a STALE build — its bundle hash does not match the local web sources.');
    console.error(`  served:   ${served}`);
    console.error(`  expected: ${expected}`);
    console.error('  Rebuild/redeploy the target (or run against the matching checkout) and rerun.');
    process.exit(2);
  }
}

async function main() {
  await verifyDeployedBundle();
  const exe = resolveChromium();
  if (!exe) { console.error('dogfood: no Chromium binary found. Set CHROMIUM=/path/to/chromium.'); process.exit(2); }

  const browser = await puppeteer.launch({ executablePath: exe, headless: 'new', args: ['--no-sandbox', '--disable-gpu', '--force-color-profile=srgb'] });
  const page = await browser.newPage();
  await page.setViewport({ width: 1440, height: 900, deviceScaleFactor: 1 });

  // request accounting (per endpoint) for storm detection, reset per view
  let reqCounts = {};
  const endpointOf = (url) => { try { const u = new URL(url); return u.pathname.replace(/\/[0-9a-f-]{16,}/gi, '/:id'); } catch { return url; } };
  let ctx = 'init';
  const favicon = new Set();
  page.on('pageerror', (e) => bug('high', ctx, 'uncaught page error', e.message));
  // Real console.error()s only — resource-load failures are tracked precisely via the response/requestfailed hooks below.
  page.on('console', (m) => { if (m.type() === 'error' && !/Failed to load resource/.test(m.text())) bug('medium', ctx, 'console error', m.text()); });
  page.on('requestfailed', (r) => { const u = r.url(); if (u.includes('/api/')) bug('high', ctx, 'API request failed', `${r.method()} ${endpointOf(u)} — ${r.failure() && r.failure().errorText}`); });
  page.on('response', (r) => {
    const u = r.url(); const s = r.status();
    if (s < 400) return;
    if (ctx === 'init') return; // the pre-auth boot deliberately gets 401s before the token is planted
    if (u.includes('/api/')) bug('high', ctx, 'API error response', `${s} ${endpointOf(u)}`);
    else if (/\/favicon\.ico$/.test(u)) favicon.add('/favicon.ico');
    else bug('low', ctx, 'static resource error', `${s} ${endpointOf(u)}`);
  });
  page.on('request', (r) => { const u = r.url(); if (u.includes('/api/')) { const k = endpointOf(u); reqCounts[k] = (reqCounts[k] || 0) + 1; } });
  const faviconNote = () => { if (favicon.size) notes.push('missing /favicon.ico (harmless 404 on every load — consider serving one)'); };

  // authenticate: load once, plant the token, then reload so the app boots authed
  await page.goto(eng('/attacks'), { waitUntil: 'domcontentloaded' });
  await page.evaluate((t) => localStorage.setItem('waypoint-token', t), TOKEN);
  // The first load booted before the token was planted, so its requests fell
  // back to a bogus token and 401'd. Reload so the app boots authed and let
  // those doomed responses drain before any view is measured — otherwise the
  // stray 401s bleed into the first visited view and read as a false failure.
  await page.reload({ waitUntil: 'networkidle2' });

  const $count = (sel) => page.$$eval(sel, (els) => els.length).catch(() => 0);
  const $texts = (sel) => page.$$eval(sel, (els) => els.map((e) => e.textContent.trim())).catch(() => []);

  // Land on a view, let it settle, watch for a request storm, scan for DOM garbage.
  async function visit(name, path, settleMs = 2500) {
    ctx = name;
    reqCounts = {};
    // Retry once: under a long headless run Chromium occasionally stalls a
    // navigation for reasons unrelated to the app, so a single timeout is not a
    // bug — only a repeatable failure is.
    let navErr = null;
    for (let attempt = 0; attempt < 2; attempt++) {
      try { await page.goto(BASE + path, { waitUntil: 'domcontentloaded', timeout: 20000 }); navErr = null; break; }
      catch (e) { navErr = e; await sleep(500); }
    }
    if (navErr) { bug('high', name, 'navigation failed/timeout', navErr.message); return; }
    await sleep(settleMs);
    for (const [ep, n] of Object.entries(reqCounts)) {
      if (n > STORM_CAP) bug('high', name, 'request storm', `${ep} fired ${n}x while idling ${settleMs}ms (cap ${STORM_CAP}) — likely a runaway loop`);
    }
    const garbage = await page.evaluate(() => {
      const bad = [];
      const walk = document.querySelectorAll('main *');
      for (const el of walk) {
        if (el.children.length) continue; // leaf text only
        const t = (el.textContent || '').trim();
        if (/\b(undefined|NaN|\[object Object\])\b/.test(t)) bad.push(t.slice(0, 60));
      }
      return [...new Set(bad)].slice(0, 8);
    }).catch(() => []);
    if (garbage.length) bug('medium', name, 'DOM garbage (undefined/NaN/[object Object])', garbage.join(' | '));
  }

  const click = (sel) => page.evaluate((s) => { const el = document.querySelector(s); if (el) { el.dispatchEvent(new MouseEvent('click', { bubbles: true })); return true; } return false; }, sel);

  // -------- Trail phases --------
  for (const ph of ['recon', 'attacks', 'findings', 'summit']) {
    await visit(`trail:${ph}`, `/engagements/${ENGAGEMENT}/${ph}`);
    if (await $count('.masthead') === 0) bug('high', `trail:${ph}`, 'masthead missing');
    if (await $count('.appnav') === 0) bug('high', `trail:${ph}`, 'left nav missing');
    if (await $count('.workspace-panel') === 0) bug('medium', `trail:${ph}`, 'phase workspace panel missing');
  }

  // -------- Summit export lifecycle --------
  // The endgame journey: start a persisted export job, watch it complete, and
  // confirm the verified receipt + bundle + PDF are all real. This exercises
  // the whole server-side pipeline (snapshot, archive, chromium PDF render,
  // receipt verification), which no other check touches.
  await visit('summit-export', `/engagements/${ENGAGEMENT}/summit`, 1500);
  if (!await click('[data-action="run-export"]')) bug('high', 'summit-export', 'Start export job control not found');
  else {
    let job = null;
    const deadline = Date.now() + 90000;
    while (Date.now() < deadline) {
      await sleep(1500);
      const res = await api('/api/v1/exports?limit=1').catch(() => null);
      if (!res || !res.ok) { bug('high', 'summit-export', 'exports API error while polling', res ? String(res.status) : 'fetch failed'); break; }
      job = (await res.json()).items?.[0] || null;
      if (job && ['completed', 'failed', 'cancelled'].includes(job.state)) break;
    }
    if (!job) bug('high', 'summit-export', 'export job never appeared');
    else if (job.state !== 'completed') bug('high', 'summit-export', `export job ended "${job.state}"`, job.failure ? `${job.failure.code}: ${job.failure.message}` : `stuck at ${job.progress?.stage || '?'} after 90s`);
    else {
      if (!job.bundle?.receiptId) bug('medium', 'summit-export', 'completed export has no verified receipt');
      const bundleRes = await api(`/api/v1/exports/${job.id}/bundle`).catch(() => null);
      if (!bundleRes || !bundleRes.ok) bug('high', 'summit-export', 'verified bundle download failed', bundleRes ? String(bundleRes.status) : 'fetch failed');
      await sleep(4500); // one poll interval, so the view catches up
      if (!await page.evaluate(() => document.body.innerText.toLowerCase().includes('completed'))) bug('medium', 'summit-export', 'UI did not reflect the completed export job');
      if (!await page.$eval('[data-action="open-verified-pdf"]', (e) => !e.disabled).catch(() => false)) bug('medium', 'summit-export', '"Open verified PDF" stayed disabled after completion');
    }
  }
  // The live report PDF is rendered server-side by chromium on demand — probe
  // it directly so a broken renderer (missing binary, sandbox refusal) fails loud.
  const pdfRes = await api(`/engagements/${ENGAGEMENT}/summit/report.pdf`).catch(() => null);
  if (!pdfRes || !pdfRes.ok) bug('high', 'report-pdf', 'live report PDF failed', pdfRes ? String(pdfRes.status) : 'fetch failed');
  else {
    const head = new Uint8Array((await pdfRes.arrayBuffer()).slice(0, 5));
    if (String.fromCharCode(...head) !== '%PDF-') bug('high', 'report-pdf', 'report.pdf did not return a PDF');
  }

  // -------- Report --------
  await visit('report', `/engagements/${ENGAGEMENT}/summit/report`, 3000);
  if (await $count('.report-hero') === 0 && await $count('.report-section') === 0) bug('high', 'report', 'report body did not render');

  // -------- Assets --------
  await visit('devices', `/engagements/${ENGAGEMENT}/devices`);
  const atlasNav = await page.$$eval('.appnav-item.is-active', (e) => e.map((x) => x.dataset.nav)).catch(() => []);
  if (!atlasNav.includes('devices')) bug('medium', 'assets', 'nav active-state not "devices"', JSON.stringify(atlasNav));
  const totalHosts = await $count('#asset-rows tr.arow');
  if (totalHosts === 0) bug('high', 'assets', 'asset table is empty');
  // search filters the table down
  const beforeSearch = await $count('#asset-rows tr.arow');
  await page.focus('.asearch input').catch(() => {});
  await page.type('.asearch input', 'zzz-no-such-host', { delay: 5 }).catch(() => {});
  await sleep(300);
  const afterNoMatch = await $count('#asset-rows tr.arow');
  if (afterNoMatch !== 0) bug('medium', 'assets', 'search did not filter (no-match query still shows rows)', `rows=${afterNoMatch}`);
  await page.evaluate(() => { const i = document.querySelector('.asearch input'); if (i) { i.value = ''; i.dispatchEvent(new Event('input', { bubbles: true })); } });
  await sleep(300);
  const afterClear = await $count('#asset-rows tr.arow');
  if (afterClear < Math.min(beforeSearch, 1)) bug('medium', 'assets', 'clearing search did not restore rows', `rows=${afterClear}`);
  // a tier facet narrows the table
  if (await click('[data-action="asset-tier"][data-val="0"]')) {
    await sleep(300);
    const t0 = await $count('#asset-rows tr.arow');
    if (t0 === 0) notes.push('assets: tier-0 facet yields 0 rows (ok if none)');
    if (t0 > afterClear) bug('medium', 'assets', 'tier facet increased the row count', `all=${afterClear} tier0=${t0}`);
    await click('[data-action="asset-tier"][data-val="0"]');
    await sleep(200);
  }
  // opening a row reveals the dossier drawer
  await click('#asset-rows tr.arow');
  await sleep(400);
  if (await $count('.ddrawer.is-open') === 0) bug('high', 'assets', 'asset dossier drawer did not open on row click');
  else {
    if (await $count('.ddrawer .dsec') === 0) bug('medium', 'assets', 'dossier opened but has no sections');
    // tier override control is present
    if (await $count('.ddrawer .dtier-btn') === 0) bug('medium', 'assets', 'dossier missing the tier override control');
    // a finding with a source capture reveals an attach picker
    if (await click('.ddrawer [data-action="finding-attach-toggle"]')) {
      await sleep(250);
      if (await $count('.ddrawer .dattach-list') === 0) notes.push('assets: attach toggle showed no candidate captures (ok if all linked)');
    }
    await click('.ddrawer .dclose'); await sleep(300);
    if (await $count('.ddrawer.is-open') !== 0) bug('low', 'assets', 'dossier close button did not close the drawer');
  }
  // identities tab switches dataset
  if (await click('[data-action="asset-kind"][data-kind="identities"]')) {
    await sleep(300);
    if (await $count('#asset-rows tr.arow') === 0) notes.push('assets: identities tab empty (ok if none discovered)');
    await click('[data-action="asset-kind"][data-kind="hosts"]');
    await sleep(200);
  }

  // -------- Captures --------
  await visit('captures', `/engagements/${ENGAGEMENT}/captures`);
  const capRows = await $count('#cap-rows tr.crow');
  if (capRows === 0) bug('high', 'captures', 'capture log is empty');
  // opening a capture reveals output terminals
  await click('#cap-rows tr.crow');
  await sleep(700);
  if (await $count('.ddrawer.is-open') === 0) bug('high', 'captures', 'capture detail drawer did not open');
  else {
    if (await $count('.ddrawer .cterm') < 2) bug('medium', 'captures', 'capture detail missing stdout/stderr terminals');
    // the stdout terminal should resolve out of the loading state
    const stdoutText = await page.$eval('.ddrawer .cterm pre', (e) => e.textContent).catch(() => '');
    if (/Loading…/.test(stdoutText)) bug('medium', 'captures', 'stdout evidence stuck loading');
    await click('.ddrawer .dclose'); await sleep(300);
  }
  // actor facet narrows the log
  const capAll = await $count('#cap-rows tr.crow');
  if (await click('.afacet[data-action="cap-actor"]')) {
    await sleep(300);
    const filtered = await $count('#cap-rows tr.crow');
    if (filtered > capAll) bug('medium', 'captures', 'actor facet increased the row count', `all=${capAll} filtered=${filtered}`);
    await click('.afacet.on[data-action="cap-actor"]'); await sleep(200);
  }

  // -------- Territory Map --------
  await visit('map', `/engagements/${ENGAGEMENT}/map`);
  const camps = await $count('.territory-camp');
  if (camps === 0) bug('high', 'map', 'no campsites rendered');
  if (await $count('.territory-li') === 0) bug('medium', 'map', 'severity legend missing');
  const sideDefault = await page.$eval('.territory-side .territory-nm', (e) => e.textContent).catch(() => null);
  if (!sideDefault) bug('medium', 'map', 'segment detail side panel did not populate');
  // operators lens shows trails + actor chips
  if (await click('[data-action="map-lens"][data-lens="operators"]')) {
    await sleep(600);
    const routes = await $count('.territory-route');
    const chips = await $count('.territory-achip');
    if (routes === 0) bug('medium', 'map', 'operators lens drew no trails');
    if (chips === 0) bug('medium', 'map', 'operators lens showed no "whose trail" chips');
    // highlighting an actor dims the others
    if (chips > 1 && await click('.territory-achip')) {
      await sleep(400);
      const ops = await page.$$eval('.territory-route', (e) => e.map((x) => parseFloat(x.style.opacity || '1'))).catch(() => []);
      const dimmed = ops.filter((o) => o < 0.5).length;
      if (dimmed === 0) bug('medium', 'map', 'actor highlight did not dim the other trails', JSON.stringify(ops));
      await click('.territory-achip');
    }
    await click('[data-action="map-lens"][data-lens="off"]');
  }
  // selecting a camp updates the side panel. Compare against the camp's visible
  // label, not its data-seg key: under role/zone grouping the key is a slug
  // (e.g. "rprint") while the label is human text ("Printers / IoT / OT").
  const picked = await page.evaluate(() => {
    const cs = [...document.querySelectorAll('.territory-camp')];
    const t = cs[cs.length - 1];
    if (!t) return null;
    t.dispatchEvent(new MouseEvent('click', { bubbles: true }));
    const label = t.querySelector('.territory-camp-label');
    return { seg: t.dataset.seg, label: label ? label.textContent.trim() : '' };
  });
  await sleep(400);
  const sideAfter = await page.$eval('.territory-side .territory-nm', (e) => e.textContent).catch(() => null);
  if (picked && picked.label && sideAfter && !sideAfter.includes(picked.label) && !picked.label.includes(sideAfter)) bug('medium', 'map', 'camp select did not update the side panel', `clicked=${picked.seg} label=${picked.label} side=${sideAfter}`);
  if (await $count('.territory-camp.is-sel') === 0) bug('low', 'map', 'selected camp not highlighted');

  // -------- Base Camp Board --------
  await visit('board', `/engagements/${ENGAGEMENT}/board`);
  const cols = await $count('.board-col');
  if (cols < 5) bug('high', 'board', 'board columns missing', `cols=${cols}`);
  if (await $count('.board-strip') === 0) notes.push('board: no "active now" strip (ok if no recent captures)');
  // expand a column with a "more" control
  if (await click('.board-more')) { await sleep(300); if (await $count('.board-more') === 0) notes.push('board: only column had a single "more" toggle'); }

  // -------- Left nav navigation --------
  ctx = 'nav';
  const navSeq = ['devices', 'captures', 'map', 'board', 'report', 'trail'];
  for (const n of navSeq) {
    const ok = await click(`[data-action="goto-view"][data-nav="${n}"]`);
    if (!ok) { bug('high', 'nav', `nav item "${n}" not found`); continue; }
    await sleep(700);
    const active = await page.$$eval('.appnav-item.is-active', (e) => e.map((x) => x.dataset.nav)).catch(() => []);
    if (!active.includes(n)) bug('medium', 'nav', `clicking "${n}" did not set it active`, JSON.stringify(active));
    const url = await page.evaluate(() => location.pathname);
    const wantFrag = { devices: '/devices', captures: '/captures', map: '/map', board: '/board', report: '/report', trail: '/attacks' }[n];
    if (wantFrag && !url.endsWith(wantFrag) && !(n === 'trail' && /\/(recon|attacks|findings|summit)$/.test(url))) bug('low', 'nav', `URL did not update for "${n}"`, url);
  }

  // -------- Theme toggle sanity --------
  ctx = 'theme';
  await visit('theme:devices-dark', `/engagements/${ENGAGEMENT}/devices`);
  const themeBefore = await page.evaluate(() => document.documentElement.dataset.theme);
  await click('[data-action="toggle-theme"]');
  await sleep(300);
  const themed = await page.evaluate(() => document.documentElement.dataset.theme);
  if (themed === themeBefore) bug('low', 'theme', 'theme toggle did not switch the theme', `stayed ${String(themed)}`);
  if (themed !== 'dark') await click('[data-action="toggle-theme"]').then(() => sleep(300)); // the assets check below expects dark
  if (await $count('#asset-rows tr.arow') === 0) bug('medium', 'theme', 'assets table broke after theme switch');

  faviconNote();
  await browser.close();
  report();
}

function report() {
  const order = { high: 0, medium: 1, low: 2 };
  bugs.sort((a, b) => order[a.severity] - order[b.severity]);
  if (process.env.DOGFOOD_JSON) { console.log(JSON.stringify({ bugs, notes }, null, 2)); }
  else {
    console.log(`\nWaypoint dogfood — ${bugs.length} issue(s) found\n`);
    for (const b of bugs) console.log(`  [${b.severity.toUpperCase()}] (${b.area}) ${b.message}${b.count > 1 ? ` (×${b.count})` : ''}${b.detail ? `\n        ${b.detail}` : ''}`);
    if (notes.length) { console.log('\n  notes:'); for (const n of notes) console.log(`    - ${n}`); }
    if (!bugs.length) console.log('  no issues found across all views and interactions.');
    console.log('');
  }
  process.exit(bugs.some((b) => b.severity === 'high' || b.severity === 'medium') ? 1 : 0);
}

main().catch((e) => { console.error('dogfood: harness crashed:', e.stack || e.message); process.exit(2); });
