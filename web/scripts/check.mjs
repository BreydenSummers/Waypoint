import { readFile } from 'node:fs/promises';
import { dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const webRoot = resolve(dirname(fileURLToPath(import.meta.url)), '..');
const app = await readFile(resolve(webRoot, 'src/App.tsx'), 'utf8');
const styles = await readFile(resolve(webRoot, 'src/styles.css'), 'utf8');
const distBundle = await readFile(resolve(webRoot, '../internal/webassets/dist/assets/waypoint.js'), 'utf8');
const distStyles = await readFile(resolve(webRoot, '../internal/webassets/dist/assets/waypoint.css'), 'utf8');
const index = await readFile(resolve(webRoot, 'index.html'), 'utf8');
const distIndex = await readFile(resolve(webRoot, '../internal/webassets/dist/index.html'), 'utf8');
const reportFixture = JSON.parse(await readFile(resolve(webRoot, '../contracts/v1/fixtures/report-snapshot.json'), 'utf8'));

function escapeHtml(value) {
  return String(value ?? '')
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#39;');
}

if (app.includes("window.history.replaceState({}, '', route)") || app.includes("window.history.replaceState({}, \"\", route)")) {
  throw new Error('client route normalization still rewrites the SPA fallback URL');
}

for (const html of [index, distIndex]) {
  if (!html.includes('id="root"') || !html.includes('/assets/waypoint.js') || !html.includes('/assets/waypoint.css')) {
    throw new Error('web shell is missing the app root or asset references');
  }
}

if (styles !== distStyles) {
  throw new Error('generated stylesheet drift detected; rerun web build');
}

if (!distBundle.includes("new Proxy(phaseDataFallback")) {
  throw new Error('Runtime phase proxy is missing from the embedded bundle');
}
if (!distBundle.includes('buildRuntimeJourneyLog') || !distBundle.includes('buildRuntimeReport') || !distBundle.includes('runtimePhaseMeta') || !distBundle.includes('loadFindings')) {
  throw new Error('Runtime data helpers are missing from the embedded bundle');
}
if (!distBundle.includes("new URL('/api/v1/audit-events'") || !distBundle.includes("fetch(new URL('/api/v1/findings'")) {
  throw new Error('Authoritative API fetches are missing from the embedded bundle');
}
if (!distBundle.includes('Session revoked') || !distBundle.includes('Notable alerts') || !distBundle.includes('Optimistic conflict') || !distBundle.includes('capture.conflict')) {
  throw new Error('Browser revocation, alert, or optimistic-conflict handling is missing from the embedded bundle');
}
if (!distBundle.includes('reportSnapshotFallback') || !distBundle.includes('journeyLog = new Proxy')) {
  throw new Error('Report or journey runtime proxies are missing from the embedded bundle');
}
if (distBundle.includes('Frozen report snapshot') && !distBundle.includes('Runtime report snapshot')) {
  throw new Error('Report snapshot title was not updated to runtime wording');
}

const requiredReportKeys = ['version', 'title', 'engagement', 'cutoff', 'scope', 'methodology', 'findings', 'evidence', 'bundle', 'attribution', 'knownCaptureGaps'];
for (const key of requiredReportKeys) {
  if (!(key in reportFixture)) {
    throw new Error(`Report fixture missing ${key}`);
  }
}

if (reportFixture.title !== 'Runtime report snapshot') {
  throw new Error(`Report fixture title was not updated to runtime wording: ${reportFixture.title}`);
}

const escapedSnippet = escapeHtml(reportFixture.evidence[1].rawSnippet);
if (!escapedSnippet.includes('&lt;script&gt;alert(&quot;x&quot;)&lt;/script&gt;')) {
  throw new Error('Report fixture escaping check failed for malicious raw evidence');
}

const bundle = reportFixture.bundle;
const isSafeBundlePath = (path) => typeof path === 'string' && path.length > 0 && !path.startsWith('/') && !path.includes('..') && !path.includes('\\') && !path.includes('//');

if (!bundle.payloads.length || bundle.signatures.version !== 'v1' || bundle.signatures.items.length !== 0) {
  throw new Error('Bundle manifest signature hook is not empty and versioned');
}

for (const requiredPath of [
  'bundle/database/engagement.dump',
  'bundle/evidence/evidence.tar.zst',
  'bundle/report/frozen-report.pdf',
  'bundle/report/report-snapshot.json',
  'bundle/metadata/export-metadata.json',
  'bundle/tools/verify-restore.mjs',
  'bundle/tools/regenerate-report.mjs',
]) {
  if (!bundle.payloads.some((payload) => payload.path === requiredPath && isSafeBundlePath(payload.path))) {
    throw new Error(`Bundle payload missing or unsafe: ${requiredPath}`);
  }
}

if (bundle.restore.tools.length !== 2 || !bundle.restore.tools.every((tool) => bundle.payloads.some((payload) => payload.path === tool))) {
  throw new Error('Offline restore tools are not included as payloads');
}

if (bundle.restore.cleanRoom.length < 3 || bundle.restore.maliciousPaths.length < 3) {
  throw new Error('Bundle restore coverage is incomplete');
}

if (!bundle.restore.maliciousPaths.every((path) => !isSafeBundlePath(path))) {
  throw new Error('Malicious bundle paths were not rejected by the safety check');
}

if (bundle.restore.cleanRoom.join(' ').indexOf('frozen snapshot') === -1) {
  throw new Error('Clean-room regeneration guidance is missing');
}

if (reportFixture.findings.length < 2 || reportFixture.evidence.length < 3 || reportFixture.attribution.length < 4) {
  throw new Error('Report fixture does not cover the semantic report sections');
}

console.log('web skeleton, bundle manifest, and report fixture check passed');
