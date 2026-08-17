import { createHash } from 'node:crypto';
import { readFile, readdir, writeFile, mkdir, lstat } from 'node:fs/promises';
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

async function readVerifiedBundleFile(root, bundlePath) {
  const filePath = bundleFilePath(root, bundlePath);
  const stats = await lstat(filePath);
  if (stats.isSymbolicLink()) {
    throw new Error(`symlink not allowed: ${bundlePath}`);
  }
  if (!stats.isFile()) {
    throw new Error(`unsupported bundle entry: ${bundlePath}`);
  }
  return readFile(filePath);
}

async function readVerifiedJsonBundleFile(root, bundlePath) {
  return JSON.parse((await readVerifiedBundleFile(root, bundlePath)).toString('utf8'));
}

async function collectBundleFiles(root, relativeRoot = '') {
  const directory = relativeRoot ? bundleFilePath(root, relativeRoot) : resolve(root);
  const entries = await readdir(directory, { withFileTypes: true });
  const files = [];
  for (const entry of entries) {
    const entryPath = relativeRoot ? `${relativeRoot}/${entry.name}` : entry.name;
    if (entry.isSymbolicLink()) {
      throw new Error(`symlink not allowed: ${entryPath}`);
    }
    if (entry.isDirectory()) {
      files.push(...await collectBundleFiles(root, entryPath));
      continue;
    }
    if (!entry.isFile()) {
      throw new Error(`unsupported bundle entry: ${entryPath}`);
    }
    files.push(entryPath);
  }
  return files;
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
  };
}

export async function loadBundleManifest(root, manifestPath = 'bundle/metadata/export-manifest.json', snapshotPath = 'bundle/report/report-snapshot.json') {
  const manifestFile = bundleFilePath(root, manifestPath);
  try {
    const manifest = await readVerifiedJsonBundleFile(root, manifestPath);
    return { manifest, manifestPath, manifestFile, source: 'manifest' };
  } catch (err) {
    if (err?.code !== 'ENOENT') throw err;
    const snapshotFile = bundleFilePath(root, snapshotPath);
    const snapshot = await readVerifiedJsonBundleFile(root, snapshotPath);
    return { manifest: deriveManifestFromSnapshot(snapshot), manifestPath: snapshotPath, manifestFile: snapshotFile, snapshot, source: 'snapshot' };
  }
}

export function canonicalizePayloadList(payloads) {
  return [...payloads].sort((a, b) => String(a.path).localeCompare(String(b.path)));
}

function bundleAbortError(reason) {
  const message = reason instanceof Error ? reason.message : String(reason || 'bundle export canceled');
  const error = new Error(message);
  error.name = 'AbortError';
  return error;
}

function bundleProgress(onProgress, detail) {
  if (typeof onProgress === 'function') onProgress(detail);
}

function validateBundleManifest(manifest) {
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
    if (!Number.isSafeInteger(payload.size) || payload.size < 0) {
      throw new Error(`payload size out of range for ${payload.path}`);
    }
    seen.add(payload.path);
  }
}

