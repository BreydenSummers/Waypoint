#!/usr/bin/env node
import { createHash } from 'node:crypto';
import { readFile, readdir, writeFile, mkdir, lstat } from 'node:fs/promises';
import { dirname, resolve, posix as pathPosix } from 'node:path';

function escapeHtml(value) {
  return String(value ?? '')
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#39;');
}

function renderList(items) {
  if (!Array.isArray(items) || items.length === 0) {
    return '<li>None recorded.</li>';
  }
  return items.map((item) => `<li>${escapeHtml(item)}</li>`).join('');
}

function renderCards(items, renderItem) {
  return items.map((item) => `<article class="report-card">${renderItem(item)}</article>`).join('');
}

function renderInlineList(items) {
  if (!Array.isArray(items) || items.length === 0) {
    return 'None recorded.';
  }
  return items.map((item) => escapeHtml(item)).join(', ');
}

function renderCaptureGap(item) {
  if (item && typeof item === 'object') {
    const title = item.claimKind ? `${item.claimKind} claim` : 'Capture gap';
    const status = item.status ? ` · ${item.status}` : '';
    const reason = item.reason || item.notes || '';
    const observedBy = item.observedBy?.handle || item.observedBy?.title || '';
    const sourceActionId = item.sourceActionId ? ` · source ${item.sourceActionId}` : '';
    return `<li><strong>${escapeHtml(title)}</strong>${escapeHtml(status)}${sourceActionId ? escapeHtml(sourceActionId) : ''}${observedBy ? ` · ${escapeHtml(observedBy)}` : ''}${reason ? ` — ${escapeHtml(reason)}` : ''}</li>`;
  }
  return `<li>${escapeHtml(item)}</li>`;
}

function isSafeBundlePath(value) {
  if (typeof value !== 'string') return false;
  if (value.trim() === '' || value.includes('\\')) return false;
  if (value.startsWith('/') || value.startsWith('./') || value.startsWith('../')) return false;
  const normalized = pathPosix.normalize(value);
  return normalized === value && !normalized.startsWith('../') && !normalized.includes('/../') && !normalized.includes('//');
}

function bundleFilePath(root, bundlePath) {
  return resolve(root, ...bundlePath.split('/'));
}

function sha256(bytes) {
  return createHash('sha256').update(bytes).digest('hex');
}

async function readVerifiedBundleFile(root, bundlePath) {
  const filePath = bundleFilePath(root, bundlePath);
  const stats = await lstat(filePath);
  if (stats.isSymbolicLink()) throw new Error(`symlink not allowed: ${bundlePath}`);
  if (!stats.isFile()) throw new Error(`unsupported bundle entry: ${bundlePath}`);
  return readFile(filePath);
}

async function collectBundleFiles(root, relativeRoot = '') {
  const directory = relativeRoot ? bundleFilePath(root, relativeRoot) : resolve(root);
  const entries = await readdir(directory, { withFileTypes: true });
  const files = [];
  for (const entry of entries) {
    const entryPath = relativeRoot ? `${relativeRoot}/${entry.name}` : entry.name;
    if (entry.isSymbolicLink()) throw new Error(`symlink not allowed: ${entryPath}`);
    if (entry.isDirectory()) {
      files.push(...await collectBundleFiles(root, entryPath));
      continue;
    }
    if (!entry.isFile()) throw new Error(`unsupported bundle entry: ${entryPath}`);
    files.push(entryPath);
  }
  return files;
}

function validateBundleManifest(manifest) {
  if (manifest.formatVersion !== '1.0.0') throw new Error(`unsupported bundle manifest version: ${manifest.formatVersion ?? 'unknown'}`);
  if (typeof manifest.exportJobId !== 'string' || manifest.exportJobId.trim() === '') throw new Error('bundle manifest missing exportJobId');
  if (typeof manifest.engagementId !== 'string' || manifest.engagementId.trim() === '') throw new Error('bundle manifest missing engagementId');
  if (typeof manifest.cutoff !== 'string' || manifest.cutoff.trim() === '') throw new Error('bundle manifest missing cutoff');
  if (!manifest.signatures || manifest.signatures.version !== 'v1' || !Array.isArray(manifest.signatures.items) || manifest.signatures.items.length !== 0) throw new Error('bundle manifest signature hook must be versioned and empty');
  if (!Array.isArray(manifest.payloads) || manifest.payloads.length === 0) throw new Error('bundle manifest has no payloads');
  const seen = new Set();
  for (const payload of manifest.payloads) {
    if (!isSafeBundlePath(payload.path)) throw new Error(`unsafe bundle path: ${payload.path}`);
    if (seen.has(payload.path)) throw new Error(`duplicate bundle path: ${payload.path}`);
    const byteLength = Number.isSafeInteger(payload.byteLength) ? payload.byteLength : payload.size;
    if (!Number.isSafeInteger(byteLength) || byteLength < 0) throw new Error(`payload size out of range for ${payload.path}`);
    if (typeof payload.sha256 !== 'string' || payload.sha256.length !== 64) throw new Error(`payload sha256 out of range for ${payload.path}`);
    seen.add(payload.path);
  }
}

