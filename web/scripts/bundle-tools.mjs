import { createHash } from 'node:crypto';
import { readFile, writeFile, mkdir } from 'node:fs/promises';
import { dirname, resolve, relative, posix as pathPosix } from 'node:path';

import { buildReportHtml } from './report-renderer.mjs';

export function isSafeBundlePath(value) {
  if (typeof value !== 'string') return false;
  if (value.trim() === '' || value.includes('\\')) return false;
  if (value.startsWith('/') || value.startsWith('./') || value.startsWith('../')) return false;
  const normalized = pathPosix.normalize(value);
  return normalized === value && !normalized.startsWith('../') && !normalized.includes('/../') && !normalized.includes('//');
}

export function bundleFilePath(root, bundlePath) {
  return resolve(root, ...bundlePath.split('/'));
}

export function sha256(bytes) {
  return createHash('sha256').update(bytes).digest('hex');
}

export async function readJson(filePath) {
  return JSON.parse(await readFile(filePath, 'utf8'));
}

export function deriveManifestFromSnapshot(snapshot) {
  const bundle = snapshot?.bundle ?? {};
  return {
    version: 'v1',
    payloads: Array.isArray(bundle.payloads) ? bundle.payloads : [],
    signatures: bundle.signatures ?? { version: 'v1', items: [] },
    reportSnapshotPath: 'bundle/report/report-snapshot.json',
    outerArchiveSha256: typeof bundle.outerArchiveSha256 === 'string' ? bundle.outerArchiveSha256 : '',
  };
}

export async function loadBundleManifest(root, manifestPath = 'bundle/metadata/export-manifest.json', snapshotPath = 'bundle/report/report-snapshot.json') {
  const manifestFile = bundleFilePath(root, manifestPath);
  try {
    const manifest = await readJson(manifestFile);
    return { manifest, manifestPath, manifestFile, source: 'manifest' };
  } catch (err) {
    const snapshotFile = bundleFilePath(root, snapshotPath);
    const snapshot = await readJson(snapshotFile);
    return { manifest: deriveManifestFromSnapshot(snapshot), manifestPath: snapshotPath, manifestFile: snapshotFile, snapshot, source: 'snapshot' };
  }
}

export function canonicalizePayloadList(payloads) {
  return [...payloads].sort((a, b) => String(a.path).localeCompare(String(b.path)));
}

export async function computeArchiveHash(root, manifest, options = {}) {
  const excluded = new Set(Array.from(options.excludedPaths ?? []).map(String));
  const hasher = createHash('sha256');
  if (options.includeManifestBytes && options.manifestBytes) {
    hasher.update(options.manifestBytes);
  }
  for (const payload of canonicalizePayloadList(manifest.payloads ?? [])) {
    if (excluded.has(payload.path)) continue;
    const filePath = bundleFilePath(root, payload.path);
    const bytes = await readFile(filePath);
    hasher.update(payload.path);
    hasher.update('\0');
    hasher.update(bytes);
  }
  return hasher.digest('hex');
}

export async function verifyBundle(root, options = {}) {
  const bundleRoot = resolve(root ?? '.');
  const manifestPath = options.manifestPath ?? 'bundle/metadata/export-manifest.json';
  const snapshotPath = options.snapshotPath ?? 'bundle/report/report-snapshot.json';
  const sidecarPath = options.sidecarPath ?? 'bundle/metadata/export-archive.sha256';

  const loaded = await loadBundleManifest(bundleRoot, manifestPath, snapshotPath);
  const manifestBytes = await readFile(loaded.manifestFile);
  const manifest = loaded.manifest;

  if (manifest.version !== 'v1') {
    throw new Error(`unsupported bundle manifest version: ${manifest.version ?? 'unknown'}`);
  }
  if (!manifest.signatures || manifest.signatures.version !== 'v1' || !Array.isArray(manifest.signatures.items) || manifest.signatures.items.length !== 0) {
    throw new Error('bundle manifest signature hook must be versioned and empty');
  }
  if (!Array.isArray(manifest.payloads) || manifest.payloads.length === 0) {
    throw new Error('bundle manifest has no payloads');
  }

  const seen = new Set();
  for (const payload of manifest.payloads) {
    if (!isSafeBundlePath(payload.path)) {
      throw new Error(`unsafe bundle path: ${payload.path}`);
    }
    if (seen.has(payload.path)) {
      throw new Error(`duplicate bundle path: ${payload.path}`);
    }
    seen.add(payload.path);

    const filePath = bundleFilePath(bundleRoot, payload.path);
    const bytes = await readFile(filePath);
    if (Number(payload.size) !== bytes.length) {
      throw new Error(`size mismatch for ${payload.path}`);
    }
    const digest = sha256(bytes);
    if (digest !== payload.sha256) {
      throw new Error(`sha256 mismatch for ${payload.path}`);
    }
  }

  const archiveSha256 = await computeArchiveHash(bundleRoot, manifest, {
    includeManifestBytes: true,
    manifestBytes,
    excludedPaths: new Set([manifestPath, snapshotPath, sidecarPath]),
  });

  const sidecarFile = bundleFilePath(bundleRoot, sidecarPath);
  try {
    const sidecar = String(await readFile(sidecarFile, 'utf8')).trim();
    if (sidecar && sidecar !== archiveSha256) {
      throw new Error('outer archive hash mismatch');
    }
  } catch (err) {
    if (err?.code !== 'ENOENT') throw err;
  }

  return {
    bundleRoot,
    manifestPath: relative(bundleRoot, loaded.manifestFile) || manifestPath,
    payloadCount: manifest.payloads.length,
    archiveSha256,
    snapshotPath,
    source: loaded.source,
  };
}

export async function regenerateReport(root, outputPath, options = {}) {
  const bundleRoot = resolve(root ?? '.');
  const snapshotPath = options.snapshotPath ?? 'bundle/report/report-snapshot.json';
  const snapshotFile = bundleFilePath(bundleRoot, snapshotPath);
  const snapshot = await readJson(snapshotFile);
  await verifyBundle(bundleRoot, options);

  const html = buildReportHtml(snapshot);
  if (outputPath) {
    const resolved = resolve(outputPath);
    await mkdir(dirname(resolved), { recursive: true });
    await writeFile(resolved, html, 'utf8');
    return { bundleRoot, snapshotPath, outputPath: resolved };
  }

  return { bundleRoot, snapshotPath, html };
}
