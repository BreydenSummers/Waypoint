import { readFile } from 'node:fs/promises';
import { dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const webRoot = resolve(dirname(fileURLToPath(import.meta.url)), '..');
const app = await readFile(resolve(webRoot, 'src/App.tsx'), 'utf8');
const distBundle = await readFile(resolve(webRoot, '../internal/webassets/dist/assets/waypoint.js'), 'utf8');
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

for (const source of [app, distBundle]) {
  if (!source.includes("workspaceTitle: 'Recon workspace'")) {
    throw new Error('Recon workspace title missing from source or embedded bundle');
  }
  if (!source.includes("workspaceLede: 'Collect raw signals, preserve provenance, and keep the audit spine instant to query.'")) {
    throw new Error('Recon workspace lede missing from source or embedded bundle');
  }
  if (!source.includes("note: 'Imported records, host notes, and discovery output stay in your pack here.'")) {
    throw new Error('Recon guide note missing from source or embedded bundle');
  }
  if (!source.includes('guideExplainers') || !source.includes('guide-note-card') || !source.includes('guide-search')) {
    throw new Error('Static guide content system is missing from source or embedded bundle');
  }
  for (const token of ['guide-recon-dns', 'guide-recon-dedup', 'guide-attacks-smb-signing', 'guide-attacks-safe-output', 'guide-findings-linking', 'guide-summit-manifest']) {
    if (!source.includes(token)) {
      throw new Error(`Guide explainer ${token} missing from source or embedded bundle`);
    }
  }
  if (!source.includes('summit/report') || !source.includes('Frozen report snapshot') || !source.includes('report-shell')) {
    throw new Error('Report snapshot route or semantic sections are missing from source or embedded bundle');
  }
  if (source.includes('exact command') || source.includes('AI claims') || source.includes('AI-generated')) {
    throw new Error('Static guide content leaked deferred v2 command or AI guidance');
  }
}

const requiredReportKeys = ['version', 'title', 'engagement', 'cutoff', 'scope', 'methodology', 'findings', 'evidence', 'attribution', 'knownCaptureGaps'];
for (const key of requiredReportKeys) {
  if (!(key in reportFixture)) {
    throw new Error(`Report fixture missing ${key}`);
  }
}

const escapedSnippet = escapeHtml(reportFixture.evidence[1].rawSnippet);
if (!escapedSnippet.includes('&lt;script&gt;alert(&quot;x&quot;)&lt;/script&gt;')) {
  throw new Error('Report fixture escaping check failed for malicious raw evidence');
}

if (reportFixture.findings.length < 2 || reportFixture.evidence.length < 3 || reportFixture.attribution.length < 4) {
  throw new Error('Report fixture does not cover the semantic report sections');
}

console.log('web skeleton and report fixture check passed');