async function computeArchiveHash(root, manifest, options = {}) {
  const hasher = createHash('sha256');
  if (options.includeManifestBytes && options.manifestBytes) hasher.update(options.manifestBytes);
  const excluded = new Set(Array.from(options.excludedPaths ?? []).map(String));
  const payloads = [...manifest.payloads ?? []].sort((a, b) => String(a.path).localeCompare(String(b.path)));
  for (const payload of payloads) {
    if (excluded.has(payload.path)) continue;
    const bytes = await readVerifiedBundleFile(root, payload.path);
    hasher.update(payload.path);
    hasher.update('\0');
    hasher.update(bytes);
  }
  return hasher.digest('hex');
}

function renderReportHtml(snapshot) {
  const bundle = snapshot?.bundle ?? {};
  const contractVersion = snapshot?.contractVersion ?? snapshot?.version ?? 'v1';
  const signatures = bundle.signatures ?? { version: 'v1', items: [] };
  const restore = bundle.restore ?? { tools: [], cleanRoom: [], maliciousPaths: [] };
  const payloads = Array.isArray(bundle.payloads) ? bundle.payloads : [];
  const findings = Array.isArray(snapshot?.findings) ? snapshot.findings : [];
  const evidence = Array.isArray(snapshot?.evidence) ? snapshot.evidence : [];
  const attribution = Array.isArray(snapshot?.attribution) ? snapshot.attribution : [];
  const scope = Array.isArray(snapshot?.scope) ? snapshot.scope : [];
  const methodology = Array.isArray(snapshot?.methodology) ? snapshot.methodology : [];
  const knownCaptureGaps = Array.isArray(snapshot?.knownCaptureGaps) ? snapshot.knownCaptureGaps : [];

  return `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>${escapeHtml(snapshot?.title || 'Waypoint report')}</title>
  <style>
    :root {
      color-scheme: light;
      --deep-bark: #3B2617;
      --bark: #4A2F1B;
      --saddle: #6B4423;
      --trail: #8B5E34;
      --harvest: #BA7517;
      --lantern: #EF9F27;
      --wheat: #FAC775;
      --parchment: #FAEEDA;
      --map-cream: #E8DCC3;
      --contour: #D4C4A0;
      --dark-cocoa: #633806;
      --cocoa: #854F0B;
      --stone: #B4A78C;
    }
    * { box-sizing: border-box; }
    body {
      margin: 0;
      padding: 32px;
      background: #f4eee0;
      color: var(--deep-bark);
      font: 14px/1.5 system-ui, sans-serif;
    }
    main { max-width: 980px; margin: 0 auto; }
    .hero, .section, .card {
      border: 1px solid var(--contour);
      border-radius: 16px;
      background: var(--parchment);
      box-shadow: 0 10px 28px rgba(59, 38, 23, 0.08);
    }
    .hero { padding: 20px; margin-bottom: 16px; }
    .eyebrow {
      margin: 0 0 6px;
      text-transform: uppercase;
      letter-spacing: 0.12em;
      font-size: 12px;
      color: var(--cocoa);
    }
    h1, h2, h3, p, ul { margin: 0; }
    h1 { font-size: 30px; line-height: 1.1; color: var(--dark-cocoa); }
    .subtitle { margin-top: 8px; color: var(--cocoa); }
    .meta { margin-top: 14px; display: flex; flex-wrap: wrap; gap: 8px; }
    .pill { padding: 6px 10px; border-radius: 999px; background: rgba(186, 117, 23, 0.12); color: var(--dark-cocoa); }
    .section { padding: 18px; margin-top: 14px; }
    .section h2 { font-size: 18px; margin-bottom: 10px; color: var(--dark-cocoa); }
    .grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(240px, 1fr)); gap: 12px; }
    .card { padding: 14px; background: rgba(255,255,255,0.35); }
    .card h3 { font-size: 15px; margin-bottom: 8px; color: var(--dark-cocoa); }
    .badge { display: inline-block; margin-bottom: 8px; font-size: 12px; text-transform: uppercase; letter-spacing: 0.08em; color: var(--saddle); }
    strong { color: var(--dark-cocoa); }
    pre { margin: 10px 0 0; white-space: pre-wrap; word-break: break-word; font: inherit; color: var(--cocoa); }
    ul { padding-left: 18px; }
    li + li { margin-top: 4px; }
    .monospace { font-family: ui-monospace, SFMono-Regular, Menlo, monospace; }
  </style>
</head>
<body>
  <main>
    <section class="hero">
      <p class="eyebrow">Waypoint · frozen report snapshot</p>
      <h1>${escapeHtml(snapshot?.title || 'Runtime report snapshot')}</h1>
      <p class="subtitle">Version ${escapeHtml(contractVersion)} · ${escapeHtml(snapshot?.engagement || 'Unknown engagement')} · Cutoff ${escapeHtml(snapshot?.cutoff || 'unknown')}</p>
      <div class="meta">
        <span class="pill">Hash verified, not signed</span>
        <span class="pill">Offline renderer</span>
        <span class="pill">Snapshot frozen before print</span>
      </div>
    </section>

    <section class="section">
      <h2>Scope</h2>
      <ul>${renderList(scope)}</ul>
    </section>

    <section class="section">
      <h2>Methodology</h2>
      <ul>${renderList(methodology)}</ul>
    </section>

    <section class="section">
      <h2>Findings</h2>
      <div class="grid">
        ${renderCards(findings, (finding) => `
          <p class="badge">${escapeHtml(finding?.severity || 'Unspecified')}</p>
          <h3>${escapeHtml(finding?.title || 'Untitled finding')}</h3>
          <p>${escapeHtml(finding?.summary || '')}</p>
          <p><strong>Status:</strong> ${escapeHtml(finding?.status || 'open')}</p>
          <p><strong>Evidence:</strong> ${renderInlineList(finding?.evidence)}</p>
          <p><strong>Promoted by:</strong> ${escapeHtml(finding?.promotedBy || '')}</p>
          <p><strong>Promoted at:</strong> ${escapeHtml(finding?.promotedAt || '')}</p>
          <p><strong>Remediation:</strong> ${escapeHtml(finding?.remediation || '')}</p>
          <p><strong>Affected entities:</strong> ${renderInlineList(finding?.affectedEntityIds)}</p>
        `)}
      </div>
    </section>

    <section class="section">
      <h2>Evidence</h2>
      <div class="grid">
        ${renderCards(evidence, (item) => `
          <p class="badge">${escapeHtml(item?.label || 'Evidence')}</p>
          <p><strong>Source:</strong> ${escapeHtml(item?.source || item?.command || '')}</p>
          <p><strong>Target:</strong> ${escapeHtml(item?.target || '')}</p>
          <p><strong>Actor:</strong> ${escapeHtml(item?.actor || '')}</p>
          <p><strong>Host:</strong> ${escapeHtml(item?.host || '')}</p>
          <p><strong>Egress:</strong> ${escapeHtml(item?.egress || '')}</p>
          <p><strong>Initiated by:</strong> ${escapeHtml(item?.initiatedBy || '')}</p>
          <p><strong>Parse status:</strong> ${escapeHtml(item?.parseStatus || '')}</p>
          <p><strong>Attribution:</strong> ${escapeHtml(item?.attribution || '')}</p>
          <pre>${escapeHtml(item?.rawSnippet || '')}</pre>
          <p>${escapeHtml(item?.note || '')}</p>
        `)}
      </div>
    </section>

    <section class="section">
      <h2>Bundle manifest</h2>
      <div class="grid">
        ${payloads.map((payload) => `
          <article class="card">
            <h3 class="monospace">${escapeHtml(payload?.path || '')}</h3>
            <p><strong>Size:</strong> ${escapeHtml(payload?.size || 0)} bytes</p>
            <pre>${escapeHtml(payload?.sha256 || '')}</pre>
          </article>
        `).join('')}
      </div>
      <div class="grid" style="margin-top: 12px;">
        <article class="card">
          <h3>Archive hash</h3>
          <pre>${escapeHtml(bundle?.outerArchiveSha256 || '')}</pre>
        </article>
        <article class="card">
          <h3>Signature hook</h3>
          <p>${escapeHtml(signatures.version || 'v1')}</p>
          <p>${signatures.items?.length ? escapeHtml(signatures.items.join(', ')) : 'empty'}</p>
        </article>
      </div>
    </section>

    <section class="section">
      <h2>Verified export receipt</h2>
      <div class="grid">
        <article class="card">
          <h3>Receipt ID</h3>
          <pre>${escapeHtml(snapshot?.receipt?.id || '')}</pre>
          <p>${escapeHtml(snapshot?.receipt?.note || '')}</p>
        </article>
        <article class="card">
          <h3>Receipt state</h3>
          <p>${escapeHtml(snapshot?.receipt?.captureState || '')}</p>
          <p><strong>Verified at:</strong> ${escapeHtml(snapshot?.receipt?.verifiedAt || '')}</p>
        </article>
        <article class="card">
          <h3>Receipt manifest hash</h3>
          <pre>${escapeHtml(snapshot?.receipt?.manifestHash || '')}</pre>
        </article>
      </div>
    </section>

    <section class="section">
      <h2>Restore and regenerate</h2>
      <div class="grid">
        <article class="card">
          <h3>Offline tools</h3>
          <ul>${renderList(restore.tools)}</ul>
        </article>
        <article class="card">
          <h3>Clean-room checks</h3>
          <ul>${renderList(restore.cleanRoom)}</ul>
        </article>
        <article class="card">
          <h3>Malicious paths</h3>
          <ul>${renderList(restore.maliciousPaths)}</ul>
        </article>
      </div>
    </section>

    <section class="section">
      <h2>Attribution</h2>
      <div class="grid">
        ${renderCards(attribution, (section) => `
          <h3>${escapeHtml(section?.title || '')}</h3>
          <ul>${renderList(section?.items)}</ul>
        `)}
      </div>
    </section>

    <section class="section">
      <h2>Known capture gaps</h2>
      <ul>${Array.isArray(knownCaptureGaps) && knownCaptureGaps.length ? knownCaptureGaps.map((item) => renderCaptureGap(item)).join('') : '<li>None recorded.</li>'}</ul>
    </section>
  </main>
</body>
</html>`;
}

