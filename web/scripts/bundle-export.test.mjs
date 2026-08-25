import assert from 'node:assert/strict';
import { cp, mkdtemp, mkdir, readFile, rm, writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { test } from 'node:test';

import { buildReportHtml } from './report-renderer.mjs';
import { computeArchiveHash, finalizeBundle, regenerateReport, sha256, verifyBundle } from './bundle-tools.mjs';

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
      { path: 'bundle/database/engagement.dump', byteLength: dumpBytes.length, sha256: sha256(dumpBytes), kind: 'database_dump' },
      { path: 'bundle/evidence/evidence.tar.zst', byteLength: evidenceBytes.length, sha256: sha256(evidenceBytes), kind: 'evidence' },
      { path: 'bundle/report/frozen-report.pdf', byteLength: pdfBytes.length, sha256: sha256(pdfBytes), kind: 'report_pdf' },
      { path: 'bundle/report/report-snapshot.json', byteLength: 0, sha256: 'pending', kind: 'report_snapshot' },
      { path: 'bundle/metadata/export-metadata.json', byteLength: metadataBytes.length, sha256: sha256(metadataBytes), kind: 'metadata' },
      { path: 'bundle/tools/verify-restore.mjs', byteLength: verifyToolBytes.length, sha256: sha256(verifyToolBytes), kind: 'verify_tool' },
      { path: 'bundle/tools/regenerate-report.mjs', byteLength: regenerateToolBytes.length, sha256: sha256(regenerateToolBytes), kind: 'regenerate_tool' },
    ],
    signatures: { version: 'v1', items: [] },
    outerArchiveSha256: '',
  };

  await writeFile(snapshotPath, JSON.stringify(snapshot, null, 2), 'utf8');
  const snapshotBytes = await readFile(snapshotPath);
  const snapshotPayload = snapshot.bundle.payloads.find((item) => item.path === 'bundle/report/report-snapshot.json');
  snapshotPayload.byteLength = snapshotBytes.length;
  snapshotPayload.sha256 = sha256(snapshotBytes);

  const manifest = {
    formatVersion: '1.0.0',
    exportJobId: '77777777-7777-4777-8777-777777777777',
    engagementId: '11111111-1111-4111-8111-111111111111',
    cutoff: '2025-01-15T11:00:00Z',
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

await test('export lifecycle preserves finding-to-evidence traceability, survives concurrent capture, and reconstructs after wipe', async () => {
  const liveRoot = await mkdtemp(join(tmpdir(), 'waypoint-live-export-'));
  const staged = await stageBundle(liveRoot);
  const captureBufferPath = join(liveRoot, 'capture-buffer', 'late-collection.log');
  await mkdir(join(liveRoot, 'capture-buffer'), { recursive: true });
  let captureWrite = Promise.resolve();
  let wroteConcurrentCapture = false;

  const exported = await finalizeBundle(staged.bundleRoot, {
    manifest: staged.manifest,
    onProgress: (event) => {
      if (!wroteConcurrentCapture && event.phase === 'payload') {
        wroteConcurrentCapture = true;
        captureWrite = writeFile(captureBufferPath, 'late capture stayed outside the frozen bundle\n', 'utf8');
      }
    },
  });
  await captureWrite;

  assert.equal(exported.archiveSha256, staged.archiveSha256);
  assert.equal(exported.payloadCount, staged.manifest.payloads.length);
  assert.equal(exported.preflight, false);

  const fixtureReportHtml = buildReportHtml(fixtureSnapshot);
  assert.match(fixtureReportHtml, /SMB relay attempt blocked by signing/);
  assert.match(fixtureReportHtml, /AI-authored kerberoast probe stayed attributed/);
  assert.match(fixtureReportHtml, /Action 103/);
  assert.match(fixtureReportHtml, /Action 104/);
  assert.match(fixtureReportHtml, /authorized by alex\.operator/);
  assert.match(fixtureReportHtml, /Hash verified, not signed/);
  assert.match(fixtureReportHtml, /Signature hook/);
  assert.match(fixtureReportHtml, /empty/);
  assert.ok(!fixtureReportHtml.includes('signature verified'));
  assert.ok(!fixtureReportHtml.includes('signed by'));

  const exportRoot = await mkdtemp(join(tmpdir(), 'waypoint-exported-bundle-'));
  await cp(join(liveRoot, 'bundle'), join(exportRoot, 'bundle'), { recursive: true });
  await rm(liveRoot, { recursive: true, force: true });

  const verified = await verifyBundle(exportRoot);
  assert.equal(verified.payloadCount, staged.manifest.payloads.length);
  assert.equal(verified.archiveSha256, exported.archiveSha256);
  assert.equal(verified.source, 'manifest');

  const manifestBytes = await readFile(join(exportRoot, 'bundle/metadata/export-manifest.json'));
  const manifest = JSON.parse(manifestBytes.toString('utf8'));
  const recomputedArchiveHash = await computeArchiveHash(exportRoot, manifest, {
    includeManifestBytes: true,
    manifestBytes,
    excludedPaths: new Set(['bundle/metadata/export-manifest.json', 'bundle/report/report-snapshot.json', 'bundle/metadata/export-archive.sha256']),
  });
  assert.equal(recomputedArchiveHash, verified.archiveSha256);

  const archiveSidecar = await readFile(join(exportRoot, 'bundle/metadata/export-archive.sha256'), 'utf8');
  assert.equal(archiveSidecar.trim(), verified.archiveSha256);

  const exportedSnapshot = JSON.parse(await readFile(join(exportRoot, 'bundle/report/report-snapshot.json'), 'utf8'));
  const exportedReportHtml = buildReportHtml(exportedSnapshot);

  const restoredPath = join(exportRoot, 'restored-report.html');
  const regenerated = await regenerateReport(exportRoot, restoredPath);
  assert.equal(regenerated.outputPath, restoredPath);
  assert.equal(await readFile(restoredPath, 'utf8'), exportedReportHtml);
  assert.match(await readFile(restoredPath, 'utf8'), /Verify the outer archive hash before restore\./);
  assert.match(await readFile(restoredPath, 'utf8'), /bundle\/tools\/verify-restore\.mjs/);
  assert.match(await readFile(restoredPath, 'utf8'), /bundle\/tools\/regenerate-report\.mjs/);
});
