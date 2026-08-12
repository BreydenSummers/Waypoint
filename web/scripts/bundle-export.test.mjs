import assert from 'node:assert/strict';
import { mkdtemp, mkdir, readFile, writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { test } from 'node:test';

import { computeArchiveHash, finalizeBundle, sha256, verifyBundle } from './bundle-tools.mjs';

const repoRoot = resolve(dirname(fileURLToPath(import.meta.url)), '..', '..');
const fixtureSnapshot = JSON.parse(await readFile(resolve(repoRoot, 'contracts/v1/fixtures/report-snapshot.json'), 'utf8'));

async function stageBundle(root) {
  const bundleDir = join(root, 'bundle');
  const dumpPath = join(bundleDir, 'database', 'engagement.dump');
  const evidencePath = join(bundleDir, 'evidence', 'evidence.tar.zst');
  const metadataPath = join(bundleDir, 'metadata', 'export-metadata.json');
  const manifestPath = join(bundleDir, 'metadata', 'export-manifest.json');
  const sidecarPath = join(bundleDir, 'metadata', 'export-archive.sha256');
  const snapshotPath = join(bundleDir, 'report', 'report-snapshot.json');
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

  const snapshot = JSON.parse(JSON.stringify(fixtureSnapshot));
  snapshot.bundle = {
    ...snapshot.bundle,
    payloads: [
      { path: 'bundle/database/engagement.dump', size: dumpBytes.length, sha256: sha256(dumpBytes) },
      { path: 'bundle/evidence/evidence.tar.zst', size: evidenceBytes.length, sha256: sha256(evidenceBytes) },
      { path: 'bundle/report/frozen-report.pdf', size: pdfBytes.length, sha256: sha256(pdfBytes) },
      { path: 'bundle/report/report-snapshot.json', size: 0, sha256: 'pending' },
      { path: 'bundle/metadata/export-metadata.json', size: metadataBytes.length, sha256: sha256(metadataBytes) },
      { path: 'bundle/tools/verify-restore.mjs', size: verifyToolBytes.length, sha256: sha256(verifyToolBytes) },
      { path: 'bundle/tools/regenerate-report.mjs', size: regenerateToolBytes.length, sha256: sha256(regenerateToolBytes) },
    ],
    signatures: { version: 'v1', items: [] },
    outerArchiveSha256: '',
  };

  await writeFile(snapshotPath, JSON.stringify(snapshot, null, 2), 'utf8');
  const snapshotBytes = await readFile(snapshotPath);
  const snapshotPayload = snapshot.bundle.payloads.find((item) => item.path === 'bundle/report/report-snapshot.json');
  snapshotPayload.size = snapshotBytes.length;
  snapshotPayload.sha256 = sha256(snapshotBytes);

  const manifest = {
    version: 'v1',
    signatures: { version: 'v1', items: [] },
    payloads: snapshot.bundle.payloads,
  };

  const archiveSha256 = await computeArchiveHash(root, manifest, {
    includeManifestBytes: true,
    manifestBytes: Buffer.from(`${JSON.stringify(manifest, null, 2)}\n`, 'utf8'),
    excludedPaths: new Set(['bundle/metadata/export-manifest.json', 'bundle/report/report-snapshot.json', 'bundle/metadata/export-archive.sha256']),
  });

  return { bundleRoot: root, bundleDir, manifest, archiveSha256, manifestPath, sidecarPath, snapshotPath };
}

await test('finalizeBundle preflight produces no files and emits progress', async () => {
  const preflightRoot = await mkdtemp(join(tmpdir(), 'waypoint-bundle-preflight-'));
  const preflight = await stageBundle(preflightRoot);
  const preflightEvents = [];
  const preflightResult = await finalizeBundle(preflight.bundleRoot, {
    manifest: preflight.manifest,
    preflight: true,
    onProgress: (event) => preflightEvents.push(event),
  });

  assert.equal(preflightResult.archiveSha256, preflight.archiveSha256);
  assert.equal(preflightResult.payloadCount, preflight.manifest.payloads.length);
  assert.equal(preflightEvents.filter((event) => event.phase === 'payload').length, preflight.manifest.payloads.length);
  assert.equal(preflightEvents[0].phase, 'manifest');
  await assert.rejects(() => readFile(preflight.manifestPath), { code: 'ENOENT' });
  await assert.rejects(() => readFile(preflight.sidecarPath), { code: 'ENOENT' });
});

await test('finalizeBundle can recover after cancellation and verifies the archive hash', async () => {
  const root = await mkdtemp(join(tmpdir(), 'waypoint-bundle-cancel-'));
  const staged = await stageBundle(root);
  const controller = new AbortController();
  let payloadsSeen = 0;

  await assert.rejects(
    () => finalizeBundle(staged.bundleRoot, {
      manifest: staged.manifest,
      signal: controller.signal,
      onProgress: (event) => {
        if (event.phase === 'payload') {
          payloadsSeen += 1;
          if (payloadsSeen === 1) controller.abort();
        }
      },
    }),
    (error) => error && error.name === 'AbortError',
  );

  const recovered = await finalizeBundle(staged.bundleRoot, { manifest: staged.manifest, resume: true });
  assert.equal(recovered.archiveSha256, staged.archiveSha256);
  assert.equal(recovered.payloadCount, staged.manifest.payloads.length);

  const verified = await verifyBundle(staged.bundleRoot, { manifest: staged.manifest });
  assert.equal(verified.archiveSha256, staged.archiveSha256);
  assert.equal(verified.payloadCount, staged.manifest.payloads.length);
});
