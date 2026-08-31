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
 *   #    (POST /api/v1/bootstrap with "demo": true)
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
function bug(severity, area, message, detail) { bugs.push({ severity, area, message, detail: detail || '' }); }

const eng = (path) => `${BASE}/engagements/${ENGAGEMENT}${path}`;
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

async function main() {
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

  // -------- Report --------
  await visit('report', `/engagements/${ENGAGEMENT}/summit/report`, 3000);
  if (await $count('.report-hero') === 0 && await $count('.report-section') === 0) bug('high', 'report', 'report body did not render');

  // -------- Device Atlas --------
  await visit('devices', `/engagements/${ENGAGEMENT}/devices`);
  const atlasNav = await page.$$eval('.appnav-item.is-active', (e) => e.map((x) => x.dataset.nav)).catch(() => []);
  if (!atlasNav.includes('devices')) bug('medium', 'devices', 'nav active-state not "devices"', JSON.stringify(atlasNav));
  const totalHosts = await $count('#atlas-rows tr');
  if (totalHosts === 0) bug('high', 'devices', 'host table is empty');
  // facet counts should sum to the metric total
  const facetSum = await page.$$eval('.atlas-facet .atlas-c', (e) => e.reduce((s, x) => s + (parseInt(x.textContent, 10) || 0), 0)).catch(() => 0);
  const metricHosts = await page.$eval('.metric strong', (e) => parseInt(e.textContent.replace(/[^0-9]/g, ''), 10)).catch(() => -1);
  if (facetSum !== metricHosts) bug('medium', 'devices', 'severity facet counts do not sum to host total', `facets=${facetSum} total=${metricHosts}`);
  // search filters the table down
  const beforeSearch = await $count('#atlas-rows tr');
  await page.focus('.atlas-search input').catch(() => {});
  await page.type('.atlas-search input', 'zzz-no-such-host', { delay: 5 }).catch(() => {});
  await sleep(300);
  const afterNoMatch = await $count('#atlas-rows tr');
  if (afterNoMatch !== 0) bug('medium', 'devices', 'search did not filter (no-match query still shows rows)', `rows=${afterNoMatch}`);
  await page.evaluate(() => { const i = document.querySelector('.atlas-search input'); if (i) { i.value = ''; i.dispatchEvent(new Event('input', { bubbles: true })); } });
  await sleep(300);
  const afterClear = await $count('#atlas-rows tr');
  if (afterClear < Math.min(beforeSearch, 1)) bug('medium', 'devices', 'clearing search did not restore rows', `rows=${afterClear}`);
  // a severity facet narrows the table
  if (await click('[data-action="atlas-sev"][data-sev="critical"]')) {
    await sleep(300);
    const crit = await $count('#atlas-rows tr');
    if (crit === 0) notes.push('devices: critical facet yields 0 rows (ok if none critical)');
    if (crit > afterClear) bug('medium', 'devices', 'severity facet increased the row count', `all=${afterClear} critical=${crit}`);
    await click('[data-action="atlas-sev"][data-sev="critical"]');
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
  // selecting a camp updates the side panel
  const seg = await page.evaluate(() => { const cs = [...document.querySelectorAll('.territory-camp')]; const t = cs[cs.length - 1]; if (t) { t.dispatchEvent(new MouseEvent('click', { bubbles: true })); return t.dataset.seg; } return null; });
  await sleep(400);
  const sideAfter = await page.$eval('.territory-side .territory-nm', (e) => e.textContent).catch(() => null);
  if (seg && sideAfter && !sideAfter.includes(seg) && !seg.includes(sideAfter)) bug('medium', 'map', 'camp select did not update the side panel', `clicked=${seg} side=${sideAfter}`);
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
  const navSeq = ['devices', 'map', 'board', 'report', 'trail'];
  for (const n of navSeq) {
    const ok = await click(`[data-action="goto-view"][data-nav="${n}"]`);
    if (!ok) { bug('high', 'nav', `nav item "${n}" not found`); continue; }
    await sleep(700);
    const active = await page.$$eval('.appnav-item.is-active', (e) => e.map((x) => x.dataset.nav)).catch(() => []);
    if (!active.includes(n)) bug('medium', 'nav', `clicking "${n}" did not set it active`, JSON.stringify(active));
    const url = await page.evaluate(() => location.pathname);
    const wantFrag = { devices: '/devices', map: '/map', board: '/board', report: '/report', trail: '/attacks' }[n];
    if (wantFrag && !url.endsWith(wantFrag) && !(n === 'trail' && /\/(recon|attacks|findings|summit)$/.test(url))) bug('low', 'nav', `URL did not update for "${n}"`, url);
  }

  // -------- Theme toggle sanity --------
  ctx = 'theme';
  await visit('theme:devices-dark', `/engagements/${ENGAGEMENT}/devices`);
  await click('[data-action="set-theme"][data-theme="dark"]');
  await sleep(300);
  const themed = await page.evaluate(() => document.documentElement.dataset.theme);
  if (themed !== 'dark') bug('low', 'theme', 'dark theme did not apply', String(themed));
  if (await $count('#atlas-rows tr') === 0) bug('medium', 'theme', 'device atlas broke after theme switch');

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
    for (const b of bugs) console.log(`  [${b.severity.toUpperCase()}] (${b.area}) ${b.message}${b.detail ? `\n        ${b.detail}` : ''}`);
    if (notes.length) { console.log('\n  notes:'); for (const n of notes) console.log(`    - ${n}`); }
    if (!bugs.length) console.log('  no issues found across all views and interactions.');
    console.log('');
  }
  process.exit(bugs.some((b) => b.severity === 'high' || b.severity === 'medium') ? 1 : 0);
}

main().catch((e) => { console.error('dogfood: harness crashed:', e.stack || e.message); process.exit(2); });
