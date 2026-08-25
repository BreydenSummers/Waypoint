#!/usr/bin/env node
import { createHash } from 'node:crypto';
import { readFile, readdir, lstat } from 'node:fs/promises';
import { resolve, posix as pathPosix } from 'node:path';

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
  if (!manifest.signatures || manifest.signatures.version !== 'v1' || !Array.isArray(manifest.signatures.items) || manifest.signatures.items.length !== 0) {
    throw new Error('bundle manifest signature hook must be versioned and empty');
  }
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

export async function verifyBundle(root, options = {}) {
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

  for (const [index, payload] of [...manifest.payloads].sort((a, b) => String(a.path).localeCompare(String(b.path))).entries()) {
    if (options.signal?.aborted) throw new Error(options.signal.reason || 'bundle export canceled');
    const bytes = await readVerifiedBundleFile(bundleRoot, payload.path);
    const byteLength = Number.isSafeInteger(payload.byteLength) ? payload.byteLength : payload.size;
    if (bytes.length !== Number(byteLength)) throw new Error(`size mismatch for ${payload.path}`);
    if (sha256(bytes) !== payload.sha256) throw new Error(`sha256 mismatch for ${payload.path}`);
    if (typeof options.onProgress === 'function') options.onProgress({ phase: 'payload', index: index + 1, total: manifest.payloads.length, path: payload.path });
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
    if (!expectedFiles.has(filePath)) throw new Error(`unexpected bundle file: ${filePath}`);
  }

  try {
    const sidecar = String(await readFile(bundleFilePath(bundleRoot, sidecarPath), 'utf8')).trim();
    if (sidecar && sidecar.length !== 64) throw new Error('outer archive hash mismatch');
  } catch (error) {
    if (error?.code !== 'ENOENT') throw error;
  }

  return { bundleRoot, manifestPath, payloadCount: manifest.payloads.length, archiveSha256, snapshotPath, source: loaded.source };
}

async function main() {
  const bundleRoot = process.argv[2] ? process.argv[2] : '.';
  try {
    const result = await verifyBundle(bundleRoot);
    process.stdout.write(`${JSON.stringify({ status: 'verified', ...result }, null, 2)}\n`);
  } catch (error) {
    console.error(error instanceof Error ? error.message : String(error));
    process.exit(1);
  }
}

if (import.meta.url === `file://${process.argv[1]}`) {
  await main();
}
