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

const allowedBundleKinds = new Set(['database_dump', 'evidence', 'report_snapshot', 'report_pdf', 'metadata', 'verify_tool', 'restore_tool', 'regenerate_tool', 'instructions']);

function inferBundlePayloadKind(bundlePath) {
  switch (bundlePath) {
    case 'bundle/database/engagement.dump':
      return 'database_dump';
    case 'bundle/evidence/evidence.tar.zst':
      return 'evidence';
    case 'bundle/report/frozen-report.pdf':
      return 'report_pdf';
    case 'bundle/report/report-snapshot.json':
      return 'report_snapshot';
    case 'bundle/metadata/export-metadata.json':
      return 'metadata';
    case 'bundle/tools/verify-restore.mjs':
      return 'verify_tool';
    case 'bundle/tools/regenerate-report.mjs':
      return 'restore_tool';
    case 'bundle/instructions/restore.md':
      return 'instructions';
    default:
      return '';
  }
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
  const payloads = Array.isArray(bundle.payloads) ? bundle.payloads.map((payload) => ({
    ...payload,
    kind: typeof payload.kind === 'string' && payload.kind ? payload.kind : inferBundlePayloadKind(payload.path),
  })) : [];
  return {
    formatVersion: '1.0.0',
    exportJobId: snapshot?.exportJobId ?? bundle.exportJobId ?? snapshot?.id ?? '',
    engagementId: snapshot?.engagement?.id ?? snapshot?.engagementId ?? bundle.engagementId ?? '',
    cutoff: snapshot?.cutoff ?? '',
    payloads,
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
  if (manifest.formatVersion !== '1.0.0') {
    throw new Error(`unsupported bundle manifest version: ${manifest.formatVersion ?? 'unknown'}`);
  }
  if (typeof manifest.exportJobId !== 'string' || manifest.exportJobId.trim() === '') {
    throw new Error('bundle manifest missing exportJobId');
  }
  if (typeof manifest.engagementId !== 'string' || manifest.engagementId.trim() === '') {
    throw new Error('bundle manifest missing engagementId');
  }
  if (typeof manifest.cutoff !== 'string' || manifest.cutoff.trim() === '') {
    throw new Error('bundle manifest missing cutoff');
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
    if (typeof payload.kind !== 'string' || !allowedBundleKinds.has(payload.kind)) {
      throw new Error(`bundle manifest payload missing kind: ${payload.path}`);
    }
    const byteLength = Number.isSafeInteger(payload.byteLength) ? payload.byteLength : payload.size;
    if (!Number.isSafeInteger(byteLength) || byteLength < 0) {
      throw new Error(`payload size out of range for ${payload.path}`);
    }
    if (typeof payload.sha256 !== 'string' || payload.sha256.length !== 64) {
      throw new Error(`payload sha256 out of range for ${payload.path}`);
    }
    seen.add(payload.path);
  }
}

function isPlainObject(value) {
  return Boolean(value) && typeof value === 'object' && !Array.isArray(value);
}

function getRowId(row) {
  const value = row?.id ?? row?.ID ?? row?.claimId ?? row?.claimID;
  return value === undefined || value === null ? '' : String(value);
}

function getRowEngagementId(row) {
  return row?.engagement_id ?? row?.engagementId ?? row?.engagementID ?? '';
}

const exportDumpMagic = 'PGDMPWP1';
const exportDumpBinaryVersion = 1;

export function encodeEngagementDump(dump) {
  const payload = Buffer.from(`${JSON.stringify(dump, null, 2)}\n`, 'utf8');
  const bytes = Buffer.alloc(exportDumpMagic.length + 8 + payload.length);
  bytes.write(exportDumpMagic, 0, 'utf8');
  bytes.writeUInt32BE(exportDumpBinaryVersion, exportDumpMagic.length);
  bytes.writeUInt32BE(payload.length, exportDumpMagic.length + 4);
  payload.copy(bytes, exportDumpMagic.length + 8);
  return bytes;
}

export function decodeEngagementDump(bytes) {
  const buffer = Buffer.isBuffer(bytes) ? bytes : Buffer.from(bytes);
  if (buffer.length >= exportDumpMagic.length + 8 && buffer.toString('utf8', 0, exportDumpMagic.length) === exportDumpMagic) {
    const version = buffer.readUInt32BE(exportDumpMagic.length);
    if (version !== exportDumpBinaryVersion) {
      throw new Error(`unsupported database dump binary version: ${version}`);
    }
    const payloadLength = buffer.readUInt32BE(exportDumpMagic.length + 4);
    const payload = buffer.subarray(exportDumpMagic.length + 8);
    if (payload.length !== payloadLength) {
      throw new Error(`database dump payload length mismatch: ${payloadLength} != ${payload.length}`);
    }
    return JSON.parse(payload.toString('utf8'));
  }
  return JSON.parse(buffer.toString('utf8'));
}

function validateEngagementDump(dump) {
  if (!isPlainObject(dump)) {
    throw new Error('database dump must be a JSON object');
  }
  if (dump.formatVersion !== '1.0.0') {
    throw new Error(`unsupported database dump version: ${dump.formatVersion ?? 'unknown'}`);
  }
  if (dump.dumpFormat !== 'postgresql-custom-reconstruction') {
    throw new Error(`unsupported database dump format: ${dump.dumpFormat ?? 'unknown'}`);
  }
  if (typeof dump.engagementId !== 'string' || dump.engagementId.trim() === '') {
    throw new Error('database dump missing engagementId');
  }
  if (!isPlainObject(dump.engagement) || dump.engagement.id !== dump.engagementId) {
    throw new Error('database dump engagement row mismatch');
  }

  const requiredArrays = [
    'actors', 'actions', 'auditEvents', 'entities', 'results', 'observations', 'evidence', 'claims', 'findings', 'findingRevisions', 'exports', 'receipts', 'grants',
  ];
  const rowSets = new Map();
  for (const key of requiredArrays) {
    if (!Array.isArray(dump[key])) {
      throw new Error(`database dump missing ${key}`);
    }
    rowSets.set(key, new Map());
  }
  if (!isPlainObject(dump.rowCounts)) {
    throw new Error('database dump missing rowCounts');
  }

  const expectedCounts = {
    engagement: 1,
    actors: dump.actors.length,
    actions: dump.actions.length,
    auditEvents: dump.auditEvents.length,
    entities: dump.entities.length,
    results: dump.results.length,
    observations: dump.observations.length,
    evidence: dump.evidence.length,
    claims: dump.claims.length,
    findings: dump.findings.length,
    findingRevisions: dump.findingRevisions.length,
    exports: dump.exports.length,
    receipts: dump.receipts.length,
    grants: dump.grants.length,
  };
  for (const [key, want] of Object.entries(expectedCounts)) {
    if (dump.rowCounts[key] !== want) {
      throw new Error(`database dump row count mismatch for ${key}`);
    }
  }

  const registerRows = (section, rows, opts = {}) => {
    const seen = rowSets.get(section);
    for (const row of rows) {
      if (!isPlainObject(row)) {
        throw new Error(`database dump section ${section} contains a non-object row`);
      }
      const id = getRowId(row);
      if (typeof id !== 'string' || id.trim() === '') {
        throw new Error(`database dump section ${section} missing row id`);
      }
      if (seen.has(id)) {
        throw new Error(`database dump section ${section} contains a duplicate row id: ${id}`);
      }
      seen.set(id, row);
      if (opts.requireEngagement !== false && getRowEngagementId(row) !== dump.engagementId) {
        throw new Error(`database dump section ${section} leaked another engagement: ${id}`);
      }
    }
  };

  registerRows('actors', dump.actors);
  registerRows('actions', dump.actions);
  registerRows('auditEvents', dump.auditEvents);
  registerRows('entities', dump.entities);
  registerRows('results', dump.results);
  registerRows('observations', dump.observations);
  registerRows('evidence', dump.evidence);
  registerRows('findings', dump.findings);
  registerRows('findingRevisions', dump.findingRevisions);
  registerRows('exports', dump.exports);
  registerRows('receipts', dump.receipts);
  registerRows('grants', dump.grants);

  const actorIds = rowSets.get('actors');
  const actionIds = rowSets.get('actions');
  const evidenceIds = rowSets.get('evidence');
  const entityIds = rowSets.get('entities');
  const resultIds = rowSets.get('results');
  const findingIds = rowSets.get('findings');
  const findingById = new Map(dump.findings.map((row) => [getRowId(row), row]));
  const exportIds = rowSets.get('exports');
  const receiptIds = rowSets.get('receipts');
  const claimIds = new Set();

  for (const row of dump.actors) {
    if (row.authorized_by && !actorIds.has(row.authorized_by)) {
      throw new Error(`database dump actor authorization target missing: ${row.id}`);
    }
  }
  for (const row of dump.actions) {
    if (!actorIds.has(row.actor_id)) {
      throw new Error(`database dump action actor missing: ${row.id}`);
    }
    if (!evidenceIds.has(row.stdout_evidence_id) || !evidenceIds.has(row.stderr_evidence_id)) {
      throw new Error(`database dump action evidence missing: ${row.id}`);
    }
  }
  for (const row of dump.results) {
    if (!actionIds.has(row.action_id)) {
      throw new Error(`database dump result action missing: ${row.id}`);
    }
  }
  for (const row of dump.observations) {
    if (!actionIds.has(row.action_id) || !resultIds.has(row.result_id)) {
      throw new Error(`database dump observation source missing: ${row.id}`);
    }
    if (row.entity_id && !entityIds.has(row.entity_id)) {
      throw new Error(`database dump observation entity missing: ${row.id}`);
    }
  }
  for (const row of dump.findings) {
    if (row.promoted_by && !actorIds.has(row.promoted_by)) {
      throw new Error(`database dump finding promoter missing: ${row.id}`);
    }
    for (const entityID of row.affected_entity_ids ?? []) {
      if (!entityIds.has(entityID)) {
        throw new Error(`database dump finding affected entity missing: ${row.id}`);
      }
    }
    for (const actionID of row.evidence_action_ids ?? []) {
      if (!actionIds.has(actionID)) {
        throw new Error(`database dump finding evidence action missing: ${row.id}`);
      }
    }
  }
  for (const row of dump.findingRevisions) {
    if (row.subject_type === 'finding' && !findingIds.has(row.subject_id)) {
      throw new Error(`database dump finding revision missing finding: ${row.id}`);
    }
  }
  for (const row of dump.exports) {
    if (!actorIds.has(row.requested_by)) {
      throw new Error(`database dump export requester missing: ${row.id}`);
    }
    if (row.retry_of_job_id && !exportIds.has(row.retry_of_job_id)) {
      throw new Error(`database dump export retry target missing: ${row.id}`);
    }
    if (row.bundle_receipt_id && !receiptIds.has(row.bundle_receipt_id)) {
      throw new Error(`database dump export receipt missing: ${row.id}`);
    }
  }
  for (const row of dump.receipts) {
    if (!exportIds.has(row.export_job_id)) {
      throw new Error(`database dump receipt export missing: ${row.id}`);
    }
    if (!actorIds.has(row.verified_by)) {
      throw new Error(`database dump receipt verifier missing: ${row.id}`);
    }
  }
  for (const row of dump.grants) {
    if (!actorIds.has(row.requested_by)) {
      throw new Error(`database dump grant requester missing: ${row.id}`);
    }
    if (!exportIds.has(row.export_job_id)) {
      throw new Error(`database dump grant export missing: ${row.id}`);
    }
    if (!receiptIds.has(row.receipt_id)) {
      throw new Error(`database dump grant receipt missing: ${row.id}`);
    }
  }
  for (const claim of dump.claims) {
    if (!isPlainObject(claim)) {
      throw new Error('database dump claims must be objects');
    }
    if (typeof claim.id !== 'string' || claim.id.trim() === '') {
      throw new Error('database dump claim missing row id');
    }
    if (claim.contractVersion !== '1.0.0') {
      throw new Error(`database dump claim contract version mismatch: ${claim.id}`);
    }
    if (claim.engagementId !== dump.engagementId) {
      throw new Error(`database dump claim leaked another engagement: ${claim.id}`);
    }
    if (claim.observedBy?.id && !actorIds.has(claim.observedBy.id)) {
      throw new Error(`database dump claim observedBy missing: ${claim.id}`);
    }
    if (claim.resolvedBy?.id && !actorIds.has(claim.resolvedBy.id)) {
      throw new Error(`database dump claim resolvedBy missing: ${claim.id}`);
    }
    if (claim.sourceActionId && !actionIds.has(claim.sourceActionId)) {
      throw new Error(`database dump claim source action missing: ${claim.id}`);
    }
    claimIds.add(claim.id);
  }

  for (const row of dump.auditEvents) {
    if (!isPlainObject(row)) {
      throw new Error('database dump auditEvents must be objects');
    }
    if (!actorIds.has(row.actor_id)) {
      throw new Error(`database dump audit event actor missing: ${row.id}`);
    }
    if (row.causation_action_id && !actionIds.has(row.causation_action_id)) {
      throw new Error(`database dump audit event causation action missing: ${row.id}`);
    }
    const subjectID = String(row.subject_id ?? '');
    switch (row.subject_type) {
      case 'action':
        if (!actionIds.has(subjectID)) {
          throw new Error(`database dump audit event action target missing: ${row.id}`);
        }
        break;
      case 'entity':
        if (!entityIds.has(subjectID)) {
          throw new Error(`database dump audit event entity target missing: ${row.id}`);
        }
        break;
      case 'finding': {
        if (!findingIds.has(subjectID)) {
          throw new Error(`database dump audit event finding target missing: ${row.id}`);
        }
        const finding = findingById.get(subjectID);
        if (finding && row.subject_revision !== finding.revision) {
          throw new Error(`database dump finding revision mismatch: ${row.id}`);
        }
        break;
      }
      case 'export':
        if (!exportIds.has(subjectID)) {
          throw new Error(`database dump audit event export target missing: ${row.id}`);
        }
        break;
      case 'out_of_band_claim':
        if (!claimIds.has(subjectID)) {
          throw new Error(`database dump audit event claim target missing: ${row.id}`);
        }
        break;
    }
  }
}

function validateExportDumpBytes(bytes) {
  const dump = decodeEngagementDump(bytes);
  validateEngagementDump(dump);
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
  let dumpBytes = null;
  for (const [index, payload] of payloads.entries()) {
    if (signal?.aborted) throw bundleAbortError(signal.reason);
    bundleProgress(onProgress, { phase: 'payload', index: index + 1, total: payloads.length, path: payload.path });

    const bytes = await readVerifiedBundleFile(bundleRoot, payload.path);
    const byteLength = Number.isSafeInteger(payload.byteLength) ? payload.byteLength : payload.size;
    if (Number(byteLength) !== bytes.length) {
      throw new Error(`size mismatch for ${payload.path}`);
    }
    const digest = sha256(bytes);
    if (digest !== payload.sha256) {
      throw new Error(`sha256 mismatch for ${payload.path}`);
    }
    if (payload.path === 'bundle/database/engagement.dump') {
      dumpBytes = bytes;
    }
  }

  if (!dumpBytes) {
    throw new Error('bundle manifest missing database dump');
  }
  validateExportDumpBytes(dumpBytes);

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
