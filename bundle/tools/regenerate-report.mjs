#!/usr/bin/env node
import { createHash } from 'node:crypto';
import { readFile, readdir, writeFile, mkdir, lstat } from 'node:fs/promises';
import { dirname, resolve, posix as pathPosix } from 'node:path';

function escapeHtml(value) {
  return String(value ?? '').replace(/[&<>"']/g, (ch) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[ch]));
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
  const payloads = Array.isArray(bundle.payloads) ? bundle.payloads : [];
  const findings = Array.isArray(snapshot?.findings) ? snapshot.findings : [];
  const evidence = Array.isArray(snapshot?.evidence) ? snapshot.evidence : [];
  const attribution = Array.isArray(snapshot?.attribution) ? snapshot.attribution : [];
  const gaps = Array.isArray(snapshot?.knownCaptureGaps) ? snapshot.knownCaptureGaps : [];
  return `<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1"><title>${escapeHtml(snapshot?.title || 'Waypoint report')}</title></head><body><h1>${escapeHtml(snapshot?.title || 'Waypoint report')}</h1><p>${escapeHtml(snapshot?.engagement || '')}</p><p>${escapeHtml(snapshot?.cutoff || '')}</p><h2>Findings</h2><ul>${findings.map((finding) => `<li>${escapeHtml(finding.title)} · ${escapeHtml(finding.severity)} · ${escapeHtml(finding.status)}</li>`).join('')}</ul><h2>Evidence</h2><ul>${evidence.map((item) => `<li>${escapeHtml(item.label)} · ${escapeHtml(item.command)} · ${escapeHtml(item.sourceAgent || '')}</li>`).join('')}</ul><h2>Attribution</h2><ul>${attribution.map((item) => `<li>${escapeHtml(item.title)}: ${escapeHtml((item.items || []).join(', '))}</li>`).join('')}</ul><h2>Known capture gaps</h2><ul>${gaps.map((item) => `<li>${escapeHtml(item.claimKind || 'Capture gap')} · ${escapeHtml(item.status || '')} · ${escapeHtml(item.reason || '')}</li>`).join('')}</ul><h2>Bundle</h2><ul>${payloads.map((item) => `<li>${escapeHtml(item.path)} · ${escapeHtml(String(item.byteLength ?? item.size ?? 0))}</li>`).join('')}</ul></body></html>`;
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
  const manifestBytes = options.manifestBytes ?? Buffer.from(`${JSON.stringify(manifest, null, 2)}\n`, 'utf8');
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
