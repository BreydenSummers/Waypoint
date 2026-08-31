const sourceHash = "728a88feafdf60093b64d9ba04b54ce5b9910282c0feb886546d7defea4f5cb6";
const sourceStrings = ["Waypoint · expedition shell","Waypoint — report snapshot","Journey log","Notable alerts","Alerts arrive from the live SSE stream","No notable alerts yet","Frozen report snapshot","Hash verified, not signed","Recon / Attacks / Findings"];
void sourceHash;
void sourceStrings;
const apiVersion = '1.0.0';
// aria-current="step" keeps the active waypoint exposed to assistive tech.
const defaultEngagementId = 'demo';
const waypointOrder = ['recon', 'attacks', 'findings', 'summit'];
const phaseNames = { recon: 'Recon', attacks: 'Attacks', findings: 'Findings', summit: 'Summit' };
const guideNotes = [
  {
    id: 'note-recon-dedup',
    phase: 'recon',
    title: 'Dedup with care',
    what: 'Use stable identifiers first, then merge only when the evidence says two sightings are the same host.',
    when: 'Best when DHCP churn, shared hostnames, or repeated observations are muddying the pack.',
    risks: 'Premature merges hide provenance. Keep the observation trail visible until the operator decides.',
  },
  {
    id: 'note-attacks-provenance',
    phase: 'attacks',
    title: 'Provenance stays attached',
    what: 'Every attempt should keep command, actor, exec host, egress, pivot chain, and raw evidence together.',
    when: 'Use this while reviewing live captures or checking whether a tool needs a parser.',
    risks: 'Rendered output should stay text-only; hostile payloads belong in evidence, not the DOM.',
  },
  {
    id: 'note-findings-promotion',
    phase: 'findings',
    title: 'Promote only confirmed results',
    what: 'Findings come from attacks, retain the source action, and stay revisioned so the report is defensible.',
    when: 'Use it once the proof is strong enough to be reportable.',
    risks: 'A finding without evidence is only a claim. Conflicts should be shown, not hidden.',
  },
  {
    id: 'note-summit-export',
    phase: 'summit',
    title: 'Freeze before teardown',
    what: 'Verify the report snapshot and SHA-256 manifest before any destroy action is armed.',
    when: 'Use this at final review when the engagement is ready to close.',
    risks: 'Hash verified never means signed. A mismatch should stop teardown immediately.',
  },
];

const positions = [
  [72, 248],
  [228, 182],
  [430, 128],
  [586, 64],
];

const state = {
  theme: 'light',
  engagementId: defaultEngagementId,
  view: 'trail',
  activePhase: 'attacks',
  mapSelectedSegment: '',
  mapLens: 'off',
  mapHighlightActor: '',
  atlasSev: new Set(),
  atlasQuery: '',
  boardExpanded: new Set(),
  token: 'demo-token',
  auditStatus: 'loading',
  actionsStatus: 'loading',
  entitiesStatus: 'loading',
  findingsStatus: 'loading',
  reportStatus: 'idle',
  auditError: '',
  actionsError: '',
  entitiesError: '',
  findingsError: '',
  reportError: '',
  auditEvents: [],
  actions: [],
  entities: [],
  findings: [],
  actorsStatus: 'loading',
  actorsError: '',
  actors: [],
  claimsStatus: 'loading',
  claimsError: '',
  claims: [],
  reportSnapshot: null,
  reportRaw: '',
  exportJobs: [],
  exportJobsStatus: 'loading',
  exportJobsError: '',
  selectedExportJobId: '',
  selectedExportJobError: '',
  selectedExportReceipt: null,
  selectedTeardownAuthorization: null,
  summitActionError: '',
  summitRequestNote: '',
  guideQuery: '',
  entityQuery: '',
  actorQuery: '',
  selectedActionId: '',
  selectedActorId: '',
  selectedClaimId: '',
  selectedEntityId: '',
  mergeSourceId: '',
  mergeTargetId: '',
  selectedObservationId: '',
  selectedFindingId: '',
  selectedEvidenceId: '',
  actorCredential: null,
  provisionDraft: { kind: 'human', handle: '', role: 'operator', agentName: '', model: '', version: '', authorizedBy: '' },
  claimResolutionDraft: { resolution: 'linked', sourceActionId: '', notes: '' },
  actorConflict: '',
  claimConflict: '',
  selectedEvidence: null,
  selectedEvidenceContent: '',
  selectedEvidenceError: '',
  actionPhaseFilter: 'attacks',
  findingDraft: { title: '', severity: 'medium', remediation: '', status: 'open' },
  findingPromotionDraft: { title: '', severity: 'medium', remediation: '', status: 'open' },
  findingConflict: '',
  entityConflict: '',
  promotionConflict: '',
  summitStatus: 'idle',
  summitProgress: 0,
  summitStep: 'Ready to preflight the bundle.',
  summitError: '',
  summitReceipt: null,
  teardownArmed: false,
  destroyPhrase: '',
  destroyed: false,
  sseAbort: null,
  streamCursor: null,
  evidenceAbort: null,
  exportAbort: null,
  exportPollTimer: null,
  exportPollAbort: null,
  renderCount: 0,
  setupRequired: false,
  setupCodeRequired: false,
  setupStep: 'form',
  setupStatus: 'idle',
  setupError: '',
  setupDraft: { code: '', engagementName: '', client: '', scope: '', ownerHandle: '', demo: false },
  setupResult: null,
};

const root = document.getElementById('root');
if (!root) throw new Error('missing app root');

function escapeHtml(value) {
  return String(value ?? '')
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#39;');
}

function jsonText(value) {
  try {
    return JSON.stringify(value, null, 2);
  } catch {
    return String(value ?? '');
  }
}

function isRecord(value) {
  return typeof value === 'object' && value !== null;
}

function getInitialTheme() {
  try {
    const stored = window.localStorage.getItem('waypoint-theme');
    if (stored === 'light' || stored === 'dark') return stored;
  } catch {
    // ignore
  }
  return window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light';
}

function getInitialEngagementId() {
  const match = window.location.pathname.match(/^\/engagements\/([^/]+)/);
  return match?.[1] || defaultEngagementId;
}

function routeFromPath(pathname) {
  if (/^\/setup\/?$/.test(pathname)) {
    return { view: 'setup', phase: 'recon' };
  }
  if (/^\/engagements\/[^/]+\/summit\/report\/?$/.test(pathname)) {
    return { view: 'report', phase: 'summit' };
  }
  if (/^\/engagements\/[^/]+\/map\/?$/.test(pathname)) {
    return { view: 'map', phase: 'attacks' };
  }
  if (/^\/engagements\/[^/]+\/devices\/?$/.test(pathname)) {
    return { view: 'devices', phase: 'attacks' };
  }
  if (/^\/engagements\/[^/]+\/board\/?$/.test(pathname)) {
    return { view: 'board', phase: 'attacks' };
  }
  const match = pathname.match(/^\/engagements\/[^/]+\/(recon|attacks|findings|summit)\/?$/);
  return match ? { view: 'trail', phase: match[1] } : { view: 'trail', phase: 'attacks' };
}

function phasePath(engagementId, phase) {
  return `/engagements/${engagementId}/${phase}`;
}

function reportPath(engagementId) {
  return `/engagements/${engagementId}/summit/report`;
}

function mapPath(engagementId) {
  return `/engagements/${engagementId}/map`;
}

function devicesPath(engagementId) {
  return `/engagements/${engagementId}/devices`;
}

function boardPath(engagementId) {
  return `/engagements/${engagementId}/board`;
}

function reportJsonPath(engagementId) {
  return `/api/v1${reportPath(engagementId)}.json`;
}

function reportPdfPath(engagementId) {
  return `${reportPath(engagementId)}.pdf`;
}

function exportsPath() {
  return '/api/v1/exports';
}

function exportJobPath(jobId) {
  return `/api/v1/exports/${jobId}`;
}

function exportCancelPath(jobId) {
  return `/api/v1/exports/${jobId}/cancel`;
}

function exportReceiptPath(receiptId) {
  return `/api/v1/export-receipts/${receiptId}`;
}

function exportReportPdfPath(jobId) {
  return `/api/v1/exports/${jobId}/report.pdf`;
}

function exportBundlePath(jobId) {
  return `/api/v1/exports/${jobId}/bundle`;
}

function teardownAuthorizationsPath() {
  return '/api/v1/teardown-authorizations';
}

function teardownAuthorizationPath(authorizationId) {
  return `/api/v1/teardown-authorizations/${authorizationId}`;
}

function teardownAuthorizationConsumePath(authorizationId) {
  return `/api/v1/teardown-authorizations/${authorizationId}/consume`;
}

function describeExportStage(stage) {
  switch (stage) {
    case 'queued':
      return 'Queued for server handling';
    case 'capacity_preflight':
      return 'Checking server capacity';
    case 'snapshot':
      return 'Freezing the live snapshot';
    case 'archive':
      return 'Assembling the bundle';
    case 'verification':
      return 'Verifying the bytes';
    case 'complete':
      return 'Export complete';
    default:
      return 'Awaiting server progress';
  }
}

function exportStatusClass(state) {
  switch (state) {
    case 'completed':
      return 'verified';
    case 'failed':
      return 'failed';
    case 'cancelled':
      return 'canceled';
    case 'queued':
    case 'preflighting':
    case 'running':
    case 'verifying':
    case 'cancel_requested':
      return 'preflight';
    default:
      return 'idle';
  }
}

function revisionHeader(revision) {
  return `W/"rev-${revision}"`;
}

function pushPath(path) {
  window.history.pushState({}, '', path);
}

function authHeaders(token, requestId) {
  const headers = {
    Authorization: `Bearer ${token}`,
    'Waypoint-Contract-Version': apiVersion,
    Accept: 'application/json',
  };
  if (requestId) headers['X-Request-ID'] = requestId;
  return headers;
}

function newRequestId() {
  return typeof crypto !== 'undefined' && 'randomUUID' in crypto ? crypto.randomUUID() : `${Date.now()}-${Math.random().toString(16).slice(2)}`;
}

async function readProblem(response) {
  try {
    const body = await response.json();
    return body.detail || body.title || `Request failed (${response.status})`;
  } catch {
    return `Request failed (${response.status})`;
  }
}

async function apiJson(path, token, signal) {
  const response = await fetch(path, { headers: authHeaders(token, newRequestId()), cache: 'no-store', signal });
  if (!response.ok) throw new Error(await readProblem(response));
  return await response.json();
}

async function apiJsonPost(path, token, body, signal) {
  const headers = { ...authHeaders(token, newRequestId()), 'Content-Type': 'application/json' };
  const response = await fetch(path, { method: 'POST', headers, cache: 'no-store', body: body === undefined ? undefined : JSON.stringify(body), signal });
  if (!response.ok) throw new Error(await readProblem(response));
  return await response.json();
}

async function apiText(path, token, signal) {
  const response = await fetch(path, {
    headers: { Authorization: `Bearer ${token}`, 'Waypoint-Contract-Version': apiVersion, Accept: '*/*' },
    cache: 'no-store',
    signal,
  });
  if (!response.ok) throw new Error(await readProblem(response));
  return await response.text();
}

async function sha256Hex(value) {
  const bytes = typeof value === 'string' ? new TextEncoder().encode(value) : value;
  const digest = await crypto.subtle.digest('SHA-256', bytes);
  return Array.from(new Uint8Array(digest), (byte) => byte.toString(16).padStart(2, '0')).join('');
}

async function readStreamBytes(response, signal, onProgress) {
  const total = Number(response.headers.get('content-length') || '0');
  if (!response.body?.getReader) {
    const bytes = new Uint8Array(await response.arrayBuffer());
    onProgress(bytes.length, bytes.length);
    return bytes;
  }
  const reader = response.body.getReader();
  const chunks = [];
  let loaded = 0;
  try {
    while (true) {
      if (signal.aborted) throw new DOMException('canceled', 'AbortError');
      const { done, value } = await reader.read();
      if (done) break;
      if (value) {
        chunks.push(value);
        loaded += value.length;
        onProgress(loaded, total || loaded);
      }
    }
  } finally {
    reader.releaseLock();
  }
  const merged = new Uint8Array(loaded);
  let offset = 0;
  for (const chunk of chunks) {
    merged.set(chunk, offset);
    offset += chunk.length;
  }
  return merged;
}

async function parseSseStream(response, signal, onEvent) {
  if (!response.body) return;
  const reader = response.body.getReader();
  const decoder = new TextDecoder();
  let buffer = '';
  let current = { id: '', event: 'message', data: '' };

  const dispatch = () => {
    if (current.data !== '') onEvent({ id: current.id || undefined, event: current.event || 'message', data: current.data.replace(/\n$/, '') });
    current = { id: '', event: 'message', data: '' };
  };

  try {
    while (true) {
      if (signal.aborted) return;
      const { done, value } = await reader.read();
      if (done) {
        dispatch();
        return;
      }
      buffer += decoder.decode(value, { stream: true });
      let newlineIndex = buffer.indexOf('\n');
      while (newlineIndex >= 0) {
        const line = buffer.slice(0, newlineIndex).replace(/\r$/, '');
        buffer = buffer.slice(newlineIndex + 1);
        if (!line) {
          dispatch();
        } else if (line.startsWith('id:')) {
          current.id = line.slice(3).trim();
        } else if (line.startsWith('event:')) {
          current.event = line.slice(6).trim();
        } else if (line.startsWith('data:')) {
          current.data += `${line.slice(5).trimStart()}\n`;
        }
        newlineIndex = buffer.indexOf('\n');
      }
    }
  } finally {
    reader.releaseLock();
  }
}

function formatTime(iso) {
  const date = new Date(iso);
  return Number.isNaN(date.getTime()) ? iso : date.toLocaleString();
}

function shortStateLabel(state) {
  if (state === 'completed') return 'Done';
  if (state === 'current') return 'Here';
  return 'Fog';
}

function stateLabel(state) {
  return state === 'completed' ? 'completed' : state === 'current' ? 'current' : 'fog';
}

function waypointState(index) {
  const activeIndex = waypointOrder.indexOf(state.activePhase);
  if (index < activeIndex) return 'completed';
  if (index === activeIndex) return 'current';
  return 'fog';
}

function phaseSummary(phase) {
  return {
    recon: 'Collect signals, preserve provenance, and merge only when the evidence is steady.',
    attacks: 'Every attempt stays attributed, searchable, and linkable to its raw evidence.',
    findings: 'Only confirmed results become findings; conflicts and revisions stay visible.',
    summit: 'Verify the snapshot, freeze the bundle, and keep teardown guarded by the receipt.',
  }[phase];
}

function summaryLineForAction(action) {
  return `${action.capture.command} · ${action.capture.target.value}`;
}

function summaryLineForEntity(entity) {
  const label = (entity.identifiers || []).map((identifier) => `${identifier.type}:${identifier.value}`).join(' · ');
  return label || entity.kind || 'entity';
}

function summaryLineForFinding(finding) {
  return `${finding.severity} · ${finding.status}`;
}

function buildAuditSummary(event) {
  const details = isRecord(event.data) ? event.data : {};
  if (event.type === 'alert.notable') return `${details.ruleId || 'notable alert'} · ${details.title || event.subject.id}`;
  if (event.type === 'finding.promoted') return `Finding promoted · ${details.title || event.subject.id}`;
  if (event.type === 'entity.merged') return 'Entity merged';
  if (event.type === 'entity.split') return 'Entity split';
  if (event.type === 'capture.accepted') return `${details.phase || event.subject.type} · ${details.parseStatus || 'captured'}`;
  if (event.type === 'actor.provisioned') return `Actor provisioned · ${details.actorId || event.subject.id}`;
  if (event.type === 'actor.credential-rotated') return `Credential rotated · v${details.credentialVersion || event.subject.revision || '?'}`;
  if (event.type === 'actor.revoked') return `Actor revoked · ${details.actorId || event.subject.id}`;
  if (event.type === 'out-of-band.flagged') return `Claim flagged · ${details.claimKind || event.subject.id}`;
  if (event.type === 'out-of-band.resolved') return `Claim resolved · ${details.claimKind || event.subject.id}`;
  return jsonText(details).slice(0, 120);
}

function buildSseAuditEvent(event, payload) {
  return {
    contractVersion: apiVersion,
    id: event.id || newRequestId(),
    type: event.event,
    engagementId: state.engagementId,
    actor: isRecord(payload.actor) ? payload.actor : { id: '', kind: 'human', handle: '', role: '' },
    occurredAt: typeof payload.occurredAt === 'string' ? payload.occurredAt : new Date().toISOString(),
    origin: isRecord(payload.origin) ? payload.origin : { kind: 'rest' },
    subject: isRecord(payload.subject) ? payload.subject : { type: 'event', id: '' },
    requestId: typeof payload.requestId === 'string' ? payload.requestId : '',
    correlationId: typeof payload.correlationId === 'string' ? payload.correlationId : '',
    data: payload.data ?? payload,
  };
}

function setTheme(theme) {
  state.theme = theme;
  document.documentElement.dataset.theme = theme;
  try { window.localStorage.setItem('waypoint-theme', theme); } catch { /* ignore */ }
  render();
}

function setToken(token) {
  state.token = token;
  try { window.localStorage.setItem('waypoint-token', token); } catch { /* ignore */ }
  scheduleRefresh();
}

function navigateToPhase(phase) {
  state.view = 'trail';
  state.activePhase = phase;
  pushPath(phasePath(state.engagementId, phase));
  render();
}

function navigateToReport() {
  state.view = 'report';
  pushPath(reportPath(state.engagementId));
  render();
}

function navigateToMap() {
  state.view = 'map';
  pushPath(mapPath(state.engagementId));
  render();
}

function navigateToDevices() {
  state.view = 'devices';
  pushPath(devicesPath(state.engagementId));
  render();
}

function navigateToBoard() {
  state.view = 'board';
  pushPath(boardPath(state.engagementId));
  render();
}

function notePhaseAfterActive() {
  const index = waypointOrder.indexOf(state.activePhase);
  return waypointOrder[Math.min(index + 1, waypointOrder.length - 1)];
}

function visibleGuideNotes() {
  const query = state.guideQuery.trim().toLowerCase();
  return guideNotes.filter((note) => {
    const haystack = [note.phase, note.title, note.what, note.when, note.risks].join(' ').toLowerCase();
    if (!query) return note.phase === state.activePhase;
    return haystack.includes(query);
  });
}

function filteredActions() {
  return state.actions.filter((action) => state.actionPhaseFilter === 'all' || action.capture.phase === state.actionPhaseFilter);
}

function filteredEntities() {
  const q = state.entityQuery.trim().toLowerCase();
  return state.entities.filter((entity) => {
    if (!q) return true;
    return [entity.kind, (entity.identifiers || []).map((identifier) => `${identifier.type} ${identifier.value}`).join(' '), jsonText(entity.attributes)]
      .join(' ')
      .toLowerCase()
      .includes(q);
  });
}

function selectedAction() {
  const list = filteredActions();
  return list.find((action) => action.id === state.selectedActionId) || list[0] || null;
}

function selectedEntity() {
  const list = filteredEntities();
  return list.find((entity) => entity.id === state.selectedEntityId) || list[0] || null;
}

function selectedFinding() {
  return state.findings.find((finding) => finding.id === state.selectedFindingId) || state.findings[0] || null;
}

function selectedActor() {
  return state.actors.find((actor) => actor.actor.id === state.selectedActorId) || state.actors[0] || null;
}

function selectedClaim() {
  return state.claims.find((claim) => claim.id === state.selectedClaimId) || state.claims[0] || null;
}

function selectedExportJob() {
  return state.exportJobs.find((job) => job.id === state.selectedExportJobId) || state.exportJobs[0] || null;
}

function selectedExportReceipt(job) {
  if (!job || !job.bundle || !job.bundle.receiptId) return null;
  return state.selectedExportReceipt && state.selectedExportReceipt.exportJobId === job.id ? state.selectedExportReceipt : null;
}

function selectedTeardownAuthorization(job) {
  return state.selectedTeardownAuthorization && state.selectedTeardownAuthorization.exportJobId === job?.id ? state.selectedTeardownAuthorization : null;
}

function activeHumanActors() {
  return state.actors.filter((actor) => actor.status === 'active' && actor.actor.kind === 'human');
}

function filteredActors() {
  const query = state.actorQuery.trim().toLowerCase();
  return state.actors.filter((actor) => {
    if (!query) return true;
    return [actor.actor.handle, actor.actor.kind, actor.actor.role, actor.status, actor.actor.agentName || '', actor.actor.model || '', actor.actor.version || '', actor.actor.authorizedBy || '', actor.createdBy, String(actor.revision)]
      .join(' ')
      .toLowerCase()
      .includes(query);
  });
}

function filteredClaims() {
  return state.claims;
}

function notableAlerts() {
  return state.auditEvents.filter((event) => event.type === 'alert.notable').slice(0, 3);
}

function currentWaypointLabel() {
  return phaseNames[state.activePhase] || 'Trail';
}

function selectedEvidenceRef(action, evidenceId) {
  if (!action) return null;
  return [action.evidenceReferences.stdout, action.evidenceReferences.stderr].find((item) => item.id === evidenceId) || null;
}

function syncProvisionAuthorizer() {
  if (state.provisionDraft.kind !== 'ai_agent') return;
  const humans = activeHumanActors();
  if (!humans.length) return;
  if (humans.some((actor) => actor.actor.id === state.provisionDraft.authorizedBy)) return;
  state.provisionDraft.authorizedBy = humans[0].actor.id;
}