async function verifyBundle(root, options = {}) {
  const bundleRoot = resolve(root ?? '.');
  const manifestPath = options.manifestPath ?? 'bundle/metadata/export-manifest.json';
  const snapshotPath = options.snapshotPath ?? 'bundle/report/report-snapshot.json';
  const sidecarPath = options.sidecarPath ?? 'bundle/metadata/export-archive.sha256';
  const loaded = options.manifest
    ? { manifest: options.manifest, manifestFile: bundleFilePath(bundleRoot, manifestPath), source: 'inline' }
    : { manifest: JSON.parse(await readFile(bundleFilePath(bundleRoot, manifestPath), 'utf8')), manifestFile: bundleFilePath(bundleRoot, manifestPath), source: 'manifest' };
  const manifest = loaded.manifest;
  const manifestBytes = options.manifestBytes ?? (options.manifest ? Buffer.from(`${JSON.stringify(manifest, null, 2)}\n`, 'utf8') : await readFile(loaded.manifestFile));
  validateBundleManifest(manifest);

  for (const payload of [...manifest.payloads].sort((a, b) => String(a.path).localeCompare(String(b.path)))) {
    const bytes = await readVerifiedBundleFile(bundleRoot, payload.path);
    const byteLength = Number.isSafeInteger(payload.byteLength) ? payload.byteLength : payload.size;
    if (bytes.length !== Number(byteLength)) throw new Error(`size mismatch for ${payload.path}`);
    if (sha256(bytes) !== payload.sha256) throw new Error(`sha256 mismatch for ${payload.path}`);
  }

  const archiveSha256 = await computeArchiveHash(bundleRoot, manifest, { includeManifestBytes: true, manifestBytes, excludedPaths: new Set([manifestPath, snapshotPath, sidecarPath]) });
  const bundleTreeRoot = manifestPath.includes('/') ? manifestPath.slice(0, manifestPath.indexOf('/')) : '';
  const bundleFiles = new Set(await collectBundleFiles(bundleRoot, bundleTreeRoot));
  const expectedFiles = new Set([manifestPath, sidecarPath, ...manifest.payloads.map((payload) => payload.path)]);
  for (const filePath of bundleFiles) {
    if (!expectedFiles.has(filePath)) throw new Error(`unexpected bundle file: ${filePath}`);
  }

  return { bundleRoot, manifestPath, payloadCount: manifest.payloads.length, archiveSha256, snapshotPath, source: loaded.source };
}

