const sourceHash = "d0bd3c3eb0e5d959aeb877ee559ae467466880884a91f209d40ea2d4e6c9653d";
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
  const match = pathname.match(/^\/engagements\/[^/]+\/(recon|attacks|findings|summit)\/?$/);
  return match ? { view: 'trail', phase: match[1] } : { view: 'trail', phase: 'attacks' };
}

function phasePath(engagementId, phase) {
  return `/engagements/${engagementId}/${phase}`;
}

function reportPath(engagementId) {
  return `/engagements/${engagementId}/summit/report`;
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
    tick();
    state.exportPollTimer = window.setInterval(tick, 3500);
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
          <section class="report-section"><h2>Scope</h2><ul>${snapshot.scope.map((item) => `<li>${escapeHtml(item)}</li>`).join('')}</ul></section>
          <section class="report-section"><h2>Methodology</h2><ul>${snapshot.methodology.map((item) => `<li>${escapeHtml(item)}</li>`).join('')}</ul></section>
          <section class="report-section">
            <h2>Findings</h2>
            <div class="report-grid">
              ${snapshot.findings.map((finding) => `
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
              ${snapshot.evidence.map((item) => `
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
          <section class="report-section"><h2>Attribution</h2><div class="report-grid">${snapshot.attribution.map((section) => `<article class="report-card"><h3>${escapeHtml(section.title)}</h3><ul>${section.items.map((item) => `<li>${escapeHtml(item)}</li>`).join('')}</ul></article>`).join('')}</div></section>
          <section class="report-section"><h2>Known capture gaps</h2><ul>${snapshot.knownCaptureGaps.map((item) => `<li>${escapeHtml(item)}</li>`).join('')}</ul></section>
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
  document.title = state.view === 'report' ? 'Waypoint — report snapshot' : `Waypoint — ${phaseNames[state.activePhase]}`;
  root.innerHTML = state.view === 'report' ? renderReportView() : renderTrailMap();
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
  if (action === 'refresh-entities') {
    await refreshEntities();
    return;
  }
  if (action === 'refresh-actions') {
    await refreshActions();
    return;
  }
  if (action === 'guide-search') return;
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