async function issueActorCredential() {
  state.actorConflict = '';
  try {
    const body = {
      kind: state.provisionDraft.kind,
      handle: state.provisionDraft.handle,
      role: state.provisionDraft.role,
      agentName: state.provisionDraft.kind === 'ai_agent' ? state.provisionDraft.agentName : undefined,
      model: state.provisionDraft.kind === 'ai_agent' ? state.provisionDraft.model : undefined,
      version: state.provisionDraft.kind === 'ai_agent' ? state.provisionDraft.version : undefined,
      authorizedBy: state.provisionDraft.kind === 'ai_agent' ? state.provisionDraft.authorizedBy || activeHumanActors()[0]?.actor.id || '' : undefined,
    };
    const response = await fetch('/api/v1/actors', {
      method: 'POST',
      headers: { ...authHeaders(state.token, newRequestId()), 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
    });
    if (!response.ok) throw new Error(await readProblem(response));
    const created = await response.json();
    state.actorCredential = created;
    state.selectedActorId = created.actorRecord.actor.id;
    state.provisionDraft.handle = '';
    state.provisionDraft.agentName = '';
    state.provisionDraft.model = '';
    state.provisionDraft.version = '';
    state.provisionDraft.authorizedBy = '';
    await refreshActors();
    state.actorConflict = `Issued credential v${created.actorRecord.credentialVersion} for ${created.actorRecord.actor.handle}.`;
  } catch (error) {
    state.actorConflict = error instanceof Error ? error.message : 'Unable to issue credential';
  }
  render();
}

async function rotateActorCredential(actor) {
  state.actorConflict = '';
  try {
    const response = await fetch(`/api/v1/actors/${actor.actor.id}/rotate`, {
      method: 'POST',
      headers: authHeaders(state.token, newRequestId()),
    });
    if (!response.ok) throw new Error(await readProblem(response));
    const rotated = await response.json();
    state.actorCredential = rotated;
    state.selectedActorId = rotated.actorRecord.actor.id;
    await refreshActors();
    state.actorConflict = `Rotated ${rotated.actorRecord.actor.handle} to credential v${rotated.actorRecord.credentialVersion}.`;
  } catch (error) {
    state.actorConflict = error instanceof Error ? error.message : 'Unable to rotate credential';
  }
  render();
}

async function revokeActorCredential(actor) {
  state.actorConflict = '';
  try {
    const response = await fetch(`/api/v1/actors/${actor.actor.id}/revoke`, {
      method: 'POST',
      headers: authHeaders(state.token, newRequestId()),
    });
    if (!response.ok) throw new Error(await readProblem(response));
    const revoked = await response.json();
    state.selectedActorId = revoked.actor.id;
    await refreshActors();
    state.actorConflict = `Revoked ${revoked.actor.handle} at revision ${revoked.revision}.`;
  } catch (error) {
    state.actorConflict = error instanceof Error ? error.message : 'Unable to revoke credential';
  }
  render();
}

async function resolveClaim() {
  const claim = selectedClaim();
  if (!claim) return;
  state.claimConflict = '';
  try {
    const sourceActionId = state.claimResolutionDraft.resolution === 'linked' ? (state.claimResolutionDraft.sourceActionId || selectedAction()?.id || '') : undefined;
    if (state.claimResolutionDraft.resolution === 'linked' && !sourceActionId) {
      throw new Error("Can't link this claim yet — sourceActionId is required when resolution is linked.");
    }
    const response = await fetch(`/api/v1/out-of-band-claims/${claim.id}/resolve`, {
      method: 'POST',
      headers: { ...authHeaders(state.token, newRequestId()), 'Content-Type': 'application/json' },
      body: JSON.stringify({
        resolution: state.claimResolutionDraft.resolution,
        sourceActionId,
        notes: state.claimResolutionDraft.notes || undefined,
        expectedRevision: claim.revision,
      }),
    });
    if (!response.ok) throw new Error(await readProblem(response));
    const updated = await response.json();
    state.claims = state.claims.map((item) => (item.id === updated.id ? updated : item));
    state.selectedClaimId = updated.id;
    state.claimConflict = `Claim ${updated.id} resolved as ${updated.status}.`;
    await refreshClaims();
  } catch (error) {
    state.claimConflict = error instanceof Error ? error.message : 'Unable to resolve claim';
  }
  render();
}

function scheduleRender() {
  if (state.renderQueued) return;
  state.renderQueued = true;
  queueMicrotask(() => {
    state.renderQueued = false;
    render();
  });
}

function scheduleRefresh() {
  void refreshEverything();
}

function setSelectedActionId(id) {
  state.selectedActionId = id;
  state.selectedEvidenceId = '';
  state.selectedEvidence = null;
  state.selectedEvidenceContent = '';
  state.selectedEvidenceError = '';
  const action = selectedAction();
  if (action && !state.findingPromotionDraft.title) {
    state.findingPromotionDraft = {
      title: `${action.capture.command} on ${action.capture.target.value}`,
      severity: 'medium',
      remediation: '',
      status: 'open',
    };
  }
  render();
}

function setSelectedEntityId(id) {
  state.selectedEntityId = id;
  render();
}

function setSelectedFindingId(id) {
  state.selectedFindingId = id;
  render();
}

function setSelectedExportJobId(id) {
  state.selectedExportJobId = id;
  state.selectedExportJobError = '';
  const job = selectedExportJob();
  if (job && job.bundle && job.bundle.receiptId) {
    void refreshExportReceipt(job);
  } else {
    state.selectedExportReceipt = null;
    state.selectedTeardownAuthorization = null;
  }
  render();
}

function setSelectedObservationId(id) {
  state.selectedObservationId = id;
  render();
}

function setActionFilter(filter) {
  state.actionPhaseFilter = filter;
  render();
}

function updateGuideQuery(value) {
  state.guideQuery = value;
  render();
}

function updateEntityQuery(value) {
  state.entityQuery = value;
  render();
}

function updateFindingPromotionDraft(form) {
  const data = new FormData(form);
  state.findingPromotionDraft = {
    title: String(data.get('title') || ''),
    severity: String(data.get('severity') || 'medium'),
    remediation: String(data.get('remediation') || ''),
    status: String(data.get('status') || 'open'),
  };
}

function updateFindingDraft(form) {
  const data = new FormData(form);
  state.findingDraft = {
    title: String(data.get('title') || ''),
    severity: String(data.get('severity') || 'medium'),
    remediation: String(data.get('remediation') || ''),
    status: String(data.get('status') || 'open'),
  };
}

async function refreshAudit(signal) {
  state.auditStatus = 'loading';
  state.auditError = '';
  render();
  try {
    const page = await apiJson('/api/v1/audit-events?limit=30', state.token, signal);
    const items = Array.isArray(page.items) ? [...page.items].reverse() : [];
    state.auditEvents = items;
    state.streamCursor = page.page?.highWaterCursor || items[0]?.id || null;
    state.auditStatus = 'ready';
  } catch (error) {
    state.auditStatus = 'error';
    state.auditError = error instanceof Error ? error.message : 'Unable to load journey log';
  }
  render();
}

async function refreshActions(signal) {
  state.actionsStatus = 'loading';
  state.actionsError = '';
  render();
  try {
    const page = await apiJson('/api/v1/actions?limit=100', state.token, signal);
    state.actions = Array.isArray(page.items) ? page.items : [];
    if (!state.selectedActionId && state.actions.length) {
      state.selectedActionId = state.actions.find((action) => action.capture.phase === 'attacks')?.id || state.actions[0].id;
    }
    state.actionsStatus = 'ready';
  } catch (error) {
    state.actionsStatus = 'error';
    state.actionsError = error instanceof Error ? error.message : 'Unable to load attacks';
  }
  render();
}

async function refreshEntities(signal) {
  state.entitiesStatus = 'loading';
  state.entityConflict = '';
  render();
  try {
    const page = await apiJson('/api/v1/entities?limit=100', state.token, signal);
    state.entities = Array.isArray(page.items) ? page.items : [];
    if (!state.selectedEntityId && state.entities.length) state.selectedEntityId = state.entities[0].id;
    state.entitiesStatus = 'ready';
  } catch (error) {
    state.entitiesStatus = 'error';
    state.entityConflict = error instanceof Error ? error.message : 'Unable to load recon';
  }
  render();
}

async function refreshFindings(signal) {
  state.findingsStatus = 'loading';
  state.findingConflict = '';
  render();
  try {
    const page = await apiJson('/api/v1/findings', state.token, signal);
    state.findings = Array.isArray(page.items) ? page.items : [];
    if (!state.selectedFindingId && state.findings.length) state.selectedFindingId = state.findings[0].id;
    state.findingsStatus = 'ready';
  } catch (error) {
    state.findingsStatus = 'error';
    state.findingConflict = error instanceof Error ? error.message : 'Unable to load findings';
  }
  render();
}

async function refreshActors(signal) {
  state.actorsStatus = 'loading';
  state.actorsError = '';
  render();
  try {
    const page = await apiJson('/api/v1/actors?limit=100', state.token, signal);
    state.actors = Array.isArray(page.items) ? page.items : [];
    if (!state.selectedActorId && state.actors.length) state.selectedActorId = state.actors[0].actor.id;
    const humans = activeHumanActors();
    if (state.provisionDraft.kind === 'ai_agent' && humans.length && !humans.some((actor) => actor.actor.id === state.provisionDraft.authorizedBy)) {
      state.provisionDraft.authorizedBy = humans[0].actor.id;
    }
    state.actorsStatus = 'ready';
  } catch (error) {
    state.actorsStatus = 'error';
    state.actorsError = error instanceof Error ? error.message : 'Unable to load actors';
  }
  render();
}

async function refreshClaims(signal) {
  state.claimsStatus = 'loading';
  state.claimsError = '';
  render();
  try {
    const page = await apiJson('/api/v1/out-of-band-claims?limit=100', state.token, signal);
    state.claims = Array.isArray(page.items) ? page.items : [];
    if (!state.selectedClaimId && state.claims.length) state.selectedClaimId = state.claims[0].id;
    state.claimsStatus = 'ready';
  } catch (error) {
    state.claimsStatus = 'error';
    state.claimsError = error instanceof Error ? error.message : 'Unable to load pending claims';
  }
  render();
}

async function refreshReport(signal) {
  state.reportStatus = 'loading';
  state.reportError = '';
  render();
  try {
    const raw = await apiText(reportJsonPath(state.engagementId), state.token, signal);
    const snapshot = JSON.parse(raw);
    state.reportRaw = raw;
    state.reportSnapshot = snapshot;
    state.reportStatus = 'ready';
  } catch (error) {
    state.reportStatus = 'error';
    state.reportError = error instanceof Error ? error.message : 'Unable to load report snapshot';
  }
  render();
}

async function refreshExportJobs(signal) {
  state.exportJobsStatus = 'loading';
  state.exportJobsError = '';
  render();
  try {
    const page = await apiJson(`${exportsPath()}?limit=25`, state.token, signal);
    state.exportJobs = Array.isArray(page.items) ? page.items : [];
    if (!state.selectedExportJobId && state.exportJobs.length) {
      state.selectedExportJobId = state.exportJobs[0].id;
    }
    state.exportJobsStatus = 'ready';
    state.selectedExportJobError = '';
    const job = selectedExportJob();
    if (job && job.bundle && job.bundle.receiptId) {
      await refreshExportReceipt(job, signal);
    } else {
      state.selectedExportReceipt = null;
      state.selectedTeardownAuthorization = null;
    }
  } catch (error) {
    state.exportJobsStatus = 'error';
    state.exportJobsError = error instanceof Error ? error.message : 'Unable to load export jobs';
  }
  render();
}

async function refreshExportReceipt(job, signal) {
  if (!job || !job.bundle || !job.bundle.receiptId) {
    state.selectedExportReceipt = null;
    return;
  }
  try {
    const receipt = await apiJson(exportReceiptPath(job.bundle.receiptId), state.token, signal);
    state.selectedExportReceipt = receipt;
    state.selectedExportJobError = '';
  } catch (error) {
    state.selectedExportReceipt = null;
    state.selectedExportJobError = error instanceof Error ? error.message : 'Unable to load verified receipt';
  }
  render();
}

async function refreshTeardownAuthorization(authorizationId, signal) {
  try {
    const auth = await apiJson(teardownAuthorizationPath(authorizationId), state.token, signal);
    state.selectedTeardownAuthorization = auth;
    state.summitActionError = '';
  } catch (error) {
    state.selectedTeardownAuthorization = null;
    state.summitActionError = error instanceof Error ? error.message : 'Unable to load teardown authorization';
  }
  render();
}

function syncExportPolling() {
  const shouldPoll = state.view === 'trail' && state.activePhase === 'summit' && Boolean(state.token);
  if (shouldPoll && !state.exportPollTimer) {
    const controller = new AbortController();
    state.exportPollAbort = controller;
    const tick = () => {
      if (!controller.signal.aborted) void refreshExportJobs(controller.signal);
    };
    // Assign the timer BEFORE the first tick: tick() runs refreshExportJobs,
    // which calls render(), which calls syncExportPolling() again — if the timer
    // weren't set yet the guard above would re-enter and recurse until the stack
    // overflows (and flood the exports endpoint). Setting it first makes the
    // re-entrant call a no-op.
    state.exportPollTimer = window.setInterval(tick, 3500);
    tick();
    return;
  }
  if (!shouldPoll && state.exportPollTimer) {
    window.clearInterval(state.exportPollTimer);
    state.exportPollTimer = null;
    state.exportPollAbort?.abort();
    state.exportPollAbort = null;
  }
}

async function refreshEverything() {
  const controller = new AbortController();
  await Promise.allSettled([
    refreshAudit(controller.signal),
    refreshActions(controller.signal),
    refreshEntities(controller.signal),
    refreshFindings(controller.signal),
    refreshActors(controller.signal),
    refreshClaims(controller.signal),
    refreshExportJobs(controller.signal),
  ]);
  controller.abort();
  if (state.view === 'report') {
    void refreshReport();
  }
  syncExportPolling();
  startStream();
}

function startStream() {
  state.sseAbort?.abort();
  const controller = new AbortController();
  state.sseAbort = controller;
  let cursor = state.streamCursor;
  let backoff = 700;

  void (async () => {
    while (!controller.signal.aborted) {
      try {
        const streamUrl = new URL('/events', window.location.origin);
        if (cursor) streamUrl.searchParams.set('after', cursor);
        const response = await fetch(streamUrl, {
          headers: { Authorization: `Bearer ${state.token}`, 'Waypoint-Contract-Version': apiVersion, Accept: 'text/event-stream' },
          signal: controller.signal,
          cache: 'no-store',
        });
        if (!response.ok) throw new Error(await readProblem(response));
        state.auditStatus = 'ready';
        state.auditError = '';
        render();
        await parseSseStream(response, controller.signal, (event) => {
          if (event.id) cursor = event.id;
          if (!event.data) return;
          let parsed = event.data;
          try { parsed = JSON.parse(event.data); } catch { /* ignore */ }
          const auditEvent = buildSseAuditEvent(event, parsed);
          state.auditEvents = [auditEvent, ...state.auditEvents.filter((item) => item.id !== auditEvent.id)].slice(0, 30);
          if (event.event === 'capture.accepted' || event.event === 'alert.notable') {
            void refreshActions();
            void refreshEntities();
            void refreshFindings();
            void refreshClaims();
          } else if (event.event.startsWith('entity.')) {
            void refreshEntities();
          } else if (event.event.startsWith('finding.')) {
            void refreshFindings();
          } else if (event.event.startsWith('actor.')) {
            void refreshActors();
          } else if (event.event.startsWith('export.')) {
            void refreshExportJobs();
          } else if (event.event.startsWith('out-of-band.')) {
            void refreshClaims();
          }
          render();
        });
        backoff = 700;
      } catch (error) {
        if (controller.signal.aborted) return;
        state.auditStatus = 'error';
        state.auditError = error instanceof Error ? error.message : 'Journey log stream disconnected';
        render();
        await new Promise((resolve) => window.setTimeout(resolve, backoff));
        backoff = Math.min(backoff * 2, 6000);
      }
    }
  })();
}

async function loadEvidence(evidenceId) {
  const action = selectedAction();
  const ref = selectedEvidenceRef(action, evidenceId);
  if (!ref) return;
  state.selectedEvidenceId = evidenceId;
  state.selectedEvidence = null;
  state.selectedEvidenceContent = '';
  state.selectedEvidenceError = '';
  state.evidenceAbort?.abort();
  const controller = new AbortController();
  state.evidenceAbort = controller;
  render();
  try {
    const evidence = await apiJson(ref.downloadPath.replace('/content', ''), state.token, controller.signal);
    state.selectedEvidence = evidence;
    if (evidence.mediaType.startsWith('text/') || evidence.mediaType.includes('json') || evidence.mediaType.includes('xml')) {
      state.selectedEvidenceContent = await apiText(evidence.contentPath, state.token, controller.signal);
    }
    render();
  } catch (error) {
    if (!controller.signal.aborted) {
      state.selectedEvidenceError = error instanceof Error ? error.message : 'Unable to load evidence';
      render();
    }
  }
}

async function runMergePreview() {
  if (!state.mergeSourceId || !state.mergeTargetId || state.mergeSourceId === state.mergeTargetId) return;
  state.entityConflict = '';
  render();
  try {
    const response = await fetch(`/api/v1/entities/${state.mergeSourceId}/merge-preview?targetEntityId=${state.mergeTargetId}`, { headers: authHeaders(state.token, newRequestId()), cache: 'no-store' });
    if (!response.ok) throw new Error(await readProblem(response));
    const preview = await response.json();
    state.entityConflict = `Preview: ${preview.source.id} → ${preview.target?.id || state.mergeTargetId}`;
  } catch (error) {
    state.entityConflict = error instanceof Error ? error.message : 'Merge preview failed';
  }
  render();
}

async function applyMerge() {
  if (!state.mergeSourceId || !state.mergeTargetId || state.mergeSourceId === state.mergeTargetId) return;
  const source = state.entities.find((entity) => entity.id === state.mergeSourceId);
  const target = state.entities.find((entity) => entity.id === state.mergeTargetId);
  try {
    const response = await fetch('/api/v1/entities/merge', {
      method: 'POST',
      headers: { ...authHeaders(state.token, newRequestId()), 'Content-Type': 'application/json' },
      body: JSON.stringify({
        sourceEntityId: state.mergeSourceId,
        targetEntityId: state.mergeTargetId,
        preview: false,
        expectedSourceRevision: source?.revision,
        expectedTargetRevision: target?.revision,
      }),
    });
    if (!response.ok) throw new Error(await readProblem(response));
    const applied = await response.json();
    state.entityConflict = `Merged ${applied.source.id} into ${applied.target?.id || state.mergeTargetId}.`;
    await refreshEntities();
  } catch (error) {
    state.entityConflict = error instanceof Error ? error.message : 'Merge failed';
    render();
  }
}

async function runSplitPreview() {
  if (!state.selectedEntityId || !state.selectedObservationId) return;
  state.entityConflict = '';
  render();
  try {
    const response = await fetch(`/api/v1/entities/${state.selectedEntityId}/split-provenance?observationId=${state.selectedObservationId}`, { headers: authHeaders(state.token, newRequestId()), cache: 'no-store' });
    if (!response.ok) throw new Error(await readProblem(response));
    const preview = await response.json();
    state.entityConflict = `Preview: split observation ${preview.observationId || state.selectedObservationId} from ${preview.source.id}.`;
  } catch (error) {
    state.entityConflict = error instanceof Error ? error.message : 'Split preview failed';
  }
  render();
}

async function applySplit() {
  if (!state.selectedEntityId || !state.selectedObservationId) return;
  try {
    const response = await fetch('/api/v1/entities/split', {
      method: 'POST',
      headers: { ...authHeaders(state.token, newRequestId()), 'Content-Type': 'application/json' },
      body: JSON.stringify({
        entityId: state.selectedEntityId,
        preview: false,
        observationId: state.selectedObservationId,
        expectedSourceRevision: state.entities.find((entity) => entity.id === state.selectedEntityId)?.revision,
      }),
    });
    if (!response.ok) throw new Error(await readProblem(response));
    const applied = await response.json();
    state.entityConflict = `Split observation ${applied.observationId || state.selectedObservationId} from ${applied.source.id}.`;
    await refreshEntities();
  } catch (error) {
    state.entityConflict = error instanceof Error ? error.message : 'Split failed';
    render();
  }
}

async function promoteSelectedAttack(form) {
  const action = selectedAction();
  if (!action) return;
  const data = new FormData(form);
  const body = {
    sourceActionId: action.id,
    title: String(data.get('title') || `${action.capture.command} on ${action.capture.target.value}`),
    severity: String(data.get('severity') || 'medium'),
    remediation: String(data.get('remediation') || 'Review the underlying control and record the operator decision.'),
    status: String(data.get('status') || 'open'),
  };
  state.promotionConflict = '';
  render();
  try {
    const response = await fetch('/api/v1/findings', {
      method: 'POST',
      headers: { ...authHeaders(state.token, newRequestId()), 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
    });
    if (!response.ok) throw new Error(await readProblem(response));
    const created = await response.json();
    state.findings = [created, ...state.findings.filter((item) => item.id !== created.id)];
    state.selectedFindingId = created.id;
    state.activePhase = 'findings';
    state.view = 'trail';
    pushPath(phasePath(state.engagementId, 'findings'));
    state.promotionConflict = 'Promoted into Findings and linked to evidence.';
    render();
  } catch (error) {
    state.promotionConflict = error instanceof Error ? error.message : 'Promotion failed';
    render();
  }
}

async function saveFinding(form) {
  const finding = selectedFinding();
  if (!finding) return;
  const data = new FormData(form);
  const body = {
    expectedRevision: finding.revision,
    title: String(data.get('title') || finding.title),
    severity: String(data.get('severity') || finding.severity),
    remediation: String(data.get('remediation') || finding.remediation),
    status: String(data.get('status') || finding.status),
    affectedEntityIds: state.selectedEntityId ? [state.selectedEntityId] : finding.affectedEntityIds,
  };
  state.findingConflict = '';
  render();
  try {
    const response = await fetch(`/api/v1/findings/${finding.id}`, {
      method: 'PATCH',
      headers: { ...authHeaders(state.token, newRequestId()), 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
    });
    if (!response.ok) throw new Error(await readProblem(response));
    const updated = await response.json();
    state.findings = state.findings.map((item) => (item.id === updated.id ? updated : item));
    state.selectedFindingId = updated.id;
    state.findingConflict = 'Finding saved with the authoritative revision.';
    render();
  } catch (error) {
    state.findingConflict = error instanceof Error ? error.message : 'Unable to save finding';
    render();
  }
}

async function startSummitExport() {
  state.exportAbort?.abort();
  const controller = new AbortController();
  state.exportAbort = controller;
  state.summitActionError = '';
  state.selectedExportJobError = '';
  state.summitRequestNote = 'Submitting a persisted export job to the server.';
  render();
  try {
    const created = await apiJsonPost(exportsPath(), state.token, { formatVersion: apiVersion }, controller.signal);
    state.selectedExportJobId = created.id;
    state.summitRequestNote = `Export job ${created.id} queued by ${created.requestedBy.handle}.`;
    await refreshExportJobs(controller.signal);
  } catch (error) {
    if (controller.signal.aborted) {
      state.summitActionError = 'Export request canceled before the server accepted it.';
      state.summitRequestNote = 'The live trail stayed intact.';
    } else {
      state.summitActionError = error instanceof Error ? error.message : 'Unable to start export job';
      state.summitRequestNote = 'Export request failed.';
    }
    render();
  } finally {
    state.exportAbort = null;
  }
}

async function cancelSelectedExport() {
  const job = selectedExportJob();
  if (!job) return;
  state.summitActionError = '';
  render();
  try {
    const response = await fetch(exportCancelPath(job.id), {
      method: 'POST',
      headers: { ...authHeaders(state.token, newRequestId()), 'If-Match': revisionHeader(job.revision) },
      cache: 'no-store',
    });
    if (!response.ok) throw new Error(await readProblem(response));
    const updated = await response.json();
    state.exportJobs = state.exportJobs.map((item) => (item.id === updated.id ? updated : item));
    state.selectedExportJobId = updated.id;
    state.summitRequestNote = `Cancellation requested for ${updated.id}.`;
    await refreshExportJobs();
  } catch (error) {
    state.summitActionError = error instanceof Error ? error.message : 'Unable to cancel export job';
    render();
  }
}

async function requestTeardownAuthorization() {
  const job = selectedExportJob();
  const receipt = selectedExportReceipt(job);
  if (!job || !receipt) return;
  state.summitActionError = '';
  render();
  try {
    const auth = await apiJsonPost(teardownAuthorizationsPath(), state.token, {
      receiptId: receipt.id,
      bundlePath: receipt.bundlePath,
      archiveSha256: receipt.archiveSha256,
      manifestSha256: receipt.manifestSha256,
      confirmation: 'destroy verified engagement data',
    });
    state.selectedTeardownAuthorization = auth;
    state.summitRequestNote = `Teardown authorization ${auth.id} is ${auth.status}.`;
    render();
  } catch (error) {
    state.summitActionError = error instanceof Error ? error.message : 'Unable to authorize teardown';
    render();
  }
}

async function consumeTeardownAuthorization() {
  const auth = selectedTeardownAuthorization(selectedExportJob());
  if (!auth) return;
  state.summitActionError = '';
  render();
  try {
    const consumed = await apiJsonPost(teardownAuthorizationConsumePath(auth.id), state.token, undefined);
    state.selectedTeardownAuthorization = consumed;
    state.summitRequestNote = `Teardown authorization ${consumed.id} consumed by the server.`;
    render();
  } catch (error) {
    state.summitActionError = error instanceof Error ? error.message : 'Unable to consume teardown authorization';
    render();
  }
}

async function openVerifiedArtifact(kind) {
  const job = selectedExportJob();
  if (!job || !job.bundle) return;
  state.summitActionError = '';
  render();
  try {
    const path = kind === 'pdf' ? exportReportPdfPath(job.id) : exportBundlePath(job.id);
    const response = await fetch(path, { headers: authHeaders(state.token, newRequestId()), cache: 'no-store' });
    if (!response.ok) throw new Error(await readProblem(response));
    const blob = await response.blob();
    const url = URL.createObjectURL(blob);
    if (kind === 'pdf') {
      const previewWindow = window.open('', '_blank', 'noopener');
      if (!previewWindow) throw new Error('Unable to open the verified PDF preview');
      previewWindow.location.href = url;
      previewWindow.addEventListener('load', () => URL.revokeObjectURL(url), { once: true });
    } else {
      const anchor = document.createElement('a');
      anchor.href = url;
      anchor.download = job.bundle.archivePath;
      anchor.click();
      window.setTimeout(() => URL.revokeObjectURL(url), 1000);
    }
  } catch (error) {
    state.summitActionError = error instanceof Error ? error.message : 'Unable to open verified artifact';
    render();
  }
}

function renderBadge(status) {
  return `<span class="status-pill ${status}">${escapeHtml(status)}</span>`;
}

function renderOperationsShell() {
  const actors = filteredActors();
  const actor = selectedActor();
  const claims = filteredClaims();
  const claim = selectedClaim();
  const humans = activeHumanActors();
  return `
    <section class="operations-shell" aria-label="Provisioning and claim review workspace">
      <div class="panel-heading">
        <div>
          <p class="guide-note-kicker">Operations</p>
          <h2>Provisioning and review</h2>
        </div>
        <p>One-time secrets are shown once, AI actors must cite an active human authorizer, and pending gaps stay visible until they are linked or dismissed.</p>
      </div>

      <div class="operations-grid">
        <article class="operations-card">
          <div class="detail-head">
            <div>
              <p class="guide-note-kicker">Actor provisioning</p>
              <h3>Issue a one-time credential</h3>
            </div>
            ${renderBadge('review')}
          </div>
          <form class="ops-form-grid" data-action="actor-provision-form">
            <label class="finding-field"><span>Kind</span><select value="${escapeHtml(state.provisionDraft.kind)}" data-action="provision-draft" data-field="kind"><option value="human" ${state.provisionDraft.kind === 'human' ? 'selected' : ''}>Human</option><option value="ai_agent" ${state.provisionDraft.kind === 'ai_agent' ? 'selected' : ''}>AI agent</option></select></label>
            <label class="finding-field"><span>Handle</span><input value="${escapeHtml(state.provisionDraft.handle)}" data-action="provision-draft" data-field="handle" placeholder="alex.operator" /></label>
            <label class="finding-field"><span>Role</span><select value="${escapeHtml(state.provisionDraft.role)}" data-action="provision-draft" data-field="role"><option value="owner" ${state.provisionDraft.role === 'owner' ? 'selected' : ''}>Owner</option><option value="operator" ${state.provisionDraft.role === 'operator' ? 'selected' : ''}>Operator</option><option value="viewer" ${state.provisionDraft.role === 'viewer' ? 'selected' : ''}>Viewer</option></select></label>
            ${state.provisionDraft.kind === 'ai_agent' ? `
              <label class="finding-field"><span>Agent name</span><input value="${escapeHtml(state.provisionDraft.agentName)}" data-action="provision-draft" data-field="agentName" placeholder="Synthetic Field Agent" /></label>
              <label class="finding-field"><span>Model</span><input value="${escapeHtml(state.provisionDraft.model)}" data-action="provision-draft" data-field="model" placeholder="gpt-4.1" /></label>
              <label class="finding-field"><span>Version</span><input value="${escapeHtml(state.provisionDraft.version)}" data-action="provision-draft" data-field="version" placeholder="2025.01" /></label>
              <label class="finding-field"><span>Authorized by</span>
                <select value="${escapeHtml(state.provisionDraft.authorizedBy)}" data-action="provision-draft" data-field="authorizedBy">
                  <option value="">Pick an active human authorizer</option>
                  ${humans.map((human) => `<option value="${human.actor.id}" ${state.provisionDraft.authorizedBy === human.actor.id ? 'selected' : ''}>${escapeHtml(human.actor.handle)} · rev ${human.revision}</option>`).join('')}
                </select>
              </label>
              <p class="guide-note-empty ops-inline-note">${humans.length ? 'Only active human operators can authorise AI actors.' : 'AI actors need an active human operator on the hook before a credential can be issued.'}</p>
            ` : ''}
            <div class="guide-tools" style="margin-top:12px">
              <button type="submit" class="primary-button" data-action="issue-credential" ${!state.provisionDraft.handle.trim() || (state.provisionDraft.kind === 'ai_agent' && !state.provisionDraft.authorizedBy) ? 'disabled' : ''}>Issue one-time credential</button>
              <button type="button" class="secondary-link" data-action="burn-credential" ${state.actorCredential ? '' : 'disabled'}>Burn copy</button>
            </div>
          </form>
          ${state.actorConflict ? `<div class="live-banner review"><strong>Credential note</strong> ${escapeHtml(state.actorConflict)}</div>` : ''}
          ${state.actorCredential ? `
            <div class="secret-token-card" aria-label="One-time credential response">
              <div class="panel-heading compact"><h4>One-time token</h4><p>${escapeHtml(state.actorCredential.actorRecord.actor.handle)} · v${state.actorCredential.actorRecord.credentialVersion} · issued ${escapeHtml(formatTime(state.actorCredential.issuedAt))}</p></div>
              <div class="secret-token" role="textbox" aria-readonly="true">${escapeHtml(state.actorCredential.token)}</div>
              <div class="guide-tools">
                <button type="button" class="secondary-link" data-action="copy-credential">Copy token</button>
                <span class="guide-note-empty">Do not paste this into the audit trail or logs.</span>
              </div>
            </div>
          ` : '<p class="guide-note-empty">The plaintext token only appears in this response once. It never returns from list or read endpoints.</p>'}
        </article>

        <article class="operations-card">
          <div class="detail-head">
            <div>
              <p class="guide-note-kicker">Actor roster</p>
              <h3>Rotate or revoke live credentials</h3>
            </div>
            ${renderBadge(state.actorsStatus === 'ready' ? 'review' : 'neutral')}
          </div>
          <label class="guide-search" style="margin-top:0">
            <span class="sr-only">Search actors</span>
            <input value="${escapeHtml(state.actorQuery)}" data-action="actor-query" placeholder="Search handle, role, revision…" aria-label="Search actors" />
          </label>
          ${state.actorsError ? `<div class="live-banner"><strong>Actor note</strong> ${escapeHtml(state.actorsError)}</div>` : ''}
          ${!actors.length && state.actorsStatus === 'ready' ? '<div class="guide-note-empty">No actors yet. Provision a credential above, then rotate or revoke it from the roster.</div>' : ''}
          <div class="actor-roster">
            ${actors.map((item) => `
              <button type="button" class="actor-row ${actor && actor.actor.id === item.actor.id ? 'is-selected' : ''}" data-action="select-actor" data-id="${item.actor.id}">
                <div class="actor-row-top">
                  <strong>${escapeHtml(item.actor.handle)}</strong>
                  ${renderBadge(item.status === 'active' ? 'success' : 'blocked')}
                </div>
                <div class="actor-row-main">
                  <span>${escapeHtml(item.actor.kind)}${item.actor.kind === 'ai_agent' ? ` · ${escapeHtml(item.actor.model || '')}` : ''}</span>
                  <span>role ${escapeHtml(item.actor.role)}</span>
                </div>
                <div class="actor-row-foot">
                  <span>cred v${item.credentialVersion}</span>
                  <span>rev ${item.revision}</span>
                </div>
              </button>`).join('')}
          </div>
          ${actor ? `
            <div class="secret-token-card" style="margin-top:12px">
              <div class="panel-heading compact"><h4>${escapeHtml(actor.actor.handle)}</h4><p>${escapeHtml(actor.actor.kind)} · ${escapeHtml(actor.actor.role)} · ${escapeHtml(actor.status)}</p></div>
              <dl class="ops-meta-grid">
                <div><dt>Created by</dt><dd>${escapeHtml(actor.createdBy)}</dd></div>
                <div><dt>Created at</dt><dd>${escapeHtml(formatTime(actor.createdAt))}</dd></div>
                <div><dt>Credential version</dt><dd>${actor.credentialVersion}</dd></div>
                <div><dt>Revision</dt><dd>${actor.revision}</dd></div>
                ${actor.actor.kind === 'ai_agent' ? `<div><dt>Authorized by</dt><dd>${escapeHtml(actor.actor.authorizedBy || '—')}</dd></div>` : ''}
                ${actor.lastRotatedAt ? `<div><dt>Last rotated</dt><dd>${escapeHtml(formatTime(actor.lastRotatedAt))}</dd></div>` : ''}
                ${actor.revokedAt ? `<div><dt>Revoked at</dt><dd>${escapeHtml(formatTime(actor.revokedAt))}</dd></div>` : ''}
              </dl>
              <div class="guide-tools">
                <button type="button" class="primary-button" data-action="rotate-credential" ${actor.status !== 'active' ? 'disabled' : ''}>Rotate credential</button>
                <button type="button" class="secondary-link" data-action="revoke-credential" ${actor.status !== 'active' ? 'disabled' : ''}>Revoke credential</button>
              </div>
            </div>
          ` : '<div class="guide-note-empty">Provision a credential above, then choose a record to rotate or revoke it.</div>'}
        </article>
      </div>

      <article class="operations-card operations-card-wide">
        <div class="detail-head">
          <div>
            <p class="guide-note-kicker">Claim review</p>
            <h3>Pending out-of-band claims</h3>
          </div>
          ${renderBadge(state.claimsStatus === 'ready' ? 'review' : 'neutral')}
        </div>
        <p class="workspace-lede">Best-effort claims stay visible until they are linked to a captured action or explicitly dismissed; resolved gaps remain in the trail for audit.</p>
        ${state.claimsError ? `<div class="live-banner"><strong>Claim note</strong> ${escapeHtml(state.claimsError)}</div>` : ''}
        ${!claims.length && state.claimsStatus === 'ready' ? '<div class="guide-note-empty">No pending claims. Best-effort claim review is clear for now.</div>' : ''}
        <div class="claim-review-layout">
          <div class="claim-review-list">
            ${claims.map((item) => `
              <button type="button" class="claim-row ${claim && claim.id === item.id ? 'is-selected' : ''}" data-action="select-claim" data-id="${item.id}">
                <div class="claim-row-top">
                  <strong>${escapeHtml(item.claimKind)} · ${escapeHtml(item.claimedSubjectId.slice(0, 8))}…</strong>
                  ${renderBadge(item.status === 'pending' ? 'review' : item.status === 'linked' ? 'success' : 'blocked')}
                </div>
                <div class="claim-row-main">
                  <span>${escapeHtml(item.reason)}</span>
                  <span>action ${escapeHtml(item.sourceActionId || 'not captured')}</span>
                </div>
                <div class="claim-row-foot">
                  <span>${escapeHtml(formatTime(item.observedAt))}</span>
                  <span>rev ${item.revision}</span>
                </div>
              </button>`).join('')}
          </div>
          <div class="claim-review-detail">
            ${claim ? `
              <div class="detail-head">
                <div>
                  <p class="guide-note-kicker">Selected claim</p>
                  <h4>${escapeHtml(claim.claimKind)} · ${escapeHtml(claim.id)}</h4>
                </div>
                ${renderBadge(claim.status === 'pending' ? 'review' : claim.status === 'linked' ? 'success' : 'blocked')}
              </div>
              <dl class="ops-meta-grid">
                <div><dt>Claimed subject</dt><dd>${escapeHtml(claim.claimedSubjectId)}</dd></div>
                <div><dt>Observed by</dt><dd>${escapeHtml(claim.observedBy.handle)}</dd></div>
                <div><dt>Observed at</dt><dd>${escapeHtml(formatTime(claim.observedAt))}</dd></div>
                <div><dt>Boundary</dt><dd>${escapeHtml(claim.detectionBoundary)}</dd></div>
                <div><dt>Source action</dt><dd>${escapeHtml(claim.sourceActionId || 'missing')}</dd></div>
                <div><dt>Revision</dt><dd>${claim.revision}</dd></div>
              </dl>
              ${claim.status === 'pending' ? `
                <form class="finding-editor-grid" data-action="claim-resolution-form" style="margin-top:12px">
                  <label class="finding-field"><span>Resolution</span><select value="${escapeHtml(state.claimResolutionDraft.resolution)}" data-action="claim-resolution-draft" data-field="resolution"><option value="linked" ${state.claimResolutionDraft.resolution === 'linked' ? 'selected' : ''}>Link to captured action</option><option value="dismissed" ${state.claimResolutionDraft.resolution === 'dismissed' ? 'selected' : ''}>Dismiss visibly</option></select></label>
                  <label class="finding-field"><span>Source action</span><input value="${escapeHtml(state.claimResolutionDraft.sourceActionId)}" data-action="claim-resolution-draft" data-field="sourceActionId" placeholder="${escapeHtml(selectedAction()?.id || 'Choose a captured action')}" ${state.claimResolutionDraft.resolution === 'dismissed' ? 'disabled' : ''} /></label>
                  <label class="finding-field finding-field-wide"><span>Notes</span><textarea data-action="claim-resolution-draft" data-field="notes" placeholder="Explain why the gap is linked or dismissed.">${escapeHtml(state.claimResolutionDraft.notes)}</textarea></label>
                  <div class="guide-tools">
                    <button type="button" class="secondary-link" data-action="use-selected-attack" ${selectedAction() ? '' : 'disabled'}>Use selected attack</button>
                    <button type="submit" class="primary-button">Resolve claim</button>
                  </div>
                </form>
              ` : '<p class="guide-note-empty">This claim is already resolved; the review view keeps the gap visible for audit purposes.</p>'}
              ${state.claimConflict ? `<div class="live-banner review"><strong>Review note</strong> ${escapeHtml(state.claimConflict)}</div>` : ''}
            ` : '<div class="guide-note-empty">Choose a pending gap to link it to a captured action or dismiss it plainly.</div>'}
          </div>
        </div>
      </article>
    </section>`;
}

/* ============================ Territory Map ============================
   A cartographic view of the estate: subnets become campsites, sized by host
   count and coloured by their worst finding severity, laid out so the most
   compromised segments climb toward the snowy summit (elevation = risk).
   Everything here is derived from real /entities + /findings data. */
const MPAL = { bark:'#4a2f1b', saddle:'#6b4423', trail:'#8b5e34', harvest:'#ba7517', lantern:'#ef9f27', wheat:'#fac775', fern:'#97c459', forest:'#639922', trees:'#6b8e4e', stone:'#b4a78c', stoned:'#8b7355', rust:'#b04c30', cocoa:'#854f0b' };
const MSEV_COLOR = { critical:'#b04c30', high:'#ba7517', medium:'#ef9f27', low:'#97c459', info:'#b4a78c' };
const MSEV_RANK = { critical:0, high:1, medium:2, low:3, info:4 };
const MSEV_LABEL = { critical:'Critical', high:'High', medium:'Medium', low:'Low', info:'Info' };
const MSEV_ORDER = ['critical','high','medium','low','info'];
const MFIRE = 'c -4 5 -5 8 -2 11 c -3 0 -5 3 -2 5 c 2 2 7 2 9 0 c 3 -2 1 -5 -2 -5 c 3 -3 1 -7 -3 -11';

function mSmoke(x, y, arr) { return `<g>${arr.map((p) => `<circle class="territory-puff" cx="${x}" cy="${y}" r="${p.r}" fill="rgba(180,167,140,1)" style="--dx:${p.dx}px;--o:${p.o};--d:${p.d}s;--dl:${p.dl}s"/>`).join('')}</g>`; }
function mFire(s = 1) { return `<g transform="scale(${s})">${mSmoke(1, -20, [{ r:2.6, dx:3, o:.42, d:3, dl:0 }, { r:3, dx:-2, o:.36, d:3.5, dl:.9 }, { r:2.3, dx:4, o:.3, d:3.2, dl:1.7 }])}<g class="territory-flame"><path d="M0 -8 C -8 -14 -7 -24 -1 -30 C -2 -22 4 -22 3 -30 C 9 -24 8 -13 0 -8 Z" fill="${MPAL.harvest}"/><path d="M0 -9 C -5 -13 -4 -21 0 -26 C 4 -21 5 -13 0 -9 Z" fill="${MPAL.lantern}"/><path d="M0 -10 C -2 -12 -2 -17 0 -20 C 2 -17 2 -12 0 -10 Z" fill="${MPAL.wheat}"/></g><path d="M-11 -1 L11 -5" stroke="${MPAL.saddle}" stroke-width="4" stroke-linecap="round"/><path d="M-11 -5 L11 -1" stroke="${MPAL.bark}" stroke-width="4" stroke-linecap="round"/></g>`; }
function mPine(s = 1) { return `<g transform="scale(${s})"><rect x="-1.8" y="-7" width="3.6" height="7" fill="${MPAL.saddle}"/><path d="M0 -30 L7 -20 L-7 -20 Z" fill="${MPAL.forest}"/><path d="M0 -25 L9 -13 L-9 -13 Z" fill="${MPAL.trees}"/><path d="M0 -19 L11 -6 L-11 -6 Z" fill="${MPAL.fern}"/></g>`; }
function mBadge(n, x, y, color) { return `<g transform="translate(${x},${y})"><rect x="-17" y="-11" width="34" height="18" rx="9" fill="var(--artifact-parchment)" stroke="${color}" stroke-width="1.7"/><text x="0" y="3.5" text-anchor="middle" font-family="'IBM Plex Mono',monospace" font-size="11" font-weight="700" fill="#3b2617">${n}</text></g>`; }
const M_CAMPS = {
  full: (n, color, s = 1) => `<g transform="scale(${s})"><ellipse cx="0" cy="2" rx="38" ry="7.5" fill="${MPAL.cocoa}" opacity=".18"/><g transform="translate(21,1) scale(.62)">${mPine(1)}</g><g transform="translate(-16,0)"><path d="M0 0 L-12 0 L-6 -18 Z" fill="${color}" stroke="${MPAL.bark}" stroke-width="1.3"/><path d="M-6 -18 L0 0 L3.5 0 L-2.5 -18 Z" fill="${MPAL.saddle}" stroke="${MPAL.bark}" stroke-width="1.3"/></g><g transform="translate(7,0) scale(.6)">${mFire(1)}</g>${mBadge(n, -3, 17, color)}</g>`,
  twin: (n, color, s = 1) => `<g transform="scale(${s})"><ellipse cx="0" cy="2" rx="38" ry="7.5" fill="${MPAL.cocoa}" opacity=".18"/><g transform="translate(-13,0) scale(.95)"><path d="M-9 0 L0 -19 L9 0 Z" fill="${color}" stroke="${MPAL.bark}" stroke-width="1.3" stroke-linejoin="round"/><path d="M-3 0 L0 -7 L3 0 Z" fill="${MPAL.saddle}" stroke="${MPAL.bark}" stroke-width="0.8"/></g><g transform="translate(15,1) scale(.8)"><path d="M-9 0 L0 -19 L9 0 Z" fill="${MPAL.saddle}" stroke="${MPAL.bark}" stroke-width="1.3" stroke-linejoin="round"/></g><g transform="translate(1,0) scale(.4)">${mFire(1)}</g>${mBadge(n, 0, 17, color)}</g>`,
  dome: (n, color, s = 1) => `<g transform="scale(${s})"><ellipse cx="0" cy="2" rx="32" ry="6.5" fill="${MPAL.cocoa}" opacity=".16"/><g transform="translate(-11,0)"><path d="M-14 0 A14 13 0 0 1 14 0 Z" fill="${color}" stroke="${MPAL.bark}" stroke-width="1.3"/><path d="M0 -13.5 L0 0" stroke="${MPAL.bark}" stroke-width="0.7"/><path d="M-6 0 A6 8 0 0 1 6 0 Z" fill="${MPAL.saddle}" stroke="${MPAL.bark}" stroke-width="0.8"/></g><g transform="translate(16,0) scale(.55)">${mFire(1)}</g>${mBadge(n, 2, 17, color)}</g>`,
  cabin: (n, color, s = 1) => `<g transform="scale(${s})"><ellipse cx="0" cy="2" rx="34" ry="7" fill="${MPAL.cocoa}" opacity=".18"/><g transform="translate(-4,0)">${mSmoke(8, -24, [{ r:2.2, dx:2, o:.34, d:3.3, dl:.2 }, { r:2, dx:-2, o:.28, d:3.7, dl:1.3 }])}<rect x="9" y="-24" width="3.6" height="10" fill="${MPAL.bark}"/><rect x="-13" y="-12" width="26" height="12" fill="${color}" stroke="${MPAL.bark}" stroke-width="1.2"/><path d="M-16 -12 L0 -22 L16 -12 Z" fill="${MPAL.saddle}" stroke="${MPAL.bark}" stroke-width="1.2" stroke-linejoin="round"/><rect x="-3.5" y="-8" width="7" height="8" fill="${MPAL.bark}"/><rect x="6" y="-8" width="4.5" height="4.5" fill="${MPAL.wheat}" stroke="${MPAL.bark}" stroke-width="0.6"/></g>${mBadge(n, 0, 17, color)}</g>`,
  hammock: (n, color, s = 1) => `<g transform="scale(${s})"><ellipse cx="0" cy="2" rx="34" ry="6.5" fill="${MPAL.cocoa}" opacity=".16"/><g transform="translate(-18,1) scale(.72)">${mPine(1)}</g><g transform="translate(18,1) scale(.72)">${mPine(1)}</g><path d="M-15 -13 Q 0 -1 15 -13" fill="none" stroke="${color}" stroke-width="3.2" stroke-linecap="round"/><g transform="translate(0,0) scale(.4)">${mFire(1)}</g>${mBadge(n, 0, 17, color)}</g>`,
};
const M_POOL = ['hammock', 'cabin', 'dome', 'twin', 'full'];
function mHash(str) { let h = 2166136261 >>> 0; for (let i = 0; i < str.length; i += 1) { h ^= str.charCodeAt(i); h = Math.imul(h, 16777619) >>> 0; } return h; }
function mCampFor(cidr, worst) { if (worst === 'critical') return M_CAMPS.full; return M_CAMPS[M_POOL[mHash(cidr) % M_POOL.length]]; }
function mSnowCap(ax, ay, lx, rx, by) { const t = 0.34, lpx = ax + t * (lx - ax), py = ay + t * (by - ay), rpx = ax + t * (rx - ax), midx = (lpx + rpx) / 2; return `M${ax.toFixed(0)} ${ay.toFixed(0)} L${lpx.toFixed(0)} ${py.toFixed(0)} Q ${((lpx + midx) / 2).toFixed(0)} ${(py + 4).toFixed(0)} ${midx.toFixed(0)} ${(py + 1).toFixed(0)} Q ${((midx + rpx) / 2).toFixed(0)} ${(py + 4).toFixed(0)} ${rpx.toFixed(0)} ${py.toFixed(0)} Z`; }
function mMountainRange(by) {
  const far = `<path d="M-20 ${by} L110 ${by - 58} L182 ${by - 30} L280 ${by - 66} L362 ${by - 32} L470 ${by - 60} L560 ${by - 30} L680 ${by - 58} L782 ${by - 30} L880 ${by - 52} L1020 ${by - 28} L1020 ${by} Z" fill="${MPAL.stone}" opacity=".22"/>`;
  const mid = `<path d="M-20 ${by} L150 ${by - 72} L260 ${by - 30} L400 ${by - 78} L520 ${by - 32} L660 ${by - 76} L820 ${by - 30} L940 ${by - 66} L1020 ${by - 30} L1020 ${by} Z" fill="${MPAL.stone}" opacity=".44"/>`;
  const peaks = [{ ax:250, ay:by - 96, lx:150, rx:356 }, { ax:760, ay:by - 92, lx:660, rx:860 }, { ax:500, ay:by - 128, lx:392, rx:610 }].map((p) => `<path d="M${p.lx} ${by} L${p.ax} ${p.ay} L${p.rx} ${by} Z" fill="${MPAL.stoned}" stroke="${MPAL.bark}" stroke-width="1.1" stroke-linejoin="round"/><path d="${mSnowCap(p.ax, p.ay, p.lx, p.rx, by)}" fill="#eef3e6"/>`).join('');
  return `<g>${far}${mid}${peaks}</g>`;
}
function mEntityIP(e) {
  const ids = e.identifiers || [];
  for (const id of ids) { if (id.type === 'ip') return id.value; }
  const a = (e.attributes && typeof e.attributes === 'object') ? e.attributes : {};
  if (a.ip) return String(a.ip);
  for (const id of ids) { const m = String(id.value || '').match(/(\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3})/); if (m) return m[1]; }
  return null;
}
function mSubnet(ip) { if (!ip) return null; const p = ip.split('.'); if (p.length < 4) return null; return `${p[0]}.${p[1]}.${p[2]}.0/24`; }
// A segment key + display label for an entity. Hosts with an IP group by /24;
// IP-less directory objects (AD identities) get their own "Directory" camp
// rather than an anonymous "unresolved" bucket.
function mSegmentKey(e) {
  const cidr = mSubnet(mEntityIP(e));
  if (cidr) return { key: cidr, label: cidr };
  const ids = e.identifiers || [];
  if (e.kind === 'identity' || ids.some((i) => i.type === 'ad_sid')) return { key: 'directory', label: 'Directory · AD' };
  return { key: 'off-subnet', label: 'Off-subnet' };
}
function mBuildSegments() {
  const sevByEntity = {};
  (state.findings || []).forEach((f) => { const s = String(f.severity || 'info').toLowerCase(); (f.affectedEntityIds || []).forEach((id) => { if (!(id in sevByEntity) || MSEV_RANK[s] < MSEV_RANK[sevByEntity[id]]) sevByEntity[id] = s; }); });
  const segs = {};
  (state.entities || []).forEach((e) => {
    const sk = mSegmentKey(e);
    if (!segs[sk.key]) segs[sk.key] = { cidr: sk.key, label: sk.label, hosts: [], worst: 'info', findings: 0 };
    segs[sk.key].hosts.push(e);
    const s = sevByEntity[e.id] || 'info';
    if (MSEV_RANK[s] < MSEV_RANK[segs[sk.key].worst]) segs[sk.key].worst = s;
    if (e.id in sevByEntity) segs[sk.key].findings += 1;
  });
  return Object.values(segs).map((s) => ({ ...s, n: s.hosts.length })).sort((a, b) => MSEV_RANK[a.worst] - MSEV_RANK[b.worst] || b.n - a.n);
}
function mSegmentRole(seg) {
  for (const h of seg.hosts) { const a = (h.attributes && typeof h.attributes === 'object') ? h.attributes : {}; if (a.role) return String(a.role); }
  return seg.hosts[0] ? (seg.hosts[0].kind || 'segment') : 'segment';
}

const M_ACTOR_COLORS = ['#ba7517', '#639922', '#b04c30', '#854f0b', '#8b5e34'];
// Derive each operator/agent's trail from their captured actions: the ordered
// list of segments they targeted over time. Grounded in real /actions data.
function mBuildTrails(positions) {
  const keyByTarget = {};
  (state.entities || []).forEach((e) => {
    const sk = mSegmentKey(e);
    const ip = mEntityIP(e); if (ip) keyByTarget[ip] = sk.key;
    (e.identifiers || []).forEach((id) => { if (id.value) keyByTarget[String(id.value).toLowerCase()] = sk.key; });
    const a = (e.attributes && typeof e.attributes === 'object') ? e.attributes : {};
    if (a.hostname) keyByTarget[String(a.hostname).toLowerCase()] = sk.key;
  });
  const segForTarget = (tv) => {
    if (!tv) return null;
    const low = String(tv).toLowerCase();
    if (keyByTarget[low] && positions[keyByTarget[low]]) return keyByTarget[low];
    const m = low.match(/(\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3})/);
    if (m) { const cidr = mSubnet(m[1]); if (cidr && positions[cidr]) return cidr; if (keyByTarget[m[1]] && positions[keyByTarget[m[1]]]) return keyByTarget[m[1]]; }
    return null;
  };
  const startedAt = (a) => (a.capture && a.capture.timing && a.capture.timing.startedAt) || a.receivedAt || '';
  const acts = (state.actions || []).slice().sort((x, y) => { const a = startedAt(x), b = startedAt(y); return a < b ? -1 : a > b ? 1 : 0; });
  const byActor = {};
  acts.forEach((act) => {
    const actor = act.actor || {};
    const handle = actor.handle || actor.id || 'operator';
    const seg = segForTarget(act.capture && act.capture.target && act.capture.target.value);
    if (!seg || !positions[seg]) return;
    if (!byActor[handle]) byActor[handle] = { handle, kind: actor.kind, segs: [] };
    const arr = byActor[handle].segs;
    if (arr[arr.length - 1] !== seg) arr.push(seg);
  });
  return Object.values(byActor).filter((t) => t.segs.length).map((t, i) => ({ ...t, color: M_ACTOR_COLORS[i % M_ACTOR_COLORS.length] }));
}
function mFootpath(a, b, color) {
  const dx = b.x - a.x, dy = b.y - a.y; const len = Math.hypot(dx, dy) || 1; const n = Math.max(2, Math.round(len / 16));
  const nx = -dy / len, ny = dx / len; const ang = Math.atan2(dy, dx) * 180 / Math.PI; let out = '';
  for (let i = 1; i < n; i += 1) { const t = i / n; const x = a.x + dx * t, y = a.y + dy * t; const off = (i % 2 ? 3.4 : -3.4); out += `<g transform="translate(${(x + nx * off).toFixed(0)},${(y + ny * off).toFixed(0)}) rotate(${ang.toFixed(0)})"><ellipse rx="2.6" ry="1.4" fill="${color}" opacity=".85"/></g>`; }
  return out;
}
function mTrailSVG(trails, positions) {
  const hl = state.mapHighlightActor;
  return trails.map((t) => {
    const segs = t.segs.filter((s) => positions[s]);
    let path = '';
    for (let i = 1; i < segs.length; i += 1) path += mFootpath(positions[segs[i - 1]], positions[segs[i]], t.color);
    const head = positions[segs[segs.length - 1]];
    const initials = t.handle.split(/[.\s@_-]/).map((w) => w[0] || '').join('').toUpperCase().slice(0, 2) || '?';
    const flag = head ? `<g transform="translate(${(head.x + 42).toFixed(0)},${(head.y - 24).toFixed(0)})"><rect x="-1" y="0" width="2.4" height="22" rx="1" fill="${MPAL.bark}"/><path d="M1.4 0 L18 4.5 L1.4 9 Z" fill="${t.color}" stroke="${MPAL.bark}" stroke-width="0.7"/><text x="8.5" y="7.5" text-anchor="middle" font-size="6.5" font-weight="800" fill="#fff" font-family="Inter,sans-serif">${escapeHtml(initials)}</text></g>` : '';
    return `<g class="territory-route" data-route="${escapeHtml(t.handle)}" style="opacity:${hl && hl !== t.handle ? 0.14 : 1}">${path}${flag}</g>`;
  }).join('');
}

/* ============================ Left nav ============================ */
const NAV_ITEMS = [
  { key: 'trail', label: 'Trail', icon: '<path d="M6 21c1-6 6-7 6-13" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"/><path d="M12 8c0-4 4-5 6-5" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"/><circle cx="6" cy="21" r="1.6"/><circle cx="18" cy="3" r="1.6"/>' },
  { key: 'devices', label: 'Devices', icon: '<rect x="4" y="4" width="16" height="4" rx="1.5"/><rect x="4" y="10" width="16" height="4" rx="1.5"/><rect x="4" y="16" width="16" height="4" rx="1.5"/>' },
  { key: 'map', label: 'Map', icon: '<path d="M3 20 L9 7 L13 14 L16 9 L21 20 Z"/><path d="M9 7 L11 10 L7 10 Z" fill="#fff"/>' },
  { key: 'board', label: 'Board', icon: '<rect x="4" y="5" width="4" height="15" rx="1.2"/><rect x="10" y="5" width="4" height="10" rx="1.2"/><rect x="16" y="5" width="4" height="7" rx="1.2"/>' },
  { key: 'report', label: 'Report', icon: '<path d="M7 3h7l4 4v14H7z" fill="none" stroke="currentColor" stroke-width="2" stroke-linejoin="round"/><path d="M14 3v4h4" fill="none" stroke="currentColor" stroke-width="2" stroke-linejoin="round"/><path d="M10 12h6M10 16h6" stroke="currentColor" stroke-width="2" stroke-linecap="round"/>' },
];
function renderNav(active) {
  const items = NAV_ITEMS.map((it) => `<a class="appnav-item${active === it.key ? ' is-active' : ''}" href="#" data-action="goto-view" data-nav="${it.key}" aria-current="${active === it.key ? 'page' : 'false'}" aria-label="${it.label}"><span class="appnav-ico"><svg viewBox="0 0 24 24" fill="currentColor" aria-hidden="true" focusable="false">${it.icon}</svg></span><span class="appnav-label">${it.label}</span></a>`).join('');
  return `<nav class="appnav" aria-label="Views"><span class="appnav-brand" aria-hidden="true"></span>${items}</nav>`;
}

/* ============================ Device Atlas ============================ */
function mSevByEntity() {
  const map = {};
  (state.findings || []).forEach((f) => { const s = String(f.severity || 'info').toLowerCase(); (f.affectedEntityIds || []).forEach((id) => { if (!(id in map) || MSEV_RANK[s] < MSEV_RANK[map[id]]) map[id] = s; }); });
  return map;
}
function mHostRows() {
  const sev = mSevByEntity();
  return (state.entities || []).map((e) => {
    const ip = mEntityIP(e);
    const a = (e.attributes && typeof e.attributes === 'object') ? e.attributes : {};
    return {
      id: e.id,
      name: (e.identifiers && e.identifiers[0] && e.identifiers[0].value) || a.hostname || e.kind || e.id,
      ip: ip || '', kind: e.kind || 'host', role: a.role || '',
      subnet: mSubnet(ip) || mSegmentKey(e).label,
      sev: sev[e.id] || 'info',
      seen: e.lastSeen ? formatTime(e.lastSeen) : '',
    };
  });
}
function filteredAtlasRows() {
  const q = state.atlasQuery.trim().toLowerCase();
  let rows = mHostRows().filter((r) => {
    if (state.atlasSev.size && !state.atlasSev.has(r.sev)) return false;
    if (q && !(`${r.name} ${r.ip} ${r.kind} ${r.role} ${r.subnet}`.toLowerCase().includes(q))) return false;
    return true;
  });
  rows.sort((a, b) => MSEV_RANK[a.sev] - MSEV_RANK[b.sev] || a.name.localeCompare(b.name));
  return rows;
}
function atlasRowsHTML(rows) {
  return rows.slice(0, 200).map((r) => `<tr>
      <td><span class="atlas-stripe" style="background:${MSEV_COLOR[r.sev]}"></span></td>
      <td><span class="atlas-host">${escapeHtml(r.name)}</span></td>
      <td class="mono muted">${escapeHtml(r.ip || '—')}</td>
      <td>${escapeHtml(r.kind)}</td>
      <td class="mono muted">${escapeHtml(r.subnet)}</td>
      <td><span class="sev-pill"><i style="background:${MSEV_COLOR[r.sev]}"></i>${MSEV_LABEL[r.sev]}</span></td>
      <td class="muted">${escapeHtml(r.seen)}</td>
    </tr>`).join('');
}
function drawAtlasRows() {
  const rows = filteredAtlasRows();
  const body = document.getElementById('atlas-rows');
  if (body) body.innerHTML = atlasRowsHTML(rows);
  const foot = document.getElementById('atlas-foot');
  if (foot) foot.innerHTML = `Showing <b>${Math.min(rows.length, 200)}</b> of <b>${mHostRows().length}</b> hosts`;
  document.querySelectorAll('#mAtlasFacets .atlas-facet').forEach((el) => { const s = el.dataset.sev; if (s) el.classList.toggle('on', state.atlasSev.has(s)); });
}
function renderDeviceAtlas() {
  const all = mHostRows();
  const sevCounts = { critical: 0, high: 0, medium: 0, low: 0, info: 0 };
  all.forEach((r) => { sevCounts[r.sev] += 1; });
  const withFindings = all.filter((r) => r.sev !== 'info').length;
  const facets = MSEV_ORDER.map((s) => `<label class="atlas-facet${state.atlasSev.has(s) ? ' on' : ''}" data-sev="${s}"><input type="checkbox" data-action="atlas-sev" data-sev="${s}" ${state.atlasSev.has(s) ? 'checked' : ''}/><span class="sev-pill"><i style="background:${MSEV_COLOR[s]}"></i>${MSEV_LABEL[s]}</span><span class="atlas-c num">${sevCounts[s]}</span></label>`).join('');
  return `
    <main class="app-shell atlas-shell">
      ${renderNav('devices')}
      <header class="masthead">
        <div class="masthead-copy"><p class="eyebrow">Waypoint · device atlas</p><h1>Devices</h1><p class="subtitle">Every discovered host, filterable by severity — the operator's inventory.</p></div>
        <div class="masthead-actions">
          <div class="theme-switcher" role="group" aria-label="Theme selection">
            <button type="button" class="${state.theme === 'light' ? 'is-active' : ''}" data-action="set-theme" data-theme="light" aria-pressed="${state.theme === 'light'}">Light</button>
            <button type="button" class="${state.theme === 'dark' ? 'is-active' : ''}" data-action="set-theme" data-theme="dark" aria-pressed="${state.theme === 'dark'}">Dark</button>
          </div>
          <div class="metrics" aria-label="Inventory summary">
            <div class="metric"><span class="metric-label">Hosts</span><strong>${all.length.toLocaleString()}</strong></div>
            <div class="metric"><span class="metric-label">With findings</span><strong>${withFindings}</strong></div>
            <div class="metric"><span class="metric-label">Critical</span><strong>${sevCounts.critical}</strong></div>
          </div>
        </div>
      </header>
      <div class="atlas-layout">
        <aside class="atlas-rail" id="mAtlasFacets">
          <div class="atlas-search"><svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="11" cy="11" r="7"/><path d="M21 21l-4-4"/></svg><input data-action="atlas-search" placeholder="Search host / IP / kind" value="${escapeHtml(state.atlasQuery)}" aria-label="Search hosts"/></div>
          <h4>Severity</h4>${facets}
        </aside>
        <div class="atlas-main">
          <div class="atlas-table">
            <div class="atlas-thead">Host inventory</div>
            <div class="atlas-scroll"><table>
              <thead><tr><th></th><th>Host</th><th>Address</th><th>Kind</th><th>Segment</th><th>Severity</th><th>Last seen</th></tr></thead>
              <tbody id="atlas-rows">${atlasRowsHTML(filteredAtlasRows())}</tbody>
            </table></div>
            <div class="atlas-tfoot"><span id="atlas-foot">Showing <b>${Math.min(all.length, 200)}</b> of <b>${all.length}</b> hosts</span><span class="muted">Sorted by severity</span></div>
          </div>
        </div>
      </div>
    </main>`;
}

/* ============================ Base Camp Board ============================ */
const BOARD_COLS = [
  { key: 'info', label: 'Discovered' },
  { key: 'low', label: 'Analyzed' },
  { key: 'medium', label: 'At risk' },
  { key: 'high', label: 'Exploited' },
  { key: 'critical', label: 'Owned' },
];
function mRecentByActor() {
  const startedAt = (a) => (a.capture && a.capture.timing && a.capture.timing.startedAt) || a.receivedAt || '';
  const byActor = {};
  (state.actions || []).forEach((a) => {
    const h = (a.actor && (a.actor.handle || a.actor.id)) || 'operator';
    const t = startedAt(a);
    if (!byActor[h] || t > byActor[h].t) byActor[h] = { handle: h, kind: a.actor && a.actor.kind, command: (a.capture && a.capture.command) || '', target: (a.capture && a.capture.target && a.capture.target.value) || '', phase: (a.capture && a.capture.phase) || '', t };
  });
  return Object.values(byActor).sort((x, y) => (x.t < y.t ? 1 : x.t > y.t ? -1 : 0));
}
function renderBaseCampBoard() {
  const rows = mHostRows();
  const buckets = { info: [], low: [], medium: [], high: [], critical: [] };
  rows.forEach((r) => { (buckets[r.sev] || buckets.info).push(r); });
  const workers = mRecentByActor();
  const strip = workers.length ? `<div class="board-strip"><span class="board-strip-title">Active now · from recent captures</span><div class="board-workers">${workers.slice(0, 6).map((w) => `<div class="board-worker${w.kind === 'ai_agent' ? ' ai' : ''}"><span class="board-av" style="background:${w.kind === 'ai_agent' ? MSEV_COLOR.high : MPAL.saddle}">${escapeHtml(w.handle.slice(0, 2).toUpperCase())}</span><div class="board-wb"><div class="board-wn">${escapeHtml(w.handle)}${w.kind === 'ai_agent' ? '<b class="ai">AI</b>' : ''}</div><div class="board-wf">${w.command ? `<span class="tg">${escapeHtml(w.command)}</span>${w.target ? ` → ${escapeHtml(w.target)}` : ''}` : 'idle'}</div></div></div>`).join('')}</div></div>` : '';
  const cardHTML = (r) => `<div class="board-card"><div class="board-card-top"><span class="board-id mono">${escapeHtml(r.name)}</span><span class="sev-tag"><i style="background:${MSEV_COLOR[r.sev]}"></i>${MSEV_LABEL[r.sev]}</span></div><div class="board-ip mono muted">${escapeHtml(r.ip || r.subnet)}</div>${r.role ? `<div class="board-chips"><span class="board-chip">${escapeHtml(r.role)}</span><span class="board-chip">${escapeHtml(r.kind)}</span></div>` : ''}</div>`;
  const cols = BOARD_COLS.map((c) => {
    const list = buckets[c.key] || [];
    const expanded = state.boardExpanded.has(c.key);
    const shown = expanded ? list : list.slice(0, 6);
    const more = list.length - shown.length;
    return `<div class="board-col"><div class="board-col-head"><span class="board-dot" style="background:${MSEV_COLOR[c.key]}"></span><span class="board-col-name">${c.label}</span><span class="board-count num">${list.length}</span></div><div class="board-cards">${shown.map(cardHTML).join('') || '<div class="board-empty">—</div>'}</div>${more > 0 ? `<div class="board-more" data-action="board-more" data-col="${c.key}">↓ ${more} more</div>` : (expanded && list.length > 6 ? `<div class="board-more" data-action="board-more" data-col="${c.key}">↑ collapse</div>` : '')}</div>`;
  }).join('');
  const findings = (state.findings || []).slice().sort((a, b) => MSEV_RANK[String(a.severity || 'info').toLowerCase()] - MSEV_RANK[String(b.severity || 'info').toLowerCase()]);
  const findingCards = findings.map((f) => { const s = String(f.severity || 'info').toLowerCase(); return `<div class="board-card"><div class="board-card-top"><span class="board-id mono">FND</span><span class="sev-tag"><i style="background:${MSEV_COLOR[s]}"></i>${MSEV_LABEL[s]}</span></div><h5 class="board-fh">${escapeHtml(f.title || 'Finding')}</h5><div class="board-ip muted">${escapeHtml(f.status || 'open')} · ${(f.affectedEntityIds || []).length} host${(f.affectedEntityIds || []).length === 1 ? '' : 's'}</div></div>`; }).join('') || '<div class="board-empty">No findings yet</div>';
  const reportedCol = `<div class="board-col"><div class="board-col-head"><span class="board-dot" style="background:${MPAL.forest}"></span><span class="board-col-name">Reported</span><span class="board-count num">${findings.length}</span></div><div class="board-cards">${findingCards}</div></div>`;
  return `
    <main class="app-shell board-shell">
      ${renderNav('board')}
      <header class="masthead">
        <div class="masthead-copy"><p class="eyebrow">Waypoint · base camp board</p><h1>Base camp</h1><p class="subtitle">Hosts progress by risk; who's working on what streams in from recent captures.</p></div>
        <div class="masthead-actions">
          <div class="theme-switcher" role="group" aria-label="Theme selection">
            <button type="button" class="${state.theme === 'light' ? 'is-active' : ''}" data-action="set-theme" data-theme="light" aria-pressed="${state.theme === 'light'}">Light</button>
            <button type="button" class="${state.theme === 'dark' ? 'is-active' : ''}" data-action="set-theme" data-theme="dark" aria-pressed="${state.theme === 'dark'}">Dark</button>
          </div>
          <div class="metrics" aria-label="Board summary">
            <div class="metric"><span class="metric-label">Hosts</span><strong>${rows.length.toLocaleString()}</strong></div>
            <div class="metric"><span class="metric-label">Active</span><strong>${workers.length}</strong></div>
            <div class="metric"><span class="metric-label">Findings</span><strong>${findings.length}</strong></div>
          </div>
        </div>
      </header>
      ${strip}
      <div class="board-cols">${cols}${reportedCol}</div>
    </main>`;
}

function renderTerritoryMap() {
  const segments = mBuildSegments();
  const totalHosts = (state.entities || []).length;
  const totalFindings = (state.findings || []).length;
  const bandY = { critical:162, high:300, medium:432, low:558, info:672 };
  const W = 1000, H = 760;
  const present = MSEV_ORDER.filter((sev) => segments.some((s) => s.worst === sev));
  const nodeSVG = [];
  const positions = {};
  MSEV_ORDER.forEach((sev) => {
    const list = segments.filter((s) => s.worst === sev);
    if (!list.length) return;
    const leftPad = 150, rightPad = 60, span = (W - leftPad - rightPad) / Math.max(list.length - 1, 1);
    list.forEach((seg, i) => {
      const x = list.length === 1 ? (leftPad + W - rightPad) / 2 : leftPad + i * span;
      const y = bandY[sev] + (i % 2 ? 24 : -20);
      const scale = 0.72 + Math.min(Math.sqrt(seg.n) / 10, 0.62);
      positions[seg.cidr] = { x, y };
      const sel = state.mapSelectedSegment === seg.cidr;
      nodeSVG.push(`<g class="territory-camp${sel ? ' is-sel' : ''}" data-action="map-select" data-seg="${escapeHtml(seg.cidr)}" transform="translate(${x.toFixed(0)},${y.toFixed(0)})">${seg.worst === 'critical' ? `<ellipse cx="0" cy="${(-6 * scale).toFixed(0)}" rx="${(30 * scale).toFixed(0)}" ry="${(26 * scale).toFixed(0)}" fill="${MPAL.rust}" opacity=".12"/>` : ''}${mCampFor(seg.cidr, seg.worst)(seg.n, MSEV_COLOR[seg.worst], scale)}<text class="territory-camp-label" x="0" y="${(20 * scale + 30).toFixed(0)}" text-anchor="middle">${escapeHtml(seg.label || seg.cidr)}</text></g>`);
    });
  });
  const riskTicks = MSEV_ORDER.filter((sev) => segments.some((s) => s.worst === sev)).map((sev) => `<text class="territory-tick" x="20" y="${bandY[sev] - 2}">${MSEV_LABEL[sev]} risk</text><text class="territory-tick-sub" x="20" y="${bandY[sev] + 12}">${segments.filter((s) => s.worst === sev).reduce((a, s) => a + s.n, 0)} hosts</text>`).join('');
  const treeSpots = [[948, 300, 1.15], [70, 470, 1.3], [500, 726, 1.5], [220, 726, 1.25], [832, 724, 1.35], [664, 724, 1.4]];
  const treesSVG = treeSpots.map(([x, y, s]) => `<g transform="translate(${x},${y})">${mPine(s)}</g>`).join('');
  const legend = MSEV_ORDER.map((sev) => `<span class="territory-li"><i style="background:${MSEV_COLOR[sev]}"></i>${MSEV_LABEL[sev]}</span>`).join('');

  const sel = segments.find((s) => s.cidr === state.mapSelectedSegment) || segments[0] || null;
  const sideHTML = sel ? `<h3>Segment</h3><p class="territory-nm">${escapeHtml(sel.label || sel.cidr)}</p><div class="territory-mt">${escapeHtml(mSegmentRole(sel))} · ${sel.n} host${sel.n === 1 ? '' : 's'} · worst: ${MSEV_LABEL[sel.worst]}</div><h3>Hosts</h3><ul class="territory-hostlist">${sel.hosts.slice(0, 60).map((h) => { const ip = mEntityIP(h); return `<li><span class="territory-dot" style="background:${MSEV_COLOR[sel.worst]}"></span><span class="territory-hn">${escapeHtml((h.identifiers && h.identifiers[0] && h.identifiers[0].value) || h.kind || h.id)}</span>${ip ? `<span class="territory-hip">${escapeHtml(ip)}</span>` : ''}</li>`; }).join('')}</ul>${sel.hosts.length > 60 ? `<p class="territory-more">+${sel.hosts.length - 60} more hosts</p>` : ''}` : '<h3>Segment</h3><p class="territory-mt">Select a campsite to inspect its hosts.</p>';

  const emptyState = segments.length ? '' : `<div class="territory-empty"><strong>No mapped hosts yet</strong><p>Entities with an IP address appear here as campsites, grouped by /24 subnet. Capture some recon to populate the map.</p></div>`;

  const lensOn = state.mapLens === 'operators';
  const trails = lensOn ? mBuildTrails(positions) : [];
  const trailSVG = lensOn ? mTrailSVG(trails, positions) : '';
  const lensControl = `<div class="territory-modeseg" role="group" aria-label="Trail lens"><button type="button" data-action="map-lens" data-lens="off" class="${!lensOn ? 'on' : ''}">Off</button><button type="button" data-action="map-lens" data-lens="operators" class="${lensOn ? 'on' : ''}">Operators</button></div>`;
  const actorLegend = (lensOn && trails.length) ? `<div class="territory-actors"><span class="territory-legend-title">Whose trail</span>${trails.map((t) => `<button type="button" class="territory-achip${state.mapHighlightActor === t.handle ? ' on' : ''}" data-action="map-actor" data-actor="${escapeHtml(t.handle)}"><i style="background:${t.color}"></i>${escapeHtml(t.handle)}${t.kind === 'ai_agent' ? '<b class="ai">AI</b>' : ''}</button>`).join('')}</div>` : '';

  return `
    <main class="app-shell territory-shell">
      ${renderNav('map')}
      <header class="masthead">
        <div class="masthead-copy">
          <p class="eyebrow">Waypoint · territory map</p>
          <h1>The estate</h1>
          <p class="subtitle">Subnets are campsites — sized by hosts, coloured by their worst finding, climbing toward the summit as risk rises.</p>
        </div>
        <div class="masthead-actions">
          <div class="theme-switcher" role="group" aria-label="Theme selection">
            <button type="button" class="${state.theme === 'light' ? 'is-active' : ''}" data-action="set-theme" data-theme="light" aria-pressed="${state.theme === 'light'}">Light</button>
            <button type="button" class="${state.theme === 'dark' ? 'is-active' : ''}" data-action="set-theme" data-theme="dark" aria-pressed="${state.theme === 'dark'}">Dark</button>
          </div>
          ${lensControl}
          <a class="secondary-link" href="${escapeHtml(phasePath(state.engagementId, 'attacks'))}" data-action="goto-trail">← Trail</a>
          <div class="metrics" aria-label="Estate summary">
            <div class="metric"><span class="metric-label">Segments</span><strong>${segments.length}</strong></div>
            <div class="metric"><span class="metric-label">Hosts</span><strong>${totalHosts.toLocaleString()}</strong></div>
            <div class="metric"><span class="metric-label">Findings</span><strong>${totalFindings}</strong></div>
          </div>
        </div>
      </header>
      <div class="territory-layout">
        <section class="territory-canvas" aria-label="Estate map">
          <svg viewBox="0 0 ${W} ${H}" preserveAspectRatio="xMidYMid meet" role="img" aria-label="Territory map of ${segments.length} segments">
            <rect class="territory-terrain" width="${W}" height="${H}"/>
            ${mMountainRange(150)}
            ${treesSVG}
            ${trailSVG}
            ${nodeSVG.join('')}
            ${riskTicks}
          </svg>
          <div class="territory-legend"><span class="territory-legend-title">Worst finding</span>${legend}</div>
          ${actorLegend}
          ${emptyState}
        </section>
        <aside class="territory-side" aria-label="Segment detail">${sideHTML}</aside>
      </div>
    </main>`;
}

function renderTrailMap() {
  const activeIndex = waypointOrder.indexOf(state.activePhase);
  const trailStatusText = `Trail ${Math.min(activeIndex + 1, waypointOrder.length)} / ${waypointOrder.length} · ${phaseNames[state.activePhase]}`;
  const traveled = Math.min(activeIndex + 1, waypointOrder.length);
  const waypoints = waypointOrder.map((phase, index) => {
    const waypointStateValue = index < activeIndex ? 'completed' : index === activeIndex ? 'current' : 'fog';
    const [x, y] = positions[index];
    return `
      <g class="waypoint ${waypointStateValue} ${phase === state.activePhase ? 'is-current' : ''}" role="none">
        <circle cx="${x}" cy="${y}" r="${waypointStateValue === 'current' ? 17 : 12}" class="waypoint-node"></circle>
        ${waypointStateValue === 'completed' ? `<path d="M${x - 4} ${y} l3 3 l6 -6" class="checkmark"></path>` : ''}
        ${phase === state.activePhase ? `<path d="M${x} ${y - 8} c -4 5 -5 8 -2 11 c -3 0 -5 3 -2 5 c 2 2 7 2 9 0 c 3 -2 1 -5 -2 -5 c 3 -3 1 -7 -3 -11" class="campfire"></path>` : ''}
        <text x="${x}" y="${y + 28}" text-anchor="middle" class="waypoint-label">${escapeHtml(phaseNames[phase])}</text>
      </g>`;
  }).join('');

  const hitboxes = waypointOrder.map((phase, index) => {
    const [x, y] = positions[index];
    const waypointStateValue = index < activeIndex ? 'completed' : index === activeIndex ? 'current' : 'fog';
    return `<button type="button" class="waypoint-hitbox${phase === state.activePhase ? ' is-active' : ''}" data-action="goto-phase" data-phase="${phase}" aria-current="${phase === state.activePhase ? 'step' : ''}" aria-label="${escapeHtml(`${phaseNames[phase]}, ${stateLabel(waypointStateValue)}${phase === state.activePhase ? ', you are here' : ''}`)}" style="left:${x / 6.4}%;top:${y / 3}%;"></button>`;
  }).join('');

  const alerts = notableAlerts();
  const guideNotesVisible = visibleGuideNotes();
  const activeGuideNote = guideNotesVisible[0] || guideNotes.find((note) => note.phase === state.activePhase) || guideNotes[0];
  const nextGuidePhase = notePhaseAfterActive();
  const logItems = state.auditEvents.length
    ? state.auditEvents.map((entry, index) => `
      <li class="${index === 0 ? 'is-current' : ''}">
        <strong>${escapeHtml(entry.type)}</strong> · ${escapeHtml(buildAuditSummary(entry))}<br>
        ${escapeHtml(entry.actor?.handle || '')} · ${escapeHtml(entry.origin?.kind || '')} · ${escapeHtml(formatTime(entry.occurredAt))}
      </li>`).join('')
    : '';
  const alertItems = alerts.length
    ? alerts.map((entry, index) => {
        const data = isRecord(entry.data) ? entry.data : {};
        return `<li class="${index === 0 ? 'is-current' : ''}"><strong>${escapeHtml(data.ruleId || 'trail alert')}</strong> · ${escapeHtml(data.title || buildAuditSummary(entry))}<br>${escapeHtml(entry.actor?.handle || '')} · ${escapeHtml(formatTime(entry.occurredAt))}${data.sourceActionId ? ` · action ${escapeHtml(data.sourceActionId)}` : ''}</li>`;
      }).join('')
    : '';

  const guideList = guideNotesVisible.length
    ? guideNotesVisible.map((note) => `
      <article class="guide-note-card" id="${escapeHtml(note.id)}">
        <p class="guide-note-kicker">${escapeHtml(phaseNames[note.phase])} · reviewed note</p>
        <h3>${escapeHtml(note.title)}</h3>
        <dl>
          <div><dt>What</dt><dd>${escapeHtml(note.what)}</dd></div>
          <div><dt>When</dt><dd>${escapeHtml(note.when)}</dd></div>
          <div><dt>Risks</dt><dd>${escapeHtml(note.risks)}</dd></div>
        </dl>
      </article>`).join('')
    : '<p class="guide-note-empty">No reviewed notes match this search.</p>';

  return `
    <main class="app-shell">
      ${renderNav('trail')}
      <header class="masthead">
        <div class="masthead-copy">
          <p class="eyebrow">Waypoint · expedition shell</p>
          <h1>Recon / Attacks / Findings</h1>
          <p class="subtitle">A calm trail map for the audit spine, with live data in the workspaces and the guide keeping pace.</p>
        </div>
        <div class="masthead-actions">
          <div class="theme-switcher" role="group" aria-label="Theme selection">
            <button type="button" class="${state.theme === 'light' ? 'is-active' : ''}" data-action="set-theme" data-theme="light" aria-pressed="${state.theme === 'light'}">Light</button>
            <button type="button" class="${state.theme === 'dark' ? 'is-active' : ''}" data-action="set-theme" data-theme="dark" aria-pressed="${state.theme === 'dark'}">Dark</button>
          </div>
          <a class="secondary-link" href="${escapeHtml(mapPath(state.engagementId))}" data-action="goto-map">⛰ Territory map</a>
          <label class="field-group">
            <span>Operator token</span>
            <input value="${escapeHtml(state.token)}" data-action="update-token" placeholder="Bearer token" aria-label="Operator token" />
          </label>
          <div class="progress-pill" aria-label="Trail progress">${escapeHtml(trailStatusText)}</div>
          <div class="metrics" aria-label="Engagement progress">
            <div class="metric"><span class="metric-label">Traveled</span><strong>${traveled} waypoints</strong></div>
            <div class="metric"><span class="metric-label">To summit</span><strong>${Math.max(0, waypointOrder.length - traveled)} left</strong></div>
          </div>
        </div>
      </header>

      <div class="layout">
        <section class="map-column">
          <section class="map-card" aria-label="Engagement trail map">
            <div class="map-stage">
              <svg viewBox="0 0 640 300" role="img" aria-label="Trail map with waypoint buttons">
                <rect width="640" height="300" class="map-terrain"></rect>
                <path d="M60 252 C 132 234, 148 194, 206 182 C 270 168, 286 220, 350 204 C 402 190, 420 148, 472 126 C 516 108, 548 84, 590 60" class="trail-path"></path>
                <path d="M0 84 Q 138 44, 250 76 T 470 62 T 640 84" class="contours"></path>
                <path d="M0 136 Q 160 94, 286 124 T 640 112" class="contours"></path>
                <path d="M0 192 Q 170 160, 332 184 T 640 166" class="contours"></path>
                <g class="trees" aria-hidden="true">
                  <path d="M92 244 L100 224 L108 244 Z"></path>
                  <path d="M118 250 L128 228 L138 250 Z"></path>
                  <path d="M182 96 L190 76 L198 96 Z"></path>
                  <path d="M510 248 L519 230 L528 248 Z"></path>
                  <path d="M536 110 L546 88 L556 110 Z"></path>
                </g>
                ${waypoints}
              </svg>
              <div class="waypoint-overlay" aria-label="Trail waypoint shortcuts">${hitboxes}</div>
            </div>
          </section>

          <section class="workspace-panel" aria-label="${escapeHtml(`${phaseNames[state.activePhase]} workspace`)}">
            <div class="workspace-header">
              <div>
                <p class="workspace-kicker">Stage ${waypointOrder.indexOf(state.activePhase) + 1} of ${waypointOrder.length}</p>
                <h2>${escapeHtml(`${phaseNames[state.activePhase]} workspace`)}</h2>
              </div>
              <div class="workspace-status-stack">
                <p class="workspace-status">Saved 2 min ago</p>
                ${renderBadge(state.activePhase === 'summit' ? 'review' : 'neutral')}
              </div>
            </div>
            <p class="workspace-lede">${escapeHtml(phaseSummary(state.activePhase))}</p>

            ${renderPhaseWorkspace()}

            ${renderOperationsShell()}

            <div class="workspace-footer">
              <a class="secondary-link" href="${escapeHtml(phasePath(state.engagementId, waypointOrder[Math.max(0, waypointOrder.indexOf(state.activePhase) - 1)]))}" data-action="go-prev">Back to ${escapeHtml(phaseNames[waypointOrder[Math.max(0, waypointOrder.indexOf(state.activePhase) - 1)]] || phaseNames.recon)}</a>
              <a class="primary-button" href="${escapeHtml(state.activePhase === 'summit' ? reportPath(state.engagementId) : phasePath(state.engagementId, waypointOrder[Math.min(waypointOrder.length - 1, waypointOrder.indexOf(state.activePhase) + 1)]))}" data-action="go-next">${escapeHtml(state.activePhase === 'summit' ? 'Open report preview →' : `Continue to ${phaseNames[waypointOrder[Math.min(waypointOrder.length - 1, waypointOrder.indexOf(state.activePhase) + 1)]]} →`)}</a>
            </div>
          </section>
        </section>

        <aside class="sidebar" aria-label="Guide and trail details">
          <section class="log-panel" aria-label="Notable alerts">
            <div class="panel-heading compact">
              <h2>⚑ Notable alerts</h2>
              <p>Alerts arrive from the live SSE stream and stay attached to the audit trail.</p>
            </div>
            ${state.auditStatus === 'loading' ? '<div class="guide-note-empty">Loading notable alerts…</div>' : ''}
            ${state.auditStatus === 'error' ? `<div class="live-banner"><strong>Alert feed error</strong> ${escapeHtml(state.auditError)}</div>` : ''}
            ${!alerts.length && state.auditStatus === 'ready' ? '<div class="guide-note-empty">No notable alerts yet. The trail is still open for live capture.</div>' : ''}
            <ul>${alertItems}</ul>
          </section>

          <nav class="route-nav" aria-label="Engagement waypoints">
            <div class="panel-heading">
              <h2>Waypoints</h2>
              <p>All phases stay accessible; fog means no data discovered yet.</p>
            </div>
            <ol>
              ${waypointOrder.map((phase, index) => `
                <li>
                  <a href="${escapeHtml(phasePath(state.engagementId, phase))}" class="route-link ${phase === state.activePhase ? 'is-active' : ''}" aria-current="${phase === state.activePhase ? 'step' : ''}" data-action="goto-phase" data-phase="${phase}">
                    <span class="route-link-copy"><strong>${escapeHtml(phaseNames[phase])}</strong><span>Stage ${index + 1} of ${waypointOrder.length}</span></span>
                    <span class="route-status ${stateLabel(index < waypointOrder.indexOf(state.activePhase) ? 'completed' : index === waypointOrder.indexOf(state.activePhase) ? 'current' : 'fog')} ">${escapeHtml(shortStateLabel(index < waypointOrder.indexOf(state.activePhase) ? 'completed' : index === waypointOrder.indexOf(state.activePhase) ? 'current' : 'fog'))}</span>
                  </a>
                </li>`).join('')}
            </ol>
          </nav>

          <section class="guide-panel artifact" aria-label="Guide's note">
            <div class="panel-icon" aria-hidden="true">🧭</div>
            <div class="guide-copy">
              <h2>Guide's note</h2>
              <p>${escapeHtml(activeGuideNote.what)}</p>
              <article id="${escapeHtml(activeGuideNote.id)}" class="guide-note-card">
                <p class="guide-note-kicker">${escapeHtml(`${phaseNames[state.activePhase]} · reviewed phase briefing`)}</p>
                <h3>${escapeHtml(activeGuideNote.title)}</h3>
                <dl>
                  <div><dt>What</dt><dd>${escapeHtml(activeGuideNote.what)}</dd></div>
                  <div><dt>When</dt><dd>${escapeHtml(activeGuideNote.when)}</dd></div>
                  <div><dt>Risks</dt><dd>${escapeHtml(activeGuideNote.risks)}</dd></div>
                </dl>
              </article>
              <div class="guide-tools">
                <label class="guide-search">
                  <span class="sr-only">Search reviewed guide notes</span>
                  <input value="${escapeHtml(state.guideQuery)}" data-action="guide-search" placeholder="Search reviewed phases and techniques" aria-label="Search reviewed guide notes" />
                </label>
                <button type="button" class="primary-button" data-action="goto-phase" data-phase="${nextGuidePhase}">Continue to ${escapeHtml(phaseNames[nextGuidePhase])} →</button>
              </div>
              <div class="guide-note-list" aria-label="Reviewed guide notes">${guideList}</div>
            </div>
          </section>

          <section class="log-panel" aria-label="Journey log">
            <div class="panel-heading compact">
              <h2>📖 Journey log</h2>
              <p>The audit trail is the journey log — one entry per meaningful action.</p>
            </div>
            ${state.auditStatus === 'loading' ? '<div class="guide-note-empty">Loading the latest trail entries…</div>' : ''}
            ${state.auditStatus === 'error' ? `<div class="live-banner"><strong>Journey log error</strong> ${escapeHtml(state.auditError)}</div>` : ''}
            ${!state.auditEvents.length && state.auditStatus === 'ready' ? '<div class="guide-note-empty">No trail entries yet. The workspace stays open and ready.</div>' : ''}
            <ul>${logItems}</ul>
          </section>

          <section class="route-summary" aria-label="Route summary">
            <div>
              <p class="metric-label">Current waypoint</p>
              <strong>${escapeHtml(currentWaypointLabel())}</strong>
            </div>
            <p>${escapeHtml(phaseSummary(state.activePhase))}</p>
          </section>
        </aside>
      </div>
    </main>`;
}

function renderPhaseWorkspace() {
  if (state.activePhase === 'recon') return renderReconWorkspace();
  if (state.activePhase === 'attacks') return renderAttacksWorkspace();
  if (state.activePhase === 'findings') return renderFindingsWorkspace();
  if (state.activePhase === 'summit') return renderSummitWorkspace();
  return '';
}

function renderReconWorkspace() {
  const entities = filteredEntities();
  const selected = selectedEntity();
  const entityRows = entities.length
    ? entities.map((entity) => `
      <button type="button" class="finding-card ${selected?.id === entity.id ? 'is-selected' : ''}" data-action="select-entity" data-id="${entity.id}">
        <div class="finding-card-head">
          <div>
            <p class="guide-note-kicker">${escapeHtml(entity.kind || 'entity')}</p>
            <h4>${escapeHtml(summaryLineForEntity(entity))}</h4>
          </div>
          ${renderBadge('neutral')}
        </div>
        <p class="finding-card-summary">First seen ${escapeHtml(formatTime(entity.firstSeen))} · last seen ${escapeHtml(formatTime(entity.lastSeen))}</p>
        <dl class="finding-card-meta">
          <div><dt>Observations</dt><dd>${(entity.observations || []).length}</dd></div>
          <div><dt>Action</dt><dd>${escapeHtml(entity.id === state.mergeSourceId ? 'Source' : entity.id === state.mergeTargetId ? 'Target' : 'Set merge slot')}</dd></div>
        </dl>
        <div class="detail-foot"><span class="route-status current">Open provenance</span><span class="route-status fog">Merge / split ready</span></div>
      </button>`).join('')
    : '';

  const selectedObs = selected?.observations || [];
  const obsRows = selectedObs.length
    ? selectedObs.map((observation) => `
      <button type="button" class="attack-row-button ${state.selectedObservationId === observation.id ? 'is-selected' : ''}" data-action="select-observation" data-id="${observation.id}">
        <div class="attack-row-top"><strong>${escapeHtml(observation.kind || 'observation')}</strong><span class="attack-row-time">${escapeHtml(formatTime(observation.observedAt))}</span></div>
        <div class="attack-row-main"><div class="attack-field"><strong>Action</strong><span>${escapeHtml(observation.sourceActionId || 'manual observation')}</span></div><div class="attack-field"><strong>Status</strong><span>${escapeHtml(observation.claimStatus)}</span></div></div>
      </button>`).join('')
    : '';

  return `
    <section class="findings-shell" aria-label="Recon workspace">
      <div class="finding-list-panel">
        <div class="panel-heading">
          <h2>Authoritative entities</h2>
          <p>Loaded from the API; provenance drill-in, merge, and split stay live.</p>
        </div>
        <div class="guide-tools" style="margin-top:0">
          <label class="guide-search">
            <span class="sr-only">Search entities</span>
            <input value="${escapeHtml(state.entityQuery)}" data-action="entity-search" placeholder="Search host, FQDN, MAC, SID…" />
          </label>
          <button type="button" class="secondary-link" data-action="refresh-entities">Refresh</button>
        </div>
        ${state.entitiesStatus === 'loading' ? '<div class="finding-empty">Loading entities…</div>' : ''}
        ${state.entitiesStatus === 'error' ? `<div class="live-banner"><strong>Recon error</strong> ${escapeHtml(state.entitiesError)}</div>` : ''}
        ${!entities.length && state.entitiesStatus === 'ready' ? '<div class="finding-empty"><strong>Nothing discovered yet</strong><p>No entities were returned for this engagement. Keep the workspace accessible and wait for recon to land.</p></div>' : ''}
        <div class="finding-list" style="margin-top:12px">${entityRows}</div>
      </div>

      <div class="finding-editor-card">
        ${selected ? `
          <div class="detail-head">
            <div><p class="guide-note-kicker">Selected entity</p><h3>${escapeHtml(summaryLineForEntity(selected))}</h3></div>
            ${renderBadge('review')}
          </div>
          <div class="detail-grid provenance-grid">
            <div><dt>First seen</dt><dd>${escapeHtml(formatTime(selected.firstSeen))}</dd></div>
            <div><dt>Last seen</dt><dd>${escapeHtml(formatTime(selected.lastSeen))}</dd></div>
            <div><dt>Revision</dt><dd>${selected.revision}</dd></div>
            <div><dt>Identifiers</dt><dd>${escapeHtml((selected.identifiers || []).map((identifier) => `${identifier.type}:${identifier.value}`).join(' · ') || 'None')}</dd></div>
          </div>
          <div class="evidence-split" style="margin-top:12px">
            <div class="evidence-box">
              <div class="evidence-head"><h4>Merge preview</h4><span class="evidence-kind">source / target</span></div>
              <p>Choose a source and target entity, then preview the authoritative merge before you apply it.</p>
              <div class="guide-tools">
                <button type="button" class="secondary-link" data-action="set-merge-source" data-id="${selected.id}">Use as source</button>
                <button type="button" class="secondary-link" data-action="set-merge-target" data-id="${selected.id}">Use as target</button>
                <button type="button" class="primary-button" data-action="merge-preview" ${!state.mergeSourceId || !state.mergeTargetId || state.mergeSourceId === state.mergeTargetId ? 'disabled' : ''}>Preview merge</button>
                <button type="button" class="primary-button" data-action="merge-apply" ${!state.mergeSourceId || !state.mergeTargetId || state.mergeSourceId === state.mergeTargetId ? 'disabled' : ''}>Apply merge</button>
              </div>
              <p class="detail-foot">Source: ${escapeHtml(state.mergeSourceId || 'unset')} · Target: ${escapeHtml(state.mergeTargetId || 'unset')}</p>
            </div>
            <div class="evidence-box">
              <div class="evidence-head"><h4>Split provenance</h4><span class="evidence-kind">observation</span></div>
              <p>Pick a provenance observation to split from the merged entity.</p>
              <label class="field-group">
                <span>Observation</span>
                <select data-action="observation-select">
                  <option value="">Choose an observation</option>
                  ${selectedObs.map((observation) => `<option value="${observation.id}" ${state.selectedObservationId === observation.id ? 'selected' : ''}>${escapeHtml(observation.kind || 'observation')} · ${escapeHtml(observation.id)}</option>`).join('')}
                </select>
              </label>
              <div class="guide-tools">
                <button type="button" class="secondary-link" data-action="split-preview" ${!state.selectedObservationId ? 'disabled' : ''}>Preview split</button>
                <button type="button" class="primary-button" data-action="split-apply" ${!state.selectedObservationId ? 'disabled' : ''}>Apply split</button>
              </div>
            </div>
          </div>
          ${selectedObs.length ? `
            <section class="finding-trace-panel" style="margin-top:12px">
              <div class="panel-heading compact"><h3>Provenance drill-in</h3><p>Each observation stays attributable and linked to the source action.</p></div>
              <div class="revision-list">${obsRows}</div>
            </section>` : ''}
          ${state.entityConflict ? `<div class="live-banner review"><strong>Recon note</strong> ${escapeHtml(state.entityConflict)}</div>` : ''}
        ` : '<div class="finding-empty"><strong>No entity selected</strong><p>Choose an entity to inspect provenance, merge candidates, and split points.</p></div>'}
      </div>
    </section>`;
}

function renderAttacksWorkspace() {
  const actions = filteredActions();
  const selected = selectedAction();
  const rows = actions.length
    ? actions.map((action) => `
      <div class="attack-row ${selected?.id === action.id ? 'is-selected' : ''}">
        <button type="button" class="attack-row-button" data-action="select-attack" data-id="${action.id}">
          <div class="attack-row-top"><strong>${escapeHtml(summaryLineForAction(action))}</strong>${renderBadge(action.capture.parsing.status === 'parsed' ? 'success' : action.capture.parsing.status === 'needs-plugin' ? 'review' : 'neutral')}</div>
          <div class="attack-row-main">
            <div class="attack-field"><strong>Actor</strong><span>${escapeHtml(action.actor.handle)}${action.actor.kind === 'ai_agent' ? ` · ${escapeHtml(action.actor.model || '')}` : ''}</span></div>
            <div class="attack-field"><strong>Target</strong><span>${escapeHtml(`${action.capture.target.kind}:${action.capture.target.value}`)}</span></div>
            <div class="attack-field"><strong>Host</strong><span>${escapeHtml(action.capture.network.execHost.address)}</span></div>
            <div class="attack-field"><strong>Result</strong><span>${escapeHtml(`${action.capture.execution.status} ${action.capture.execution.exitCode === 0 ? '· ok' : ''}`.trim())}</span></div>
          </div>
          <div class="attack-row-foot"><span>${escapeHtml(formatTime(action.capture.timing.startedAt))}</span><span>${escapeHtml(action.capture.parsing.plugin?.id || 'raw evidence')}</span></div>
        </button>
      </div>`).join('')
    : '';

  const evidenceDrill = state.selectedEvidence ? `
    <section class="finding-trace-panel">
      <div class="panel-heading compact"><h3>Evidence drill-in</h3><p>${escapeHtml(state.selectedEvidence.id)} · ${escapeHtml(state.selectedEvidence.mediaType)}</p></div>
      ${state.selectedEvidenceError ? `<div class="live-banner"><strong>Evidence error</strong> ${escapeHtml(state.selectedEvidenceError)}</div>` : ''}
      ${!state.selectedEvidenceError ? `
        <div class="evidence-split">
          <div class="evidence-box">
            <div class="evidence-head"><h4>Metadata</h4><span class="evidence-kind">${escapeHtml(state.selectedEvidence.role)}</span></div>
            <p><strong>Action:</strong> ${escapeHtml(state.selectedEvidence.actionId)}</p>
            <p><strong>SHA-256:</strong> ${escapeHtml(state.selectedEvidence.sha256)}</p>
            <p><strong>Created:</strong> ${escapeHtml(formatTime(state.selectedEvidence.createdAt))}</p>
            <p><strong>Path:</strong> ${escapeHtml(state.selectedEvidence.contentPath)}</p>
          </div>
          <div class="evidence-box">
            <div class="evidence-head"><h4>Raw content</h4><span class="evidence-kind">${escapeHtml(`${state.selectedEvidence.byteLength} bytes`)}</span></div>
            <pre>${escapeHtml(state.selectedEvidenceContent || 'Binary evidence or no text preview available.')}</pre>
          </div>
        </div>` : ''}
    </section>` : '';

  return `
    <section class="attack-toolbar" aria-label="Attack filters and promotion controls">
      <div class="attack-group-switcher" role="group" aria-label="Attack phase filter">
        ${['all', 'recon', 'attacks'].map((phase) => `<button type="button" class="${state.actionPhaseFilter === phase ? 'is-active' : ''}" data-action="attack-filter" data-phase="${phase}">${phase === 'all' ? 'All' : phaseNames[phase]}</button>`).join('')}
      </div>
      <div class="field-group"><span>Promote</span><button type="button" class="primary-button" data-action="jump-findings">Send to Findings</button></div>
      <div class="field-group"><span>Operator note</span><input value="${escapeHtml(state.promotionConflict)}" placeholder="Notable alerts and promotion status appear here." aria-label="Promotion message" readonly /></div>
    </section>
    <section class="attack-shell" aria-label="Attacks workspace">
      <div class="attack-list-column">
        <div class="attack-group">
          <div class="attack-group-header">
            <div><p class="guide-note-kicker">Authoritative actions</p><h3>${state.actionsStatus === 'loading' ? 'Loading attacks…' : 'Captured attempts'}</h3></div>
            <button type="button" class="secondary-link" data-action="refresh-actions">Refresh</button>
          </div>
          ${state.actionsStatus === 'error' ? `<div class="live-banner"><strong>Attack error</strong> ${escapeHtml(state.actionsError)}</div>` : ''}
          ${!actions.length && state.actionsStatus === 'ready' ? '<div class="finding-empty"><strong>No attacks yet</strong><p>There are no captured attempts for this phase. Keep the workspace open and wait for the next action.</p></div>' : ''}
          <div class="attack-group-list">${rows}</div>
        </div>
      </div>
      <div class="attack-detail">
        ${selected ? `
          <article class="detail-card" aria-label="Selected attack details">
            <div class="detail-head">
              <div><p class="guide-note-kicker">Selected action</p><h3>${escapeHtml(summaryLineForAction(selected))}</h3></div>
              ${renderBadge(selected.capture.parsing.status === 'parsed' ? 'success' : selected.capture.parsing.status === 'needs-plugin' ? 'review' : 'neutral')}
            </div>
            <div class="detail-grid provenance-grid">
              <div><dt>Actor</dt><dd>${escapeHtml(selected.actor.handle)}${selected.actor.kind === 'ai_agent' ? ` · ${escapeHtml(selected.actor.model || '')}` : ''}</dd></div>
              <div><dt>Initiated by</dt><dd>${escapeHtml(selected.capture.initiatedBy)}</dd></div>
              <div><dt>Exec host</dt><dd>${escapeHtml(selected.capture.network.execHost.address)}</dd></div>
              <div><dt>Egress</dt><dd>${escapeHtml(selected.capture.network.egress.address || selected.capture.network.egress.mode || 'off')}</dd></div>
              <div><dt>Target</dt><dd>${escapeHtml(`${selected.capture.target.kind}:${selected.capture.target.value}`)}</dd></div>
              <div><dt>Phase</dt><dd>${escapeHtml(selected.capture.phase)}</dd></div>
            </div>
            <div class="evidence-split">
              ${['stdout', 'stderr'].map((role) => {
                const ref = role === 'stdout' ? selected.evidenceReferences.stdout : selected.evidenceReferences.stderr;
                return `<button type="button" class="evidence-box" data-action="open-evidence" data-id="${ref.id}">
                  <div class="evidence-head"><h4>${role.toUpperCase()} evidence</h4><span class="evidence-kind">${escapeHtml(ref.mediaType)}</span></div>
                  <p>${ref.byteLength} bytes · ${escapeHtml(ref.sha256.slice(0, 12))}…</p>
                  <pre>${escapeHtml(ref.downloadPath)}</pre>
                </button>`;
              }).join('')}
            </div>
            <div class="detail-foot"><span>${escapeHtml(selected.capture.command)}</span><span>${escapeHtml(selected.capture.argv.join(' '))}</span></div>
            ${evidenceDrill}
          </article>` : '<div class="finding-empty"><strong>No attack selected</strong><p>Pick an action to inspect provenance, evidence, and promotion options.</p></div>'}
      </div>
    </section>
    <section class="findings-shell" aria-label="Promotion and attack evidence">
      <div class="finding-list-panel">
        <div class="panel-heading">
          <h2>Promote selected attack</h2>
          <p>Fill in a reviewable finding draft, then keep the evidence linked to the source action.</p>
        </div>
        ${selected ? `
          <form class="finding-editor-grid" data-action="promotion-form">
            <label class="finding-field"><span>Title</span><input name="title" value="${escapeHtml(state.findingPromotionDraft.title || `${selected.capture.command} on ${selected.capture.target.value}`)}" /></label>
            <label class="finding-field"><span>Severity</span><select name="severity"><option value="info">Info</option><option value="low">Low</option><option value="medium" ${state.findingPromotionDraft.severity === 'medium' ? 'selected' : ''}>Medium</option><option value="high">High</option><option value="critical">Critical</option></select></label>
            <label class="finding-field finding-field-wide"><span>Remediation</span><textarea name="remediation">${escapeHtml(state.findingPromotionDraft.remediation)}</textarea></label>
            <label class="finding-field"><span>Status</span><input name="status" value="${escapeHtml(state.findingPromotionDraft.status)}" /></label>
            <div class="finding-actions"><button type="submit" class="primary-button">Promote selected attack</button></div>
          </form>
          ${state.promotionConflict ? `<div class="live-banner review"><strong>Promotion</strong> ${escapeHtml(state.promotionConflict)}</div>` : ''}
        ` : '<div class="finding-empty"><strong>No attack selected</strong><p>Go back to Attacks and choose a captured attempt before promotion.</p></div>'}
      </div>
      <div class="finding-editor-card">
        <div class="panel-heading"><h2>Captured attempts</h2><p>Action rows are authoritative and stream from the live API.</p></div>
        <div class="finding-list">${rows}</div>
      </div>
    </section>`;
}

function renderFindingsWorkspace() {
  const rows = state.findings.length
    ? state.findings.map((finding) => `
      <button type="button" class="finding-card ${state.selectedFindingId === finding.id ? 'is-selected' : ''}" data-action="select-finding" data-id="${finding.id}">
        <div class="finding-card-head"><div><p class="guide-note-kicker">${escapeHtml(finding.status)}</p><h4>${escapeHtml(finding.title)}</h4></div>${renderBadge((finding.severity || '').toLowerCase().includes('high') || (finding.severity || '').toLowerCase().includes('critical') ? 'blocked' : (finding.severity || '').toLowerCase().includes('medium') ? 'review' : 'neutral')}</div>
        <p class="finding-card-summary">${escapeHtml(summaryLineForFinding(finding))}</p>
        <dl class="finding-card-meta"><div><dt>Evidence</dt><dd>${escapeHtml((finding.evidenceActionIds || []).join(', ') || 'none')}</dd></div><div><dt>Promoted by</dt><dd>${escapeHtml(finding.promotedBy || '')}</dd></div></dl>
      </button>`).join('')
    : '';
  const selected = selectedFinding();
  const promotionAction = selectedAction();

  return `
    <section class="findings-shell" aria-label="Findings workspace">
      <div class="finding-list-panel">
        <div class="panel-heading">
          <h2>Authoritative findings</h2>
          <p>Promote from an attack, then keep revisions and conflict states visible.</p>
        </div>
        ${state.findingsStatus === 'error' ? `<div class="live-banner"><strong>Findings error</strong> ${escapeHtml(state.findingConflict || state.findingsError)}</div>` : ''}
        ${!state.findings.length && state.findingsStatus === 'ready' ? '<div class="finding-empty"><strong>No findings yet</strong><p>Nothing has been promoted. Select an attack and send it over when the evidence is ready.</p></div>' : ''}
        <div class="finding-list">${rows}</div>
      </div>
      <div class="finding-editor-card">
        <div class="detail-head"><div><p class="guide-note-kicker">Promotion</p><h3>${promotionAction ? `Promote ${escapeHtml(promotionAction.capture.command)}` : 'Select an attack to promote'}</h3></div>${renderBadge('review')}</div>
        ${promotionAction ? `
          <form class="finding-editor-grid" data-action="promotion-form">
            <label class="finding-field"><span>Title</span><input name="title" value="${escapeHtml(state.findingPromotionDraft.title || `${promotionAction.capture.command} on ${promotionAction.capture.target.value}`)}" /></label>
            <label class="finding-field"><span>Severity</span><select name="severity"><option value="info">Info</option><option value="low">Low</option><option value="medium" ${state.findingPromotionDraft.severity === 'medium' ? 'selected' : ''}>Medium</option><option value="high">High</option><option value="critical">Critical</option></select></label>
            <label class="finding-field finding-field-wide"><span>Remediation</span><textarea name="remediation">${escapeHtml(state.findingPromotionDraft.remediation)}</textarea></label>
            <label class="finding-field"><span>Status</span><input name="status" value="${escapeHtml(state.findingPromotionDraft.status)}" /></label>
            <div class="finding-actions"><button type="submit" class="primary-button">Promote selected attack</button></div>
          </form>
          ${state.promotionConflict ? `<div class="live-banner review"><strong>Promotion</strong> ${escapeHtml(state.promotionConflict)}</div>` : ''}
        ` : '<div class="finding-empty"><strong>No attack selected</strong><p>Go back to Attacks and choose a captured attempt before promotion.</p></div>'}

        ${selected ? `
          <form class="finding-editor-grid" data-action="finding-form" style="margin-top:14px">
            <label class="finding-field"><span>Title</span><input name="title" value="${escapeHtml(selected.title)}" /></label>
            <label class="finding-field"><span>Severity</span><select name="severity"><option value="info" ${selected.severity === 'info' ? 'selected' : ''}>Info</option><option value="low" ${selected.severity === 'low' ? 'selected' : ''}>Low</option><option value="medium" ${selected.severity === 'medium' ? 'selected' : ''}>Medium</option><option value="high" ${selected.severity === 'high' ? 'selected' : ''}>High</option><option value="critical" ${selected.severity === 'critical' ? 'selected' : ''}>Critical</option></select></label>
            <label class="finding-field finding-field-wide"><span>Remediation</span><textarea name="remediation">${escapeHtml(selected.remediation)}</textarea></label>
            <label class="finding-field"><span>Status</span><input name="status" value="${escapeHtml(selected.status)}" /></label>
          </form>
          <div class="finding-actions"><button type="button" class="secondary-link" data-action="save-finding">Save revision</button></div>
          ${state.findingConflict ? `<div class="live-banner review"><strong>Finding note</strong> ${escapeHtml(state.findingConflict)}</div>` : ''}
          <section class="finding-trace-panel" style="margin-top:14px">
            <div class="panel-heading compact"><h3>Evidence trace</h3><p>${escapeHtml((selected.evidenceActionIds || []).join(', ') || 'No evidence linked yet.')}</p></div>
            <div class="finding-trace-list">${(selected.evidenceActionIds || []).map((actionId) => `<div class="revision-list"><span>${escapeHtml(actionId)}</span></div>`).join('')}</div>
          </section>
          <section class="finding-revision-panel" style="margin-top:14px">
            <div class="panel-heading compact"><h3>Revision history</h3><p>${escapeHtml(selected.id)}</p></div>
            <div class="revision-list"><div><span>Revision</span><small>${selected.revision}</small></div><div><span>Promoted</span><small>${escapeHtml(formatTime(selected.promotedAt))}</small></div><div><span>Updated</span><small>${escapeHtml(formatTime(selected.updatedAt))}</small></div></div>
          </section>
        ` : ''}
      </div>
    </section>`;
}

function renderSummitWorkspace() {
  const job = selectedExportJob();
  const receipt = selectedExportReceipt(job);
  const auth = selectedTeardownAuthorization(job);
  const selectedExportStatus = job?.state || (state.exportJobsStatus === 'loading' ? 'loading' : 'idle');
  const selectedExportProgress = job?.progress?.percent ?? 0;
  const selectedExportStage = job ? describeExportStage(job.progress.stage) : 'Ready to start export';
  const selectedExportFailure = job?.failure?.message || '';
  const summitDownloadReady = Boolean(job?.state === 'completed' && job.bundle && receipt && receipt.status === 'verified');
  const summitCanAuthorize = Boolean(receipt && receipt.status === 'verified' && receipt.bundlePath && receipt.archiveSha256 && receipt.manifestSha256);
  const summitCanConsume = Boolean(auth && auth.status === 'authorized');
  return `
    <section class="summit-flow" aria-label="Summit export and teardown flow">
      <div class="summit-status">
        <span class="status-chip ${exportStatusClass(selectedExportStatus)}">${escapeHtml(selectedExportStatus)}</span>
        <div class="export-meter" role="progressbar" aria-label="Export progress" aria-valuemin="0" aria-valuemax="100" aria-valuenow="${selectedExportProgress}"><span style="width:${selectedExportProgress}%"></span></div>
        <p class="summit-step">${escapeHtml(selectedExportStage)}</p>
        <p>${job ? `Server job ${escapeHtml(job.id)} · updated ${escapeHtml(formatTime(job.updatedAt))}` : 'The server owns export jobs, receipts, and teardown authorization.'}</p>
        ${state.summitRequestNote ? `<p class="summit-note">${escapeHtml(state.summitRequestNote)}</p>` : ''}
        ${selectedExportFailure ? `<p class="summit-error">${escapeHtml(selectedExportFailure)}</p>` : ''}
        ${state.selectedExportJobError ? `<p class="summit-error">${escapeHtml(state.selectedExportJobError)}</p>` : ''}
        ${state.summitActionError ? `<p class="summit-error">${escapeHtml(state.summitActionError)}</p>` : ''}
      </div>
      <div class="summit-controls">
        <button type="button" class="primary-button" data-action="run-export" ${state.exportAbort ? 'disabled' : ''}>Start export job</button>
        <button type="button" class="secondary-link" data-action="refresh-export-jobs" ${state.exportJobsStatus === 'loading' ? 'disabled' : ''}>Refresh jobs</button>
        <button type="button" class="secondary-link" data-action="cancel-export" ${!job || job.state === 'completed' || job.state === 'failed' || job.state === 'cancelled' ? 'disabled' : ''}>Cancel selected job</button>
        <button type="button" class="secondary-link" data-action="open-verified-pdf" ${!summitDownloadReady ? 'disabled' : ''}>Open verified PDF</button>
        <button type="button" class="secondary-link" data-action="download-verified-bundle" ${!summitDownloadReady ? 'disabled' : ''}>Download verified bundle</button>
      </div>
      <section class="summit-dashboard" aria-label="Persisted export jobs and receipts">
        <article class="summit-job-list">
          <div class="panel-heading compact">
            <h3>Persisted export jobs</h3>
            <p>Reconnectable server state, newest first.</p>
          </div>
          ${state.exportJobsStatus === 'error' ? `<div class="live-banner"><strong>Export jobs</strong> ${escapeHtml(state.exportJobsError)}</div>` : ''}
          ${!state.exportJobs.length && state.exportJobsStatus === 'ready' ? '<div class="guide-note-empty">Start a server-owned export job from this Summit waypoint.</div>' : ''}
          <div class="revision-list">
            ${state.exportJobs.map((item) => `
              <button type="button" class="attack-row-button ${job && job.id === item.id ? 'is-selected' : ''}" data-action="select-export-job" data-id="${item.id}">
                <div class="attack-row-top">
                  <strong>${escapeHtml(item.id)}</strong>
                  ${renderBadge(exportStatusClass(item.state))}
                </div>
                <div class="attack-row-main">
                  <div class="attack-field"><strong>Stage</strong><span>${escapeHtml(describeExportStage(item.progress.stage))}</span></div>
                  <div class="attack-field"><strong>Progress</strong><span>${item.progress.percent}%</span></div>
                  <div class="attack-field"><strong>Receipt</strong><span>${escapeHtml(item.bundle?.receiptId || 'pending')}</span></div>
                  <div class="attack-field"><strong>Updated</strong><span>${escapeHtml(formatTime(item.updatedAt))}</span></div>
                </div>
                <div class="attack-row-foot">
                  <span>${escapeHtml(item.requestedBy.handle)}</span>
                  <span>${escapeHtml(item.bundle?.archivePath || item.formatVersion)}</span>
                </div>
              </button>`).join('')}
          </div>
        </article>

        <article class="summit-detail-card" aria-label="Selected export job details">
          ${job ? `
            <div class="detail-head">
              <div>
                <p class="guide-note-kicker">Selected export job</p>
                <h3>${escapeHtml(job.id)}</h3>
              </div>
              ${renderBadge(exportStatusClass(job.state))}
            </div>
            <div class="detail-grid provenance-grid">
              <div><dt>Requested by</dt><dd>${escapeHtml(job.requestedBy.handle)}</dd></div>
              <div><dt>Created</dt><dd>${escapeHtml(formatTime(job.createdAt))}</dd></div>
              <div><dt>Started</dt><dd>${job.startedAt ? escapeHtml(formatTime(job.startedAt)) : 'Waiting on server'}</dd></div>
              <div><dt>Completed</dt><dd>${job.completedAt ? escapeHtml(formatTime(job.completedAt)) : 'Still running'}</dd></div>
              <div><dt>Cutoff</dt><dd>${escapeHtml(job.cutoff || 'Pending')}</dd></div>
              <div><dt>Revision</dt><dd>${job.revision}</dd></div>
            </div>
            <div class="receipt-grid export-receipt-grid">
              <div><dt>Progress stage</dt><dd>${escapeHtml(selectedExportStage)}</dd></div>
              <div><dt>Processed</dt><dd>${(job.progress.processedBytes || 0).toLocaleString()} bytes</dd></div>
              <div><dt>Bundle path</dt><dd>${escapeHtml(job.bundle?.archivePath || 'bundle pending')}</dd></div>
              <div><dt>Snapshot</dt><dd>${escapeHtml(job.bundle?.reportSnapshotId || job.snapshotId || 'pending')}</dd></div>
              <div><dt>Archive SHA-256</dt><dd class="report-snippet">${escapeHtml(job.bundle?.archiveSha256 || 'waiting on archive')}</dd></div>
              <div><dt>Manifest SHA-256</dt><dd class="report-snippet">${escapeHtml(job.bundle?.manifestSha256 || 'waiting on manifest')}</dd></div>
            </div>
            ${receipt ? `
              <article class="receipt-card" aria-label="Verified export receipt" style="margin-top:12px">
                <div class="panel-heading compact"><h3>Server receipt</h3><p>Hash verified, not signed. Verified by the server, not the browser.</p></div>
                <dl class="receipt-grid">
                  <div><dt>Status</dt><dd>${escapeHtml(receipt.status)}</dd></div>
                  <div><dt>Verified</dt><dd>${escapeHtml(formatTime(receipt.verifiedAt))}</dd></div>
                  <div><dt>Receipt</dt><dd>${escapeHtml(receipt.id)}</dd></div>
                  <div><dt>Bundle</dt><dd>${escapeHtml(receipt.bundlePath)}</dd></div>
                  <div><dt>Archive</dt><dd class="report-snippet">${escapeHtml(receipt.archiveSha256)}</dd></div>
                  <div><dt>Manifest</dt><dd class="report-snippet">${escapeHtml(receipt.manifestSha256)}</dd></div>
                </dl>
              </article>` : ''}
            ${auth ? `
              <article class="break-glass-panel" aria-label="Teardown authorization" style="margin-top:12px">
                <div class="panel-heading compact"><h3>Guarded teardown authorization</h3><p>The server will consume this one-time authorization immediately before the external wipe.</p></div>
                <dl class="receipt-grid">
                  <div><dt>Status</dt><dd>${escapeHtml(auth.status)}</dd></div>
                  <div><dt>Requested</dt><dd>${escapeHtml(formatTime(auth.requestedAt))}</dd></div>
                  <div><dt>Expires</dt><dd>${escapeHtml(formatTime(auth.expiresAt))}</dd></div>
                  <div><dt>Authorization</dt><dd>${escapeHtml(auth.id)}</dd></div>
                  <div><dt>Receipt</dt><dd>${escapeHtml(auth.receiptId)}</dd></div>
                  <div><dt>Consumed</dt><dd>${auth.consumedAt ? escapeHtml(formatTime(auth.consumedAt)) : 'Not yet consumed'}</dd></div>
                </dl>
              </article>` : ''}
            <div class="summit-controls" style="margin-top:12px">
              <label class="break-glass-input"><span>Type destroy verified engagement data</span><input value="${escapeHtml(state.destroyPhrase)}" data-action="destroy-phrase" placeholder="destroy verified engagement data" /></label>
              <button type="button" class="primary-button" data-action="authorize-teardown" ${!summitCanAuthorize || state.destroyPhrase.trim() !== 'destroy verified engagement data' ? 'disabled' : ''}>Authorize teardown</button>
              <button type="button" class="danger-button" data-action="consume-authorization" ${!summitCanConsume ? 'disabled' : ''}>Consume authorization</button>
            </div>
          ` : '<div class="guide-note-empty">Run or select an export job to reveal its verified bundle, receipt, and teardown guard.</div>'}
        </article>
      </section>
      ${state.reportSnapshot ? '<div class="live-banner review"><strong>Report</strong> Snapshot ready for preview in the Summit report view.</div>' : ''}
    </section>`;
}

function renderReportView() {
  const snapshot = state.reportSnapshot;
  return `
    <main class="app-shell report-shell" aria-label="Frozen report snapshot">
      ${renderNav('report')}
      <section class="report-hero artifact">
        <div>
          <p class="eyebrow">Waypoint · frozen report snapshot</p>
          <h1>${escapeHtml(snapshot?.title || 'Loading report snapshot')}</h1>
          <p class="subtitle">${snapshot ? `Version ${escapeHtml(snapshot.version)} · ${escapeHtml(snapshot.engagement)} · Cutoff ${escapeHtml(snapshot.cutoff)}` : 'The report is fetched from the authoritative API.'}</p>
        </div>
        <div class="report-toolbar">
          <button type="button" class="secondary-link" data-action="back-to-summit">Back to Summit</button>
          <button type="button" class="primary-button" data-action="open-pdf">Open PDF artifact</button>
        </div>
      </section>
      ${state.reportStatus === 'loading' ? '<div class="live-banner review"><strong>Loading</strong> Authoritative report snapshot in the pack…</div>' : ''}
      ${state.reportStatus === 'error' ? `<div class="live-banner"><strong>Report error</strong> ${escapeHtml(state.reportError)}</div>` : ''}
      ${snapshot ? `
        <article class="report-page" aria-label="Printable engagement report">
          <section class="report-section"><h2>Scope</h2><ul>${(snapshot.scope || []).map((item) => `<li>${escapeHtml(item)}</li>`).join('')}</ul></section>
          <section class="report-section"><h2>Methodology</h2><ul>${(snapshot.methodology || []).map((item) => `<li>${escapeHtml(item)}</li>`).join('')}</ul></section>
          <section class="report-section">
            <h2>Findings</h2>
            <div class="report-grid">
              ${(snapshot.findings || []).map((finding) => `
                <article class="report-card">
                  <p class="report-badge">${escapeHtml(finding.severity)}</p>
                  <h3>${escapeHtml(finding.title)}</h3>
                  <p>${escapeHtml(finding.status ? `${finding.status} · ` : '')}${escapeHtml(finding.promotedBy ? `Promoted by ${finding.promotedBy}` : '')}</p>
                  <p><strong>Evidence:</strong> ${escapeHtml((finding.evidence || []).join(', '))}</p>
                  <p><strong>Remediation:</strong> ${escapeHtml(finding.remediation)}</p>
                </article>`).join('')}
            </div>
          </section>
          <section class="report-section">
            <h2>Evidence</h2>
            <div class="report-grid">
              ${(snapshot.evidence || []).map((item) => `
                <article class="report-card">
                  <p class="report-badge">${escapeHtml(item.label)}</p>
                  <p><strong>Source:</strong> ${escapeHtml(item.command)}</p>
                  <p><strong>Target:</strong> ${escapeHtml(item.target)}</p>
                  <p><strong>Actor:</strong> ${escapeHtml(item.actor)}</p>
                  <p><strong>Host:</strong> ${escapeHtml(item.host)}</p>
                  <p><strong>Attribution:</strong> ${escapeHtml(item.attribution)}</p>
                  <p class="report-snippet">${escapeHtml(item.rawStdout)}</p>
                </article>`).join('')}
            </div>
          </section>
          <section class="report-section"><h2>Attribution</h2><div class="report-grid">${(snapshot.attribution || []).map((section) => `<article class="report-card"><h3>${escapeHtml(section.title)}</h3><ul>${(section.items || []).map((item) => `<li>${escapeHtml(item)}</li>`).join('')}</ul></article>`).join('')}</div></section>
          <section class="report-section"><h2>Known capture gaps</h2><ul>${(snapshot.knownCaptureGaps || []).map((item) => `<li>${escapeHtml(item)}</li>`).join('')}</ul></section>
        </article>` : ''}
    </main>`;
}

function renderSetupWizard() {
  const draft = state.setupDraft;
  const busy = state.setupStatus === 'saving';
  if (state.setupStep === 'done' && state.setupResult) {
    const result = state.setupResult;
    return `
    <main class="setup-shell">
      <div class="setup-backdrop" aria-hidden="true">
        <span class="setup-star" style="top:18%;left:12%"></span>
        <span class="setup-star" style="top:32%;left:82%"></span>
        <span class="setup-star" style="top:66%;left:24%"></span>
      </div>
      <section class="setup-card" aria-label="Setup complete">
        <div class="setup-badge" aria-hidden="true">⛺</div>
        <p class="setup-kicker">Base camp established</p>
        <h1>You're all set, ${escapeHtml(result.actorRecord.actor.handle)}</h1>
        <p class="setup-lede">Your engagement and owner account are ready. Save your owner token below — it is shown once, and only its digest is stored on the server.</p>
        <div class="secret-token-card" aria-label="Owner token">
          <div class="panel-heading compact"><h4>Owner token</h4><p>${escapeHtml(result.actorRecord.actor.handle)} · owner · issued ${escapeHtml(formatTime(result.issuedAt))}</p></div>
          <div class="secret-token" role="textbox" aria-readonly="true">${escapeHtml(result.token)}</div>
          <div class="guide-tools">
            <button type="button" class="secondary-link" data-action="copy-setup-token">Copy token</button>
            <span class="guide-note-empty">Do not paste this into the audit trail or logs.</span>
          </div>
        </div>
        <div class="setup-actions">
          <button type="button" class="primary-button" data-action="setup-enter">Enter Waypoint →</button>
        </div>
      </section>
    </main>`;
  }

  return `
    <main class="setup-shell">
      <div class="setup-backdrop" aria-hidden="true">
        <span class="setup-star" style="top:18%;left:12%"></span>
        <span class="setup-star" style="top:24%;left:70%"></span>
        <span class="setup-star" style="top:58%;left:86%"></span>
        <span class="setup-star" style="top:72%;left:18%"></span>
      </div>
      <section class="setup-card" aria-label="First-time setup">
        <div class="setup-badge" aria-hidden="true">🧭</div>
        <p class="setup-kicker">Waypoint · first-time setup</p>
        <h1>Set up your expedition</h1>
        <p class="setup-lede">Welcome. Create your first engagement and owner account to begin. No config files needed — just the setup code printed in the server's startup logs.</p>
        ${state.setupError ? `<div class="live-banner"><strong>Setup note</strong> ${escapeHtml(state.setupError)}</div>` : ''}
        <form class="setup-form" data-action="setup-form">
          ${state.setupCodeRequired ? `
            <label class="finding-field finding-field-wide">
              <span>Setup code</span>
              <input value="${escapeHtml(draft.code)}" data-action="setup-draft" data-field="code" placeholder="XXXX-XXXX-XXXX-XXXX" autocomplete="off" spellcheck="false" aria-describedby="setup-code-hint" />
            </label>
            <p class="guide-note-empty setup-hint" id="setup-code-hint">Find this in your server logs — look for the “WAYPOINT — FIRST-TIME SETUP” banner (e.g. <code>docker compose logs waypoint</code>).</p>
          ` : ''}
          <div class="setup-fieldset">
            <p class="setup-section-label">Engagement</p>
            <div class="setup-grid">
              <label class="finding-field"><span>Name</span><input value="${escapeHtml(draft.engagementName)}" data-action="setup-draft" data-field="engagementName" placeholder="Autumn Campus Assessment" /></label>
              <label class="finding-field"><span>Client</span><input value="${escapeHtml(draft.client)}" data-action="setup-draft" data-field="client" placeholder="Acme University" /></label>
              <label class="finding-field finding-field-wide"><span>Scope</span><input value="${escapeHtml(draft.scope)}" data-action="setup-draft" data-field="scope" placeholder="campus /16, excludes dorm VLANs" /></label>
            </div>
          </div>
          <div class="setup-fieldset">
            <p class="setup-section-label">Owner account</p>
            <div class="setup-grid">
              <label class="finding-field finding-field-wide"><span>Your handle</span><input value="${escapeHtml(draft.ownerHandle)}" data-action="setup-draft" data-field="ownerHandle" placeholder="alex.operator" /></label>
            </div>
            <p class="guide-note-empty setup-hint">You'll be the first owner — the human who can provision other operators and AI actors.</p>
          </div>
          <label class="setup-demo-toggle">
            <input type="checkbox" data-action="setup-demo" ${draft.demo ? 'checked' : ''} />
            <span class="setup-demo-copy">
              <span class="setup-demo-title">Load this as a demo instance</span>
              <span class="setup-demo-note">Fills the engagement with a coherent sample assessment — recon, attacks, findings, entities, and an audit trail — so you can explore a populated dashboard right away.</span>
            </span>
          </label>
          <div class="setup-actions">
            <button type="submit" class="primary-button" data-action="setup-submit" ${busy || !setupFormReady() ? 'disabled' : ''}>${busy ? 'Creating…' : 'Create engagement'}</button>
          </div>
        </form>
        <p class="guide-note-empty setup-foot">Running an automated deployment? Set the <code>WAYPOINT_BOOTSTRAP_*</code> environment variables to skip this screen entirely.</p>
      </section>
    </main>`;
}

function setupFormReady() {
  const draft = state.setupDraft;
  if (state.setupCodeRequired && !draft.code.trim()) return false;
  return Boolean(draft.engagementName.trim() && draft.client.trim() && draft.scope.trim() && draft.ownerHandle.trim());
}

async function submitSetup() {
  if (!setupFormReady() || state.setupStatus === 'saving') return;
  state.setupStatus = 'saving';
  state.setupError = '';
  render();
  const draft = state.setupDraft;
  try {
    const headers = {
      'Content-Type': 'application/json',
      'Waypoint-Contract-Version': apiVersion,
      Accept: 'application/json',
      'X-Request-ID': newRequestId(),
    };
    const response = await fetch('/api/v1/bootstrap', {
      method: 'POST',
      headers,
      cache: 'no-store',
      body: JSON.stringify({
        setupCode: draft.code.trim(),
        engagement: { name: draft.engagementName.trim(), client: draft.client.trim(), scope: draft.scope.trim() },
        owner: { handle: draft.ownerHandle.trim() },
        demo: Boolean(draft.demo),
      }),
    });
    if (!response.ok) throw new Error(await readProblem(response));
    const result = await response.json();
    state.setupResult = result;
    state.setupStep = 'done';
    state.setupStatus = 'done';
    state.setupRequired = false;
    state.engagementId = result.engagementId;
    render();
  } catch (error) {
    state.setupStatus = 'idle';
    state.setupError = error instanceof Error ? error.message : 'Setup failed';
    render();
  }
}

function enterAfterSetup() {
  if (!state.setupResult) return;
  const engagementId = state.setupResult.engagementId;
  setToken(state.setupResult.token);
  state.setupResult = null;
  state.view = 'trail';
  state.activePhase = 'recon';
  pushPath(phasePath(engagementId, 'recon'));
  render();
  void refreshEverything();
}

function render() {
  state.renderCount += 1;
  document.documentElement.dataset.theme = state.theme;
  document.documentElement.dataset.view = state.view;
  if (state.view === 'setup') {
    document.title = 'Waypoint — first-time setup';
    root.innerHTML = renderSetupWizard();
    return;
  }
  const titleByView = { report: 'Waypoint — report snapshot', map: 'Waypoint — territory map', devices: 'Waypoint — device atlas', board: 'Waypoint — base camp board' };
  document.title = titleByView[state.view] || `Waypoint — ${phaseNames[state.activePhase]}`;
  const viewRenderers = { report: renderReportView, map: renderTerritoryMap, devices: renderDeviceAtlas, board: renderBaseCampBoard };
  root.innerHTML = (viewRenderers[state.view] || renderTrailMap)();
  syncExportPolling();
}

async function handleSubmit(event) {
  const form = event.target.closest('form[data-action]');
  if (!form) return;
  const action = form.dataset.action;
  event.preventDefault();
  if (action === 'promotion-form') {
    updateFindingPromotionDraft(form);
    await promoteSelectedAttack(form);
  } else if (action === 'finding-form') {
    updateFindingDraft(form);
    await saveFinding(form);
  } else if (action === 'actor-provision-form') {
    await issueActorCredential();
  } else if (action === 'claim-resolution-form') {
    await resolveClaim();
  } else if (action === 'setup-form') {
    await submitSetup();
  }
}

// flashCopied gives a copy button an unmistakable "it worked" beat: the label
// becomes a checkmark and the .copied class runs the pop + ring animation. The
// original label and a pending timer are stashed on the element so rapid repeat
// clicks re-arm cleanly.
function flashCopied(button) {
  if (!button) return;
  if (button.dataset.copyLabel === undefined) {
    button.dataset.copyLabel = button.textContent;
  }
  if (button._copyTimer) clearTimeout(button._copyTimer);
  button.classList.remove('copied');
  // Force a reflow so re-adding the class restarts the animation on a fast
  // second click.
  void button.offsetWidth;
  button.classList.add('copied');
  button.textContent = 'Copied ✓';
  button._copyTimer = setTimeout(() => {
    button.classList.remove('copied');
    if (button.dataset.copyLabel !== undefined) {
      button.textContent = button.dataset.copyLabel;
    }
    button._copyTimer = null;
  }, 1600);
}

function copyToClipboard(text, button) {
  if (!text) return;
  const result = navigator.clipboard?.writeText(text);
  if (result && typeof result.then === 'function') {
    result.then(() => flashCopied(button)).catch(() => {});
  } else {
    flashCopied(button);
  }
}

async function handleClick(event) {
  const target = event.target.closest('[data-action]');
  if (!target) return;
  const action = target.dataset.action;
  if (action === 'set-theme') {
    setTheme(target.dataset.theme || 'light');
    return;
  }
  if (action === 'update-token') return;
  if (action === 'setup-draft') return;
  if (action === 'setup-submit') return;
  if (action === 'setup-enter') {
    enterAfterSetup();
    return;
  }
  if (action === 'copy-setup-token') {
    copyToClipboard(state.setupResult?.token, target);
    return;
  }
  if (action === 'goto-phase') {
    event.preventDefault();
    navigateToPhase(target.dataset.phase || 'attacks');
    return;
  }
  if (action === 'goto-map') {
    event.preventDefault();
    navigateToMap();
    return;
  }
  if (action === 'goto-trail') {
    event.preventDefault();
    navigateToPhase(target.dataset.phase || 'attacks');
    return;
  }
  if (action === 'map-select') {
    state.mapSelectedSegment = target.dataset.seg || '';
    render();
    return;
  }
  if (action === 'map-lens') {
    state.mapLens = target.dataset.lens || 'off';
    if (state.mapLens === 'off') state.mapHighlightActor = '';
    render();
    return;
  }
  if (action === 'map-actor') {
    const a = target.dataset.actor || '';
    state.mapHighlightActor = state.mapHighlightActor === a ? '' : a;
    render();
    return;
  }
  if (action === 'goto-view') {
    event.preventDefault();
    const nav = target.dataset.nav;
    if (nav === 'trail') navigateToPhase(state.activePhase && state.activePhase !== 'summit' ? state.activePhase : 'attacks');
    else if (nav === 'devices') navigateToDevices();
    else if (nav === 'map') navigateToMap();
    else if (nav === 'board') navigateToBoard();
    else if (nav === 'report') navigateToReport();
    return;
  }
  if (action === 'atlas-sev') {
    const s = target.dataset.sev;
    if (state.atlasSev.has(s)) state.atlasSev.delete(s); else state.atlasSev.add(s);
    drawAtlasRows();
    return;
  }
  if (action === 'board-more') {
    const c = target.dataset.col;
    if (state.boardExpanded.has(c)) state.boardExpanded.delete(c); else state.boardExpanded.add(c);
    render();
    return;
  }
  if (action === 'refresh-entities') {
    await refreshEntities();
    return;
  }
  if (action === 'refresh-actions') {
    await refreshActions();
    return;
  }
  if (action === 'guide-search') return;
  if (action === 'atlas-search') return;
  if (action === 'entity-search') return;
  if (action === 'actor-query') return;
  if (action === 'provision-draft') return;
  if (action === 'claim-resolution-draft') return;
  if (action === 'select-entity') {
    setSelectedEntityId(target.dataset.id || '');
    state.mergeSourceId = target.dataset.id || '';
    return;
  }
  if (action === 'set-merge-source') {
    state.mergeSourceId = target.dataset.id || '';
    render();
    return;
  }
  if (action === 'set-merge-target') {
    state.mergeTargetId = target.dataset.id || '';
    render();
    return;
  }
  if (action === 'merge-preview') {
    await runMergePreview();
    return;
  }
  if (action === 'merge-apply') {
    await applyMerge();
    return;
  }
  if (action === 'split-preview') {
    await runSplitPreview();
    return;
  }
  if (action === 'split-apply') {
    await applySplit();
    return;
  }
  if (action === 'select-observation') {
    setSelectedObservationId(target.dataset.id || '');
    return;
  }
  if (action === 'select-attack') {
    setSelectedActionId(target.dataset.id || '');
    return;
  }
  if (action === 'open-evidence') {
    await loadEvidence(target.dataset.id || '');
    return;
  }
  if (action === 'attack-filter') {
    setActionFilter(target.dataset.phase || 'attacks');
    return;
  }
  if (action === 'jump-findings') {
    navigateToPhase('findings');
    return;
  }
  if (action === 'select-finding') {
    setSelectedFindingId(target.dataset.id || '');
    return;
  }
  if (action === 'select-actor') {
    state.selectedActorId = target.dataset.id || '';
    render();
    return;
  }
  if (action === 'select-claim') {
    state.selectedClaimId = target.dataset.id || '';
    render();
    return;
  }
  if (action === 'use-selected-attack') {
    state.claimResolutionDraft.sourceActionId = selectedAction()?.id || state.claimResolutionDraft.sourceActionId;
    render();
    return;
  }
  if (action === 'burn-credential') {
    state.actorCredential = null;
    render();
    return;
  }
  if (action === 'copy-credential') {
    copyToClipboard(state.actorCredential?.token, target);
    return;
  }
  if (action === 'rotate-credential') {
    const actor = selectedActor();
    if (actor) await rotateActorCredential(actor);
    return;
  }
  if (action === 'revoke-credential') {
    const actor = selectedActor();
    if (actor) await revokeActorCredential(actor);
    return;
  }
  if (action === 'issue-credential') {
    await issueActorCredential();
    return;
  }
  if (action === 'save-finding') {
    const form = root.querySelector('form[data-action="finding-form"]');
    if (form) await saveFinding(form);
    return;
  }
  if (action === 'run-export') {
    await startSummitExport();
    return;
  }
  if (action === 'refresh-export-jobs') {
    await refreshExportJobs();
    return;
  }
  if (action === 'select-export-job') {
    setSelectedExportJobId(target.dataset.id || '');
    return;
  }
  if (action === 'cancel-export') {
    await cancelSelectedExport();
    return;
  }
  if (action === 'authorize-teardown') {
    await requestTeardownAuthorization();
    return;
  }
  if (action === 'consume-authorization') {
    await consumeTeardownAuthorization();
    return;
  }
  if (action === 'open-verified-pdf') {
    await openVerifiedArtifact('pdf');
    return;
  }
  if (action === 'download-verified-bundle') {
    await openVerifiedArtifact('bundle');
    return;
  }
  if (action === 'destroy-phrase') return;
  if (action === 'toggle-teardown') {
    state.teardownArmed = target.checked;
    render();
    return;
  }
  if (action === 'destroy-instance') {
    state.destroyed = true;
    render();
    return;
  }
  if (action === 'back-to-summit') {
    state.view = 'trail';
    state.activePhase = 'summit';
    pushPath(phasePath(state.engagementId, 'summit'));
    render();
    return;
  }
  if (action === 'open-pdf') {
    const previewWindow = window.open('', '_blank', 'noopener');
    if (!previewWindow) {
      state.reportError = 'Unable to open the PDF preview';
      render();
      return;
    }

    try {
      state.reportError = '';
      const response = await fetch(reportPdfPath(state.engagementId), { headers: authHeaders(state.token, newRequestId()), cache: 'no-store' });
      if (!response.ok) throw new Error(await readProblem(response));
      const pdfBlob = await response.blob();
      const previewUrl = URL.createObjectURL(pdfBlob);
      previewWindow.location.href = previewUrl;
      previewWindow.addEventListener('load', () => URL.revokeObjectURL(previewUrl), { once: true });
    } catch (error) {
      previewWindow.close();
      state.reportError = error instanceof Error ? error.message : 'Unable to open the PDF preview';
      render();
    }
    return;
  }
  if (action === 'go-prev') {
    event.preventDefault();
    navigateToPhase(waypointOrder[Math.max(0, waypointOrder.indexOf(state.activePhase) - 1)]);
    return;
  }
  if (action === 'go-next') {
    event.preventDefault();
    if (state.activePhase === 'summit') {
      navigateToReport();
    } else {
      navigateToPhase(waypointOrder[Math.min(waypointOrder.length - 1, waypointOrder.indexOf(state.activePhase) + 1)]);
    }
    return;
  }
}

function handleInput(event) {
  const target = event.target.closest('[data-action]');
  if (!target) return;
  const action = target.dataset.action;
  if (action === 'update-token') {
    setToken(target.value);
    return;
  }
  if (action === 'guide-search') {
    updateGuideQuery(target.value);
    return;
  }
  if (action === 'atlas-search') {
    state.atlasQuery = target.value;
    drawAtlasRows();
    return;
  }
  if (action === 'entity-search') {
    updateEntityQuery(target.value);
    return;
  }
  if (action === 'actor-query') {
    state.actorQuery = target.value;
    render();
    return;
  }
  if (action === 'provision-draft') {
    const field = target.dataset.field || '';
    if (!field) return;
    state.provisionDraft[field] = target.value;
    if (field === 'kind') {
      state.provisionDraft.kind = target.value;
      if (target.value === 'human') state.provisionDraft.authorizedBy = '';
      syncProvisionAuthorizer();
    }
    if (field === 'authorizedBy' || field === 'kind') syncProvisionAuthorizer();
    render();
    return;
  }
  if (action === 'claim-resolution-draft') {
    const field = target.dataset.field || '';
    if (!field) return;
    state.claimResolutionDraft[field] = target.value;
    render();
    return;
  }
  if (action === 'setup-draft') {
    const field = target.dataset.field || '';
    if (!field) return;
    state.setupDraft[field] = target.value;
    const submit = root.querySelector('button[data-action="setup-submit"]');
    if (submit) submit.disabled = state.setupStatus === 'saving' || !setupFormReady();
    return;
  }
  if (action === 'destroy-phrase') {
    state.destroyPhrase = target.value;
    return;
  }
  if (action === 'observation-select') {
    state.selectedObservationId = target.value;
    render();
    return;
  }
}

function handleChange(event) {
  const target = event.target.closest('[data-action]');
  if (!target) return;
  if (target.dataset.action === 'update-token') setToken(target.value);
  if (target.dataset.action === 'toggle-teardown') {
    state.teardownArmed = target.checked;
    render();
  }
  if (target.dataset.action === 'setup-demo') {
    // Reflect the toggle without re-rendering, so any text the operator is
    // mid-way through typing in the form keeps its focus and selection.
    state.setupDraft.demo = target.checked;
  }
  if (target.dataset.action === 'provision-draft' || target.dataset.action === 'claim-resolution-draft') {
    handleInput(event);
  }
}

function handlePopState() {
  const route = routeFromPath(window.location.pathname);
  state.view = route.view;
  state.activePhase = route.phase;
  render();
}

function initializeSelectionFromData() {
  if (!state.selectedActionId && state.actions.length) {
    state.selectedActionId = state.actions.find((action) => action.capture.phase === 'attacks')?.id || state.actions[0].id;
  }
  if (!state.selectedEntityId && state.entities.length) state.selectedEntityId = state.entities[0].id;
  if (!state.selectedFindingId && state.findings.length) state.selectedFindingId = state.findings[0].id;
  if (!state.selectedActorId && state.actors.length) state.selectedActorId = state.actors[0].actor.id;
  if (!state.selectedClaimId && state.claims.length) state.selectedClaimId = state.claims[0].id;
  if (!state.selectedExportJobId && state.exportJobs.length) state.selectedExportJobId = state.exportJobs[0].id;
  syncProvisionAuthorizer();
  const action = selectedAction();
  if (action && !state.findingPromotionDraft.title) {
    state.findingPromotionDraft = {
      title: `${action.capture.command} on ${action.capture.target.value}`,
      severity: 'medium',
      remediation: '',
      status: 'open',
    };
  }
  const finding = selectedFinding();
  if (finding) {
    state.findingDraft = { title: finding.title, severity: String(finding.severity || 'medium').toLowerCase(), remediation: finding.remediation, status: finding.status };
  }
}

async function boot() {
  state.theme = getInitialTheme();
  state.engagementId = getInitialEngagementId();
  const route = routeFromPath(window.location.pathname);
  state.view = route.view;
  state.activePhase = route.phase;
  state.token = (() => {
    try {
      return window.localStorage.getItem('waypoint-token') || 'demo-token';
    } catch {
      return 'demo-token';
    }
  })();
  document.documentElement.dataset.theme = state.theme;
  document.documentElement.dataset.view = state.view;
  window.addEventListener('popstate', handlePopState);
  root.addEventListener('click', handleClick);
  root.addEventListener('submit', handleSubmit);
  root.addEventListener('input', handleInput);
  root.addEventListener('change', handleChange);

  await loadSetupState();
  if (state.setupRequired) {
    state.view = 'setup';
    if (window.location.pathname !== '/setup') pushPath('/setup');
    render();
    return;
  }

  render();
  await refreshEverything();
  initializeSelectionFromData();
  render();
}

async function loadSetupState() {
  try {
    const response = await fetch('/api/v1/runtime', { headers: { Accept: 'application/json' }, cache: 'no-store' });
    if (!response.ok) return;
    const runtime = await response.json();
    state.setupRequired = Boolean(runtime?.setup?.required);
    state.setupCodeRequired = Boolean(runtime?.setup?.codeRequired);
  } catch {
    // A runtime probe failure should not block the normal app path.
  }
}

void boot();
