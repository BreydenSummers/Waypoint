import { access, mkdtemp, readFile, rm } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { buildWebAssets, embeddedDistRoot } from './web-assets.mjs';

const webRoot = resolve(dirname(fileURLToPath(import.meta.url)), '..');
const repoRoot = resolve(webRoot, '..');
const tempRoot = await mkdtemp(resolve(tmpdir(), 'waypoint-web-check-'));
const tempRoot2 = await mkdtemp(resolve(tmpdir(), 'waypoint-web-check-'));
const app = await readFile(resolve(webRoot, 'src/App.tsx'), 'utf8');
const styles = await readFile(resolve(webRoot, 'src/styles.css'), 'utf8');
const distBundle = await readFile(resolve(embeddedDistRoot, 'assets/waypoint.js'), 'utf8');
const distStyles = await readFile(resolve(embeddedDistRoot, 'assets/waypoint.css'), 'utf8');
const index = await readFile(resolve(webRoot, 'index.html'), 'utf8');
const distIndex = await readFile(resolve(embeddedDistRoot, 'index.html'), 'utf8');
const rootArtifacts = ['waypoint', 'server.test'];
const reportFixture = JSON.parse(await readFile(resolve(webRoot, '../contracts/v1/fixtures/report-snapshot.json'), 'utf8'));
const reportRenderer = await readFile(resolve(webRoot, 'scripts/report-renderer.mjs'), 'utf8');
const renderReportScript = await readFile(resolve(webRoot, 'scripts/render-report.mjs'), 'utf8');

function escapeHtml(value) {
  return String(value ?? '')
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#39;');
}

for (const html of [index, distIndex]) {
  if (!html.includes('id="root"') || !html.includes('/assets/waypoint.js') || !html.includes('/assets/waypoint.css')) {
    throw new Error('web shell is missing the app root or asset references');
  }
}

if (styles !== distStyles) {
  throw new Error('generated stylesheet drift detected; rerun web build');
}

if (!distBundle.includes('Waypoint · expedition shell') || !distBundle.includes('Waypoint — report snapshot') || !distBundle.includes('Journey log')) {
  throw new Error('compiled bundle is missing key source strings from App.tsx');
}
if (!distBundle.includes('Hash verified, not signed') || !distBundle.includes('Frozen report snapshot') || !distBundle.includes('sourceHash')) {
  throw new Error('compiled bundle is missing deterministic build markers');
}
if (distBundle.includes('Imported records, host notes, and discovery output stay in your pack here.')) {
  throw new Error('stale placeholder bundle text is still embedded');
}
if (distBundle.includes('window.print')) {
  throw new Error('report view still relies on browser printing');
}

for (const artifact of rootArtifacts) {
  if (await access(resolve(repoRoot, artifact)).then(() => true, () => false)) {
    throw new Error(`generated runtime artifact should not live at repo root: ${artifact}`);
  }
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

if (!reportRenderer.includes('export function buildReportHtml') || !reportRenderer.includes('escapeHtml') || !reportRenderer.includes('Hash verified, not signed')) {
  throw new Error('Report renderer helper is missing the frozen escaped template');
}
if (!renderReportScript.includes('buildReportHtml') || !renderReportScript.includes("WAYPOINT_CHROMIUM || '/usr/bin/chromium'") || !renderReportScript.includes('pathToFileURL(htmlPath).href')) {
  throw new Error('Report render script is not using the offline frozen renderer');
}
if (renderReportScript.includes('window.print')) {
  throw new Error('Report render script should not rely on browser printing');
}

const escapedSnippet = escapeHtml(reportFixture.evidence[1].rawSnippet);
if (!escapedSnippet.includes('&lt;script&gt;alert(&quot;x&quot;)&lt;/script&gt;')) {
  throw new Error('Report fixture escaping check failed for malicious raw evidence');
}

await buildWebAssets(tempRoot);
await buildWebAssets(tempRoot2);
for (const asset of ['index.html', 'assets/waypoint.css', 'assets/waypoint.js']) {
  const generated = await readFile(resolve(tempRoot, asset), 'utf8');
  const embedded = await readFile(resolve(embeddedDistRoot, asset), 'utf8');
  const secondRun = await readFile(resolve(tempRoot2, asset), 'utf8');
  if (generated !== embedded || generated !== secondRun) {
    throw new Error(`embedded web asset drift detected: ${asset}`);
  }
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

await rm(tempRoot, { recursive: true, force: true });
await rm(tempRoot2, { recursive: true, force: true });

console.log('web skeleton, bundle manifest, and report fixture check passed');
