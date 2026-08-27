import assert from 'node:assert/strict';
import { mkdtemp, mkdir, readFile, symlink, unlink, writeFile, rm } from 'node:fs/promises';
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

const dump = {
  formatVersion: '1.0.0',
  dumpFormat: 'postgresql-custom-reconstruction',
  engagementId: '11111111-1111-4111-8111-111111111111',
  snapshotId: '99999999-9999-4999-8999-999999999999',
  cutoff: '2025-01-15T11:00:00Z',
  engagement: { id: '11111111-1111-4111-8111-111111111111', name: 'Q3 launch', client: 'Client', scope: '10.10.12.0/24\ncorp.local', status: 'active', created_at: '2025-01-15T10:55:00Z', updated_at: '2025-01-15T11:00:00Z' },
  actors: [{ id: '22222222-2222-4222-8222-222222222222', engagement_id: '11111111-1111-4111-8111-111111111111', kind: 'human', handle: 'alex.operator', token_hash: 'a'.repeat(64), role: 'owner', created_at: '2025-01-15T10:55:00Z', updated_at: '2025-01-15T11:00:00Z' }],
  evidence: [
    { id: '33333333-3333-4333-8333-333333333333', engagement_id: '11111111-1111-4111-8111-111111111111', kind: 'stdout', sha256: 'b'.repeat(64), byte_length: 12, media_type: 'text/plain', storage_key: 'captures/stdout', created_at: '2025-01-15T10:58:00Z' },
    { id: '44444444-4444-4444-8444-444444444444', engagement_id: '11111111-1111-4111-8111-111111111111', kind: 'stderr', sha256: 'c'.repeat(64), byte_length: 8, media_type: 'text/plain', storage_key: 'captures/stderr', created_at: '2025-01-15T10:58:00Z' },
  ],
  actions: [{ id: '55555555-5555-4555-8555-555555555555', engagement_id: '11111111-1111-4111-8111-111111111111', actor_id: '22222222-2222-4222-8222-222222222222', source_agent_id: '22222222-2222-4222-8222-222222222222', initiated_by: 'manual', phase: 'recon', command: 'nmap', argv: [], cwd: '/', exec_host_ip: '10.0.0.12', egress_public_ip: '198.51.100.10', pivot_chain: [], target_kind: 'host', target_value: '10.10.12.0/24', started_at: '2025-01-15T10:58:00Z', ended_at: '2025-01-15T10:58:01Z', exit_code: 0, stdout_evidence_id: '33333333-3333-4333-8333-333333333333', stderr_evidence_id: '44444444-4444-4444-8444-444444444444', plugin_id: null, parse_status: 'raw', decision_context: null, created_at: '2025-01-15T10:58:01Z' }],
  entities: [{ id: '66666666-6666-4666-8666-666666666666', engagement_id: '11111111-1111-4111-8111-111111111111', kind: 'host', key_type: 'fqdn', key_value: 'demo.local', attributes: {}, first_seen: '2025-01-15T10:58:02Z', last_seen: '2025-01-15T10:58:02Z', merged_into_entity_id: null, created_at: '2025-01-15T10:58:02Z', updated_at: '2025-01-15T10:58:02Z' }],
  results: [{ id: '77777777-7777-4777-8777-777777777777', engagement_id: '11111111-1111-4111-8111-111111111111', action_id: '55555555-5555-4555-8555-555555555555', plugin_id: 'nmap', schema_id: 'scan.result', schema_version: '1', extracted: {}, created_at: '2025-01-15T10:58:02Z' }],
  observations: [{ id: '88888888-8888-4888-8888-888888888888', engagement_id: '11111111-1111-4111-8111-111111111111', action_id: '55555555-5555-4555-8555-555555555555', result_id: '77777777-7777-4777-8777-777777777777', entity_id: '66666666-6666-4666-8666-666666666666', kind: 'discovery', identifiers: [], attributes: {}, observed_at: '2025-01-15T10:58:02Z', created_at: '2025-01-15T10:58:02Z' }],
  claims: [{ contractVersion: '1.0.0', id: '99999999-9999-4999-8999-999999999999', engagementId: '11111111-1111-4111-8111-111111111111', claimKind: 'entity', claimedSubjectId: '66666666-6666-4666-8666-666666666666', sourceActionId: '55555555-5555-4555-8555-555555555555', detectionBoundary: 'best_effort', reason: 'missing_captured_source_action', status: 'pending', observedAt: '2025-01-15T10:59:00Z', observedBy: { id: '22222222-2222-4222-8222-222222222222', kind: 'human', handle: 'alex.operator', role: 'owner' }, revision: 1 }],
  auditEvents: [{ id: 106, engagement_id: '11111111-1111-4111-8111-111111111111', actor_id: '22222222-2222-4222-8222-222222222222', actor_kind: 'human', actor_handle: 'alex.operator', actor_role: 'owner', actor_agent_name: null, actor_model: null, actor_version: null, actor_authorized_by: null, occurred_at: '2025-01-15T10:58:03Z', type: 'finding.promoted', origin_kind: 'rest', origin_service: null, subject_type: 'finding', subject_id: 'aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa', subject_revision: 1, request_id: 'req-1', correlation_id: 'req-1', causation_action_id: '55555555-5555-4555-8555-555555555555', causation_event_id: null, data: {} }],
  findingRevisions: [{ id: 106, engagement_id: '11111111-1111-4111-8111-111111111111', actor_id: '22222222-2222-4222-8222-222222222222', actor_kind: 'human', actor_handle: 'alex.operator', actor_role: 'owner', actor_agent_name: null, actor_model: null, actor_version: null, actor_authorized_by: null, occurred_at: '2025-01-15T10:58:03Z', type: 'finding.promoted', origin_kind: 'rest', origin_service: null, subject_type: 'finding', subject_id: 'aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa', subject_revision: 1, request_id: 'req-1', correlation_id: 'req-1', causation_action_id: '55555555-5555-4555-8555-555555555555', causation_event_id: null, data: {} }],
  findings: [{ id: 'aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa', engagement_id: '11111111-1111-4111-8111-111111111111', title: 'SMB signing enforced', severity: 'low', affected_entity_ids: ['66666666-6666-4666-8666-666666666666'], evidence_action_ids: ['55555555-5555-4555-8555-555555555555'], remediation: 'Keep SMB signing enabled.', status: 'open', promoted_by: '22222222-2222-4222-8222-222222222222', promoted_at: '2025-01-15T10:58:03Z', revision: 1, created_at: '2025-01-15T10:58:03Z', updated_at: '2025-01-15T10:58:03Z' }],
  exports: [{ id: 'bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb', engagement_id: '11111111-1111-4111-8111-111111111111', requested_by: '22222222-2222-4222-8222-222222222222', retry_of_job_id: null, format_version: '1.0.0', state: 'completed', progress_stage: 'complete', progress_percent: 100, processed_bytes: 123, estimated_total_bytes: 123, snapshot_id: '99999999-9999-4999-8999-999999999999', cutoff: '2025-01-15T11:00:00Z', bundle_archive_path: 'export-bundle.tar.gz', bundle_archive_byte_length: 123, bundle_archive_sha256: 'd'.repeat(64), bundle_manifest_sha256: 'e'.repeat(64), bundle_report_snapshot_id: 'cccccccc-cccc-4ccc-8ccc-cccccccccccc', bundle_receipt_id: 'dddddddd-dddd-4ddd-8ddd-dddddddddddd', failure_code: null, failure_message: null, failure_retryable: null, created_at: '2025-01-15T11:00:00Z', started_at: '2025-01-15T10:59:30Z', completed_at: '2025-01-15T11:00:00Z', updated_at: '2025-01-15T11:00:00Z', revision: 1 }],
  receipts: [{ id: 'dddddddd-dddd-4ddd-8ddd-dddddddddddd', export_job_id: 'bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb', engagement_id: '11111111-1111-4111-8111-111111111111', status: 'verified', bundle_path: 'export-bundle.tar.gz', archive_byte_length: 123, archive_sha256: 'd'.repeat(64), manifest_sha256: 'e'.repeat(64), cutoff: '2025-01-15T11:00:00Z', verified_at: '2025-01-15T11:00:01Z', verified_by: '22222222-2222-4222-8222-222222222222', verifier_version: '1.0.0', invalidated_at: null, invalidation_reason: null, revision: 1 }],
  grants: [{ id: 'eeeeeeee-eeee-4eee-8eee-eeeeeeeeeeee', engagement_id: '11111111-1111-4111-8111-111111111111', receipt_id: 'dddddddd-dddd-4ddd-8ddd-dddddddddddd', export_job_id: 'bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb', bundle_path: 'export-bundle.tar.gz', archive_sha256: 'd'.repeat(64), manifest_sha256: 'e'.repeat(64), requested_by: '22222222-2222-4222-8222-222222222222', requested_at: '2025-01-15T11:00:02Z', expires_at: '2025-01-15T11:15:02Z', status: 'authorized', consumed_at: null, updated_at: '2025-01-15T11:00:02Z', revision: 1 }],
  rowCounts: { engagement: 1, actors: 1, actions: 1, auditEvents: 1, entities: 1, results: 1, observations: 1, evidence: 2, claims: 1, findings: 1, findingRevisions: 1, exports: 1, receipts: 1, grants: 1 },
};
await writeFile(dumpPath, JSON.stringify(dump, null, 2), 'utf8');
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
  formatVersion: '1.0.0',
  exportJobId: '77777777-7777-4777-8777-777777777777',
  engagementId: '11111111-1111-4111-8111-111111111111',
  cutoff: '2025-01-15T11:00:00Z',
  signatures: { version: 'v1', items: [] },
  payloads: [
    { path: 'bundle/database/engagement.dump', size: dumpBytes.length, sha256: sha256(dumpBytes), kind: 'database_dump' },
    { path: 'bundle/evidence/evidence.tar.zst', size: evidenceBytes.length, sha256: sha256(evidenceBytes), kind: 'evidence' },
    { path: 'bundle/report/frozen-report.pdf', size: pdfBytes.length, sha256: sha256(pdfBytes), kind: 'report_pdf' },
    { path: 'bundle/report/report-snapshot.json', size: snapshotSeed.length, sha256: sha256(snapshotSeed), kind: 'report_snapshot' },
    { path: 'bundle/metadata/export-metadata.json', size: metadataBytes.length, sha256: sha256(metadataBytes), kind: 'metadata' },
    { path: 'bundle/tools/verify-restore.mjs', size: verifyToolBytes.length, sha256: sha256(verifyToolBytes), kind: 'verify_tool' },
    { path: 'bundle/tools/regenerate-report.mjs', size: regenerateToolBytes.length, sha256: sha256(regenerateToolBytes), kind: 'restore_tool' },
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

const dumpJson = JSON.parse(await readFile(dumpPath, 'utf8'));
const mutatedCountsDump = JSON.parse(JSON.stringify(dumpJson));
mutatedCountsDump.rowCounts.actions += 1;
const mutatedCountsBytes = Buffer.from(`${JSON.stringify(mutatedCountsDump, null, 2)}\n`, 'utf8');
await writeFile(dumpPath, mutatedCountsBytes, 'utf8');
const mutatedCountsManifest = JSON.parse(JSON.stringify(manifest));
mutatedCountsManifest.payloads = mutatedCountsManifest.payloads.map((payload) => payload.path === 'bundle/database/engagement.dump'
  ? { ...payload, size: mutatedCountsBytes.length, byteLength: mutatedCountsBytes.length, sha256: sha256(mutatedCountsBytes) }
  : payload);
await assert.rejects(
  () => verifyBundle(bundleRoot, { manifest: mutatedCountsManifest, manifestBytes: Buffer.from(`${JSON.stringify(mutatedCountsManifest, null, 2)}\n`, 'utf8'), checkSidecar: false }),
  /row count mismatch for actions/,
);
await writeFile(dumpPath, JSON.stringify(dumpJson, null, 2), 'utf8');

const leakedDump = JSON.parse(await readFile(dumpPath, 'utf8'));
leakedDump.actions[0].engagement_id = '22222222-2222-4222-8222-222222222222';
const leakedBytes = Buffer.from(`${JSON.stringify(leakedDump, null, 2)}\n`, 'utf8');
await writeFile(dumpPath, leakedBytes, 'utf8');
const leakedManifest = JSON.parse(JSON.stringify(manifest));
leakedManifest.payloads = leakedManifest.payloads.map((payload) => payload.path === 'bundle/database/engagement.dump'
  ? { ...payload, size: leakedBytes.length, byteLength: leakedBytes.length, sha256: sha256(leakedBytes) }
  : payload);
await assert.rejects(
  () => verifyBundle(bundleRoot, { manifest: leakedManifest, manifestBytes: Buffer.from(`${JSON.stringify(leakedManifest, null, 2)}\n`, 'utf8'), checkSidecar: false }),
  /leaked another engagement/,
);
await writeFile(dumpPath, JSON.stringify(dumpJson, null, 2), 'utf8');

await writeFile(dumpPath, 'postgres dump bytes mutated\n', 'utf8');
await assert.rejects(() => verifyBundle(bundleRoot), /(size|sha256) mismatch for bundle\/database\/engagement\.dump/);
await writeFile(dumpPath, JSON.stringify(dumpJson, null, 2), 'utf8');

await writeFile(sidecarPath, `${archiveSha256}-tampered\n`, 'utf8');
await assert.rejects(() => verifyBundle(bundleRoot), /outer archive hash mismatch/);
await writeFile(sidecarPath, `${archiveSha256}\n`, 'utf8');

await writeFile(manifestPath, JSON.stringify({
  ...manifest,
  signatures: { version: 'v1', items: ['signed'] },
}, null, 2), 'utf8');
await assert.rejects(() => verifyBundle(bundleRoot), /bundle manifest signature hook must be versioned and empty/);
await writeFile(manifestPath, JSON.stringify(manifest, null, 2), 'utf8');

await writeFile(manifestPath, JSON.stringify({
  ...manifest,
  payloads: [...manifest.payloads, manifest.payloads[0]],
}, null, 2), 'utf8');
await assert.rejects(() => verifyBundle(bundleRoot), /duplicate bundle path: bundle\/database\/engagement\.dump/);
await writeFile(manifestPath, JSON.stringify(manifest, null, 2), 'utf8');

await writeFile(manifestPath, JSON.stringify({
  ...manifest,
  payloads: [...manifest.payloads, { path: '../escape.dump', size: 1, sha256: sha256(Buffer.from('x')), kind: 'database_dump' }],
}, null, 2), 'utf8');
await assert.rejects(() => verifyBundle(bundleRoot), /unsafe bundle path/);

const unsafeOutputHtml = join(tempRoot, 'unsafe-report.html');
const unsafeRegenRun = spawnSync('node', [resolve(repoRoot, 'bundle/tools/regenerate-report.mjs'), bundleRoot, unsafeOutputHtml], { encoding: 'utf8' });
assert.equal(unsafeRegenRun.status, 1, unsafeRegenRun.stderr);
assert.match(unsafeRegenRun.stderr, /unsafe bundle path/);

const unsafeRegenerated = await readFile(unsafeOutputHtml, 'utf8').catch(() => '');
assert.equal(unsafeRegenerated, '');
await writeFile(manifestPath, JSON.stringify(manifest, null, 2), 'utf8');

await rm(dumpPath, { force: true });
await assert.rejects(() => verifyBundle(bundleRoot), /ENOENT/);
await writeFile(dumpPath, JSON.stringify(dumpJson, null, 2), 'utf8');

const extraPath = join(bundleDir, 'notes', 'unexpected.txt');
await mkdir(join(bundleDir, 'notes'), { recursive: true });
await writeFile(extraPath, 'unexpected file\n', 'utf8');
await assert.rejects(() => verifyBundle(bundleRoot), /unexpected bundle file: bundle\/notes\/unexpected\.txt/);
await rm(extraPath, { force: true });
await rm(join(bundleDir, 'notes'), { recursive: true, force: true });

const symlinkTarget = join(tempRoot, 'symlink-target.txt');
await writeFile(symlinkTarget, 'symlink payload\n', 'utf8');
await rm(dumpPath, { force: true });
await symlink(symlinkTarget, dumpPath);
await assert.rejects(() => verifyBundle(bundleRoot), /symlink not allowed: bundle\/database\/engagement\.dump/);
await unlink(dumpPath);
await writeFile(dumpPath, JSON.stringify(dumpJson, null, 2), 'utf8');

await writeFile(manifestPath, JSON.stringify({
  ...manifest,
  payloads: manifest.payloads.map((payload) => payload.path === 'bundle/database/engagement.dump'
    ? { ...payload, size: Number.MAX_SAFE_INTEGER + 1 }
    : payload),
}, null, 2), 'utf8');
await assert.rejects(() => verifyBundle(bundleRoot), /payload size out of range/);
await writeFile(manifestPath, JSON.stringify(manifest, null, 2), 'utf8');

const parityDump = JSON.parse(JSON.stringify(dump));
parityDump.findings[0].revision += 1;
const parityDumpBytes = Buffer.from(`${JSON.stringify(parityDump, null, 2)}\n`, 'utf8');
await writeFile(dumpPath, parityDumpBytes, 'utf8');

const parityManifest = JSON.parse(JSON.stringify(manifest));
parityManifest.payloads = parityManifest.payloads.map((payload) => payload.path === 'bundle/database/engagement.dump'
  ? { ...payload, size: parityDumpBytes.length, byteLength: parityDumpBytes.length, sha256: sha256(parityDumpBytes) }
  : payload);
await writeFile(manifestPath, JSON.stringify(parityManifest, null, 2), 'utf8');

await assert.rejects(() => verifyBundle(bundleRoot), /finding revision mismatch/);
await writeFile(dumpPath, JSON.stringify(dump, null, 2), 'utf8');
await writeFile(manifestPath, JSON.stringify(manifest, null, 2), 'utf8');
