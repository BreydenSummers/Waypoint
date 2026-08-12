import assert from 'node:assert/strict';
import { mkdtemp, mkdir, readFile, writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { spawnSync } from 'node:child_process';

import { buildReportHtml } from './report-renderer.mjs';
import { computeArchiveHash, isSafeBundlePath, sha256, verifyBundle } from './bundle-tools.mjs';

const repoRoot = resolve(dirname(fileURLToPath(import.meta.url)), '..', '..');
const fixtureSnapshot = JSON.parse(await readFile(resolve(repoRoot, 'contracts/v1/fixtures/report-snapshot.json'), 'utf8'));

assert.equal(isSafeBundlePath('bundle/report/report-snapshot.json'), true);
assert.equal(isSafeBundlePath('../escape.dump'), false);
assert.equal(isSafeBundlePath('/absolute/report.pdf'), false);

const tempRoot = await mkdtemp(join(tmpdir(), 'waypoint-bundle-'));
const bundleRoot = tempRoot;
const bundleDir = join(bundleRoot, 'bundle');
const manifestPath = join(bundleDir, 'metadata', 'export-manifest.json');
const sidecarPath = join(bundleDir, 'metadata', 'export-archive.sha256');
const snapshotPath = join(bundleDir, 'report', 'report-snapshot.json');
const metadataPath = join(bundleDir, 'metadata', 'export-metadata.json');
const dumpPath = join(bundleDir, 'database', 'engagement.dump');
const evidencePath = join(bundleDir, 'evidence', 'evidence.tar.zst');
const pdfPath = join(bundleDir, 'report', 'frozen-report.pdf');
const verifyToolPath = join(bundleDir, 'tools', 'verify-restore.mjs');
const regenerateToolPath = join(bundleDir, 'tools', 'regenerate-report.mjs');

await mkdir(join(bundleDir, 'database'), { recursive: true });
await mkdir(join(bundleDir, 'evidence'), { recursive: true });
await mkdir(join(bundleDir, 'metadata'), { recursive: true });
await mkdir(join(bundleDir, 'report'), { recursive: true });
await mkdir(join(bundleDir, 'tools'), { recursive: true });

await writeFile(dumpPath, 'postgres dump bytes\n', 'utf8');
await writeFile(evidencePath, 'evidence bytes\n', 'utf8');
await writeFile(pdfPath, '%PDF-1.4 fake report\n', 'utf8');
await writeFile(metadataPath, JSON.stringify({
  version: 'v1',
  bundleRoot: 'bundle',
  manifestPath: 'bundle/metadata/export-manifest.json',
  snapshotPath: 'bundle/report/report-snapshot.json',
  pdfPath: 'bundle/report/frozen-report.pdf',
  dumpPath: 'bundle/database/engagement.dump',
  evidencePath: 'bundle/evidence/evidence.tar.zst',
}, null, 2), 'utf8');
await writeFile(verifyToolPath, await readFile(resolve(repoRoot, 'bundle/tools/verify-restore.mjs'), 'utf8'), 'utf8');
await writeFile(regenerateToolPath, await readFile(resolve(repoRoot, 'bundle/tools/regenerate-report.mjs'), 'utf8'), 'utf8');

const dumpBytes = await readFile(dumpPath);
const evidenceBytes = await readFile(evidencePath);
const pdfBytes = await readFile(pdfPath);
const metadataBytes = await readFile(metadataPath);
const verifyToolBytes = await readFile(verifyToolPath);
const regenerateToolBytes = await readFile(regenerateToolPath);
const snapshotSeed = JSON.stringify(fixtureSnapshot, null, 2);

const manifest = {
  version: 'v1',
  signatures: { version: 'v1', items: [] },
  payloads: [
    { path: 'bundle/database/engagement.dump', size: dumpBytes.length, sha256: sha256(dumpBytes) },
    { path: 'bundle/evidence/evidence.tar.zst', size: evidenceBytes.length, sha256: sha256(evidenceBytes) },
    { path: 'bundle/report/frozen-report.pdf', size: pdfBytes.length, sha256: sha256(pdfBytes) },
    { path: 'bundle/report/report-snapshot.json', size: snapshotSeed.length, sha256: sha256(snapshotSeed) },
    { path: 'bundle/metadata/export-metadata.json', size: metadataBytes.length, sha256: sha256(metadataBytes) },
    { path: 'bundle/tools/verify-restore.mjs', size: verifyToolBytes.length, sha256: sha256(verifyToolBytes) },
    { path: 'bundle/tools/regenerate-report.mjs', size: regenerateToolBytes.length, sha256: sha256(regenerateToolBytes) },
  ],
};

const snapshot = {
  ...fixtureSnapshot,
  bundle: {
    ...fixtureSnapshot.bundle,
    payloads: manifest.payloads,
    signatures: { version: 'v1', items: [] },
    outerArchiveSha256: '',
  },
};

await writeFile(snapshotPath, JSON.stringify(snapshot, null, 2), 'utf8');
const snapshotBytes = await readFile(snapshotPath);
const snapshotPayload = manifest.payloads.find((item) => item.path === 'bundle/report/report-snapshot.json');
snapshotPayload.size = snapshotBytes.length;
snapshotPayload.sha256 = sha256(snapshotBytes);
await writeFile(manifestPath, JSON.stringify(manifest, null, 2), 'utf8');
const frozenSnapshot = JSON.parse(snapshotBytes.toString('utf8'));

const archiveSha256 = await computeArchiveHash(bundleRoot, manifest, { includeManifestBytes: true, manifestBytes: await readFile(manifestPath), excludedPaths: new Set(['bundle/metadata/export-manifest.json', 'bundle/report/report-snapshot.json', 'bundle/metadata/export-archive.sha256']) });
await writeFile(sidecarPath, `${archiveSha256}\n`, 'utf8');

const verifyRun = spawnSync('node', [resolve(repoRoot, 'bundle/tools/verify-restore.mjs'), bundleRoot], { encoding: 'utf8' });
assert.equal(verifyRun.status, 0, verifyRun.stderr);
const verifyOutput = JSON.parse(verifyRun.stdout);
assert.equal(verifyOutput.status, 'verified');
assert.equal(verifyOutput.payloadCount, manifest.payloads.length);
assert.equal(verifyOutput.archiveSha256, archiveSha256);
assert.equal(verifyOutput.source, 'manifest');
assert.equal(verifyOutput.manifestPath, 'bundle/metadata/export-manifest.json');
assert.equal(verifyOutput.snapshotPath, 'bundle/report/report-snapshot.json');

const outputHtml = join(tempRoot, 'restored-report.html');
const regenRun = spawnSync('node', [resolve(repoRoot, 'bundle/tools/regenerate-report.mjs'), bundleRoot, outputHtml], { encoding: 'utf8' });
assert.equal(regenRun.status, 0, regenRun.stderr);
const regenerated = await readFile(outputHtml, 'utf8');
assert.equal(regenerated, buildReportHtml(frozenSnapshot));
assert.match(regenerated, /Verify the outer archive hash before restore\./);
assert.match(regenerated, /bundle\/database\/engagement\.dump/);
assert.match(regenerated, /bundle\/tools\/verify-restore\.mjs/);

await writeFile(dumpPath, 'postgres dump bytes mutated\n', 'utf8');
await assert.rejects(() => verifyBundle(bundleRoot), /(size|sha256) mismatch for bundle\/database\/engagement\.dump/);
await writeFile(dumpPath, 'postgres dump bytes\n', 'utf8');

await writeFile(sidecarPath, `${archiveSha256}-tampered\n`, 'utf8');
await assert.rejects(() => verifyBundle(bundleRoot), /outer archive hash mismatch/);
await writeFile(sidecarPath, `${archiveSha256}\n`, 'utf8');

await writeFile(manifestPath, JSON.stringify({
  ...manifest,
  payloads: [...manifest.payloads, manifest.payloads[0]],
}, null, 2), 'utf8');
await assert.rejects(() => verifyBundle(bundleRoot), /duplicate bundle path: bundle\/database\/engagement\.dump/);
await writeFile(manifestPath, JSON.stringify(manifest, null, 2), 'utf8');

await writeFile(manifestPath, JSON.stringify({
  ...manifest,
  payloads: [...manifest.payloads, { path: '../escape.dump', size: 1, sha256: sha256(Buffer.from('x')) }],
}, null, 2), 'utf8');
await assert.rejects(() => verifyBundle(bundleRoot), /unsafe bundle path/);

const unsafeOutputHtml = join(tempRoot, 'unsafe-report.html');
const unsafeRegenRun = spawnSync('node', [resolve(repoRoot, 'bundle/tools/regenerate-report.mjs'), bundleRoot, unsafeOutputHtml], { encoding: 'utf8' });
assert.equal(unsafeRegenRun.status, 1, unsafeRegenRun.stderr);
assert.match(unsafeRegenRun.stderr, /unsafe bundle path/);

const unsafeRegenerated = await readFile(unsafeOutputHtml, 'utf8').catch(() => '');
assert.equal(unsafeRegenerated, '');