export async function computeArchiveHash(root, manifest, options = {}) {
  const excluded = new Set(Array.from(options.excludedPaths ?? []).map(String));
  const hasher = createHash('sha256');
  if (options.includeManifestBytes && options.manifestBytes) {
    hasher.update(options.manifestBytes);
  }
  for (const payload of canonicalizePayloadList(manifest.payloads ?? [])) {
    if (excluded.has(payload.path)) continue;
    const bytes = await readVerifiedBundleFile(root, payload.path);
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
  const onProgress = options.onProgress;
  const signal = options.signal;
  const checkSidecar = options.checkSidecar !== false;

  const loaded = options.manifest
    ? { manifest: options.manifest, manifestFile: bundleFilePath(bundleRoot, manifestPath), source: 'inline' }
    : await loadBundleManifest(bundleRoot, manifestPath, snapshotPath);
  const manifest = loaded.manifest;
  const manifestBytes = options.manifestBytes ?? (options.manifest ? Buffer.from(`${JSON.stringify(manifest, null, 2)}\n`, 'utf8') : await readFile(loaded.manifestFile));

  validateBundleManifest(manifest);

  const payloads = canonicalizePayloadList(manifest.payloads);
  for (const [index, payload] of payloads.entries()) {
    if (signal?.aborted) throw bundleAbortError(signal.reason);
    bundleProgress(onProgress, { phase: 'payload', index: index + 1, total: payloads.length, path: payload.path });

    const bytes = await readVerifiedBundleFile(bundleRoot, payload.path);
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

  const bundleTreeRoot = manifestPath.includes('/') ? manifestPath.slice(0, manifestPath.indexOf('/')) : '';
  const bundleFiles = new Set(await collectBundleFiles(bundleRoot, bundleTreeRoot));
  const expectedFiles = new Set([manifestPath, sidecarPath, ...manifest.payloads.map((payload) => payload.path)]);
  for (const filePath of bundleFiles) {
    if (!expectedFiles.has(filePath)) {
      throw new Error(`unexpected bundle file: ${filePath}`);
    }
  }

  if (checkSidecar) {
    const sidecarFile = bundleFilePath(bundleRoot, sidecarPath);
    try {
      const sidecar = String(await readFile(sidecarFile, 'utf8')).trim();
      if (sidecar && sidecar !== archiveSha256) {
        throw new Error('outer archive hash mismatch');
      }
    } catch (err) {
      if (err?.code !== 'ENOENT') throw err;
    }
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

export async function finalizeBundle(root, options = {}) {
  const bundleRoot = resolve(root ?? '.');
  const manifestPath = options.manifestPath ?? 'bundle/metadata/export-manifest.json';
  const snapshotPath = options.snapshotPath ?? 'bundle/report/report-snapshot.json';
  const sidecarPath = options.sidecarPath ?? 'bundle/metadata/export-archive.sha256';
  const preflight = Boolean(options.preflight);
  const resume = Boolean(options.resume);
  const signal = options.signal;

  if (resume) {
    try {
      const sidecarFile = bundleFilePath(bundleRoot, sidecarPath);
      await readFile(sidecarFile, 'utf8');
      return { ...(await verifyBundle(bundleRoot, options)), resumed: true };
    } catch (error) {
      if (preflight) throw error;
    }
  }

  const loaded = options.manifest ? null : await loadBundleManifest(bundleRoot, manifestPath, snapshotPath);
  const manifest = options.manifest ?? loaded.manifest;
  validateBundleManifest(manifest);

  if (signal?.aborted) throw bundleAbortError(signal.reason);
  bundleProgress(options.onProgress, { phase: 'manifest', path: manifestPath });

  if (!preflight) {
    const manifestFile = bundleFilePath(bundleRoot, manifestPath);
    await mkdir(dirname(manifestFile), { recursive: true });
    await writeFile(manifestFile, `${JSON.stringify(manifest, null, 2)}\n`, 'utf8');
  }

  const verified = await verifyBundle(bundleRoot, { ...options, manifest, manifestBytes: Buffer.from(`${JSON.stringify(manifest, null, 2)}\n`, 'utf8'), manifestPath, snapshotPath, sidecarPath, checkSidecar: false });
  if (signal?.aborted) throw bundleAbortError(signal.reason);
  bundleProgress(options.onProgress, { phase: 'sidecar', path: sidecarPath, archiveSha256: verified.archiveSha256 });

  if (!preflight) {
    const sidecarFile = bundleFilePath(bundleRoot, sidecarPath);
    await mkdir(dirname(sidecarFile), { recursive: true });
    await writeFile(sidecarFile, `${verified.archiveSha256}\n`, 'utf8');
  }

  return { ...verified, preflight, resumed: false };
}

export const exportBundle = finalizeBundle;

export async function regenerateReport(root, outputPath, options = {}) {
  const bundleRoot = resolve(root ?? '.');
  const snapshotPath = options.snapshotPath ?? 'bundle/report/report-snapshot.json';
  const snapshot = await readVerifiedJsonBundleFile(bundleRoot, snapshotPath);
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