export async function regenerateReport(root, outputPath, options = {}) {
  const bundleRoot = resolve(root ?? '.');
  const snapshotPath = options.snapshotPath ?? 'bundle/report/report-snapshot.json';
  const snapshot = JSON.parse(await readFile(bundleFilePath(bundleRoot, snapshotPath), 'utf8'));
  await verifyBundle(bundleRoot, options);
  const html = renderReportHtml(snapshot);
  if (outputPath) {
    const resolved = resolve(outputPath);
    await mkdir(dirname(resolved), { recursive: true });
    await writeFile(resolved, html, 'utf8');
    return { bundleRoot, snapshotPath, outputPath: resolved };
  }
  return { bundleRoot, snapshotPath, html };
}

async function main() {
  const bundleRoot = process.argv[2] ? process.argv[2] : '.';
  const outputPath = process.argv[3];
  try {
    const result = await regenerateReport(bundleRoot, outputPath);
    if (result.html) process.stdout.write(result.html);
    else process.stdout.write(`${JSON.stringify({ status: 'rendered', ...result }, null, 2)}\n`);
  } catch (error) {
    console.error(error instanceof Error ? error.message : String(error));
    process.exit(1);
  }
}

if (import.meta.url === `file://${process.argv[1]}`) {
  await main();
}
