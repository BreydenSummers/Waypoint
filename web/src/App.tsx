import { useEffect, useMemo, useRef, useState } from 'react';

type ThemeMode = 'light' | 'dark';
type PhaseId = 'recon' | 'attacks' | 'findings' | 'summit';
type RouteView = 'trail' | 'report';
type ResourceStatus = 'idle' | 'loading' | 'ready' | 'error';
type WaypointState = 'completed' | 'current' | 'fog';

type ContractPage<T> = {
  contractVersion: string;
  items: T[];
  page: { hasMore: boolean; nextCursor?: string; highWaterCursor?: string | null };
};

type ContractProblem = {
  code?: string;
  title?: string;
  detail?: string;
  status?: number;
  retryable?: boolean;
  fieldErrors?: Array<{ pointer: string; code: string; message: string }>;
};

type AuditActor = {
  id: string;
  kind: 'human' | 'ai_agent';
  handle: string;
  role: string;
  agentName?: string;
  model?: string;
  version?: string;
  authorizedBy?: string;
};

type AuditEvent = {
  contractVersion: string;
  id: string;
  type: string;
  engagementId: string;
  actor: AuditActor;
  occurredAt: string;
  origin: { kind: string; service?: string };
  subject: { type: string; id: string; revision?: number };
  requestId: string;
  correlationId: string;
  data: any;
};

type ActionItem = {
  contractVersion: string;
  id: string;
  engagementId: string;
  actor: AuditActor;
  capture: {
    contractVersion: string;
    captureId: string;
    sourceAgent: {
      id: string;
      kind: string;
      name: string;
      version: string;
      platform: { os: string; arch: string };
    };
    phase: PhaseId;
    initiatedBy: string;
    command: string;
    argv: string[];
    cwd: string;
    target: { kind: string; value: string; port?: number | null; transport?: string };
    timing: { startedAt: string; endedAt?: string; durationMs: number };
    execution: { status: string; exitCode?: number | null; signal?: string; failureCode?: string };
    network: {
      execHost: { address: string; method: string; confidence: string; interface?: string };
      egress: { mode: string; status: string; address?: string; observedAt?: string };
      pivotChain: Array<{ type: string; host?: string; port?: number; label?: string }>;
    };
    evidence: {
      stdout: { mediaType: string; byteLength: number; sha256: string };
      stderr: { mediaType: string; byteLength: number; sha256: string };
    };
    parsing: {
      status: string;
      plugin?: { id: string; version: string; artifactSha256?: string; match?: { binary?: string; specificity?: number; reason?: string } };
      result?: { schemaId?: string; schemaVersion?: string; extracted?: any; entities?: any[] };
    };
    decisionContext?: any;
  };
  receivedAt: string;
  clockSkew?: { status: string; offsetMs: number } | null;
  evidenceReferences: {
    stdout: { id: string; role: string; mediaType: string; byteLength: number; sha256: string; downloadPath: string };
    stderr: { id: string; role: string; mediaType: string; byteLength: number; sha256: string; downloadPath: string };
  };
  auditEventCursor: string;
};

type EntityObservation = {
  id: string;
  entityId?: string;
  kind?: string;
  sourceActionId?: string;
  claimStatus: string;
  identifiers?: Array<{ type: string; value: string }>;
  attributes?: Record<string, any>;
  observedAt: string;
};

type EntityItem = {
  contractVersion: string;
  id: string;
  engagementId: string;
  kind: string;
  identifiers: Array<{ type: string; value: string }>;
  attributes: Record<string, any>;
  observations: EntityObservation[];
  firstSeen: string;
  lastSeen: string;
  revision: number;
};

type FindingItem = {
  contractVersion: string;
  id: string;
  engagementId: string;
  title: string;
  severity: string;
  affectedEntityIds: string[];
  evidenceActionIds: string[];
  remediation: string;
  status: string;
  promotedBy: string;
  promotedAt: string;
  revision: number;
  createdAt: string;
  updatedAt: string;
};

type ActorLifecycleRecord = {
  contractVersion: string;
  engagementId: string;
  actor: AuditActor;
  status: 'active' | 'revoked';
  credentialVersion: number;
  createdAt: string;
  createdBy: string;
  lastRotatedAt?: string;
  lastRotatedBy?: string;
  revokedAt?: string;
  revokedBy?: string;
  revision: number;
};

type ActorCredentialResponse = {
  contractVersion: string;
  actorRecord: ActorLifecycleRecord;
  token: string;
  issuedAt: string;
};

type OutOfBandClaimItem = {
  contractVersion: string;
  id: string;
  engagementId: string;
  claimKind: 'entity' | 'result';
  claimedSubjectId: string;
  sourceActionId?: string | null;
  detectionBoundary: string;
  reason: string;
  status: 'pending' | 'linked' | 'dismissed';
  observedAt: string;
  observedBy: AuditActor;
  resolvedAt?: string;
  resolvedBy?: AuditActor;
  notes?: string;
  revision: number;
};

type FindingRevision = AuditEvent;

type EvidenceItem = {
  contractVersion: string;
  id: string;
  engagementId: string;
  actionId: string;
  role: string;
  mediaType: string;
  byteLength: number;
  sha256: string;
  createdAt: string;
  contentPath: string;
};

type ReportSnapshot = {
  version: string;
  title: string;
  engagement: string;
  cutoff: string;
  scope: string[];
  methodology: string[];
  findings: Array<{
    title: string;
    severity: string;
    evidence: string[];
    remediation: string;
    status?: string;
    promotedBy?: string;
    promotedAt?: string;
  }>;
  evidence: Array<{
    label: string;
    command: string;
    target: string;
    actor: string;
    host: string;
    egress: string;
    initiatedBy: string;
    parseStatus: string;
    rawStdout: string;
    rawStderr: string;
    attribution: string;
  }>;
  attribution: Array<{ title: string; items: string[] }>;
  knownCaptureGaps: string[];
};

type GuideNote = {
  id: string;
  phase: PhaseId;
  title: string;
  what: string;
  when: string;
  risks: string;
};

const apiVersion = '1.0.0';
const defaultEngagementId = 'demo';
const waypointOrder: PhaseId[] = ['recon', 'attacks', 'findings', 'summit'];
const phaseNames: Record<PhaseId, string> = {
  recon: 'Recon',
  attacks: 'Attacks',
  findings: 'Findings',
  summit: 'Summit',
};

const guideNotes: GuideNote[] = [
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

function getInitialTheme(): ThemeMode {
  if (typeof window === 'undefined') return 'light';
  try {
    const stored = window.localStorage.getItem('waypoint-theme');
    if (stored === 'light' || stored === 'dark') return stored;
  } catch {
    // ignore
  }
  return window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light';
}

function getInitialEngagementId(): string {
  if (typeof window === 'undefined') return defaultEngagementId;
  const match = window.location.pathname.match(/^\/engagements\/([^/]+)/);
  return match?.[1] || defaultEngagementId;
}

function routeFromPath(pathname: string): { view: RouteView; phase: PhaseId } {
  if (/^\/engagements\/[^/]+\/summit\/report\/?$/.test(pathname)) {
    return { view: 'report', phase: 'summit' };
  }
  const match = pathname.match(/^\/engagements\/[^/]+\/(recon|attacks|findings|summit)\/?$/);
  return match ? { view: 'trail', phase: match[1] as PhaseId } : { view: 'trail', phase: 'attacks' };
}

function getInitialRoute(): { view: RouteView; phase: PhaseId } {
  if (typeof window === 'undefined') return { view: 'trail', phase: 'attacks' };
  return routeFromPath(window.location.pathname);
}

function phasePath(engagementId: string, phase: PhaseId) {
  return `/engagements/${engagementId}/${phase}`;
}

function reportPath(engagementId: string) {
  return `/engagements/${engagementId}/summit/report`;
}

function reportJsonPath(engagementId: string) {
  return `/api/v1${reportPath(engagementId)}.json`;
}

function reportPdfPath(engagementId: string) {
  return `${reportPath(engagementId)}.pdf`;
}

function pushPath(path: string) {
  window.history.pushState({}, '', path);
}

function navigateToPhase(engagementId: string, phase: PhaseId) {
  pushPath(phasePath(engagementId, phase));
}

function navigateToReport(engagementId: string) {
  pushPath(reportPath(engagementId));
}

function authHeaders(token: string, requestId?: string): HeadersInit {
  const headers: Record<string, string> = {
    Authorization: `Bearer ${token}`,
    'Waypoint-Contract-Version': apiVersion,
    Accept: 'application/json',
  };
  if (requestId) headers['X-Request-ID'] = requestId;
  return headers;
}

function revisionHeader(revision: number) {
  return `"${revision}"`;
}

function newRequestId() {
  return typeof crypto !== 'undefined' && 'randomUUID' in crypto ? crypto.randomUUID() : `${Date.now()}-${Math.random().toString(16).slice(2)}`;
}

function isRecord(value: unknown): value is Record<string, any> {
  return typeof value === 'object' && value !== null;
}

async function readProblem(response: Response): Promise<string> {
  try {
    const body = (await response.json()) as ContractProblem;
    const parts = [body.detail || body.title || `Request failed (${response.status})`];
    if (body.fieldErrors?.length) {
      parts.push(body.fieldErrors.map((field) => `${field.pointer}: ${field.message}`).join(' · '));
    }
    return parts.filter(Boolean).join(' — ');
  } catch {
    return `Request failed (${response.status})`;
  }
}

async function apiJson<T>(path: string, token: string, signal?: AbortSignal): Promise<T> {
  const response = await fetch(path, { headers: authHeaders(token, newRequestId()), cache: 'no-store', signal });
  if (!response.ok) throw new Error(await readProblem(response));
  return (await response.json()) as T;
}

async function apiText(path: string, token: string, signal?: AbortSignal): Promise<string> {
  const response = await fetch(path, {
    headers: { Authorization: `Bearer ${token}`, 'Waypoint-Contract-Version': apiVersion, Accept: '*/*' },
    cache: 'no-store',
    signal,
  });
  if (!response.ok) throw new Error(await readProblem(response));
  return await response.text();
}

async function sha256Hex(value: string | Uint8Array): Promise<string> {
  const bytes = typeof value === 'string' ? new TextEncoder().encode(value) : value;
  const digest = await crypto.subtle.digest('SHA-256', bytes);
  return Array.from(new Uint8Array(digest), (byte) => byte.toString(16).padStart(2, '0')).join('');
}

async function readStreamBytes(response: Response, signal: AbortSignal, onProgress: (loaded: number, total: number) => void): Promise<Uint8Array> {
  const total = Number(response.headers.get('content-length') || '0');
  if (!response.body?.getReader) {
    const bytes = new Uint8Array(await response.arrayBuffer());
    onProgress(bytes.length, bytes.length);
    return bytes;
  }
  const reader = response.body.getReader();
  const chunks: Uint8Array[] = [];
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

function stateForIndex(index: number, activeIndex: number): WaypointState {
  if (index < activeIndex) return 'completed';
  if (index === activeIndex) return 'current';
  return 'fog';
}

function shortStateLabel(state: WaypointState) {
  if (state === 'completed') return 'Done';
  if (state === 'current') return 'Here';
  return 'Fog';
}

function stateLabel(state: WaypointState) {
  return state === 'completed' ? 'completed' : state === 'current' ? 'current' : 'fog';
}

function compactJson(value: any) {
  try {
    return JSON.stringify(value, null, 2);
  } catch {
    return String(value ?? '');
  }
}

function formatTime(iso: string) {
  const date = new Date(iso);
  return Number.isNaN(date.getTime()) ? iso : date.toLocaleString();
}

function phaseSummary(phase: PhaseId) {
  return {
    recon: 'Collect signals, preserve provenance, and merge only when the evidence is steady.',
    attacks: 'Every attempt stays attributed, searchable, and linkable to its raw evidence.',
    findings: 'Only confirmed results become findings; conflicts and revisions stay visible.',
    summit: 'Verify the snapshot, freeze the bundle, and keep teardown guarded by the receipt.',
  }[phase];
}

function summaryLineForAction(action: ActionItem) {
  return `${action.capture.command} · ${action.capture.target.value}`;
}

function summaryLineForEntity(entity: EntityItem) {
  const label = entity.identifiers.map((identifier) => `${identifier.type}:${identifier.value}`).join(' · ');
  return label || entity.kind;
}

function summaryLineForFinding(finding: FindingItem) {
  return `${finding.severity} · ${finding.status}`;
}

function buildAuditSummary(event: AuditEvent) {
  const details = isRecord(event.data) ? event.data : {};
  if (event.type === 'alert.notable') {
    return `${details.ruleId || 'notable alert'} · ${details.title || event.subject.id}`;
  }
  if (event.type === 'finding.promoted') {
    return `Finding promoted · ${details.title || event.subject.id}`;
  }
  if (event.type === 'entity.merged') {
    return 'Entity merged';
  }
  if (event.type === 'entity.split') {
    return 'Entity split';
  }
  if (event.type === 'capture.accepted') {
    return `${details.phase || event.subject.type} · ${details.parseStatus || 'captured'}`;
  }
  if (event.type === 'actor.provisioned') {
    return `Actor provisioned · ${details.actorId || event.subject.id}`;
  }
  if (event.type === 'actor.credential-rotated') {
    return `Credential rotated · v${details.credentialVersion || event.subject.revision || '?'}`;
  }
  if (event.type === 'actor.revoked') {
    return `Actor revoked · ${details.actorId || event.subject.id}`;
  }
  if (event.type === 'out-of-band.flagged') {
    return `Claim flagged · ${details.claimKind || event.subject.id}`;
  }
  if (event.type === 'out-of-band.resolved') {
    return `Claim ${details.resolution || 'resolved'} · ${details.claimId || event.subject.id}`;
  }
  return compactJson(details).slice(0, 120);
}

function isProblemLike(value: unknown): value is ContractProblem {
  return isRecord(value) && ('detail' in value || 'title' in value || 'code' in value);
}

function assertReportSnapshot(value: unknown): asserts value is ReportSnapshot {
  if (!isRecord(value)) throw new Error('report snapshot was not an object');
  for (const key of ['version', 'title', 'engagement', 'cutoff']) {
    if (typeof value[key] !== 'string') throw new Error(`report snapshot missing ${key}`);
  }
  for (const key of ['scope', 'methodology', 'findings', 'evidence', 'attribution', 'knownCaptureGaps']) {
    if (!Array.isArray(value[key])) throw new Error(`report snapshot missing ${key}`);
  }
}

async function parseSseStream(
  response: Response,
  signal: AbortSignal,
  onEvent: (event: { id?: string; event: string; data: string }) => void,
): Promise<void> {
  if (!response.body) return;
  const reader = response.body.getReader();
  const decoder = new TextDecoder();
  let buffer = '';
  let current = { id: '', event: 'message', data: '' };

  const dispatch = () => {
    if (current.data !== '') {
      onEvent({ id: current.id || undefined, event: current.event || 'message', data: current.data.replace(/\n$/, '') });
    }
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

function StatusPill({ status, children }: { status: string; children: any }) {
  return <span className={`status-pill ${status}`.trim()}>{children}</span>;
}

function EmptyState({ title, detail }: { title: string; detail: string }) {
  return (
    <div className="finding-empty">
      <strong>{title}</strong>
      <p>{detail}</p>
    </div>
  );
}

export function App() {
  const initialTheme = getInitialTheme();
  const initialRoute = getInitialRoute();
  const initialEngagementId = getInitialEngagementId();

  const [theme, setTheme] = useState<ThemeMode>(initialTheme);
  const [engagementId] = useState(initialEngagementId);
  const [view, setView] = useState<RouteView>(initialRoute.view);
  const [activePhase, setActivePhase] = useState<PhaseId>(initialRoute.phase);
  const activeId = activePhase;
  const [token, setToken] = useState(() => {
    if (typeof window === 'undefined') return 'demo-token';
    return window.localStorage.getItem('waypoint-token') || 'demo-token';
  });

  const [auditStatus, setAuditStatus] = useState<ResourceStatus>('loading');
  const [actionsStatus, setActionsStatus] = useState<ResourceStatus>('loading');
  const [entitiesStatus, setEntitiesStatus] = useState<ResourceStatus>('loading');
  const [findingsStatus, setFindingsStatus] = useState<ResourceStatus>('loading');
  const [actorsStatus, setActorsStatus] = useState<ResourceStatus>('loading');
  const [claimsStatus, setClaimsStatus] = useState<ResourceStatus>('loading');
  const [reportStatus, setReportStatus] = useState<ResourceStatus>('idle');

  const [auditError, setAuditError] = useState('');
  const [actionsError, setActionsError] = useState('');
  const [entitiesError, setEntitiesError] = useState('');
  const [findingsError, setFindingsError] = useState('');
  const [actorsError, setActorsError] = useState('');
  const [claimsError, setClaimsError] = useState('');
  const [reportError, setReportError] = useState('');

  const [auditEvents, setAuditEvents] = useState<AuditEvent[]>([]);
  const [actions, setActions] = useState<ActionItem[]>([]);
  const [guideQuery, setGuideQuery] = useState('');
  const [entities, setEntities] = useState<EntityItem[]>([]);
  const [findings, setFindings] = useState<FindingItem[]>([]);
  const [actors, setActors] = useState<ActorLifecycleRecord[]>([]);
  const [claims, setClaims] = useState<OutOfBandClaimItem[]>([]);
  const [reportSnapshot, setReportSnapshot] = useState<ReportSnapshot | null>(null);
  const [reportRaw, setReportRaw] = useState('');

  const [selectedActionId, setSelectedActionId] = useState('');
  const [selectedEntityId, setSelectedEntityId] = useState('');
  const [mergeSourceId, setMergeSourceId] = useState('');
  const [mergeTargetId, setMergeTargetId] = useState('');
  const [selectedObservationId, setSelectedObservationId] = useState('');
  const [selectedFindingId, setSelectedFindingId] = useState('');
  const [selectedEvidenceId, setSelectedEvidenceId] = useState('');
  const [selectedActorId, setSelectedActorId] = useState('');
  const [selectedClaimId, setSelectedClaimId] = useState('');
  const [selectedEvidence, setSelectedEvidence] = useState<EvidenceItem | null>(null);
  const [selectedEvidenceContent, setSelectedEvidenceContent] = useState('');
  const [selectedEvidenceError, setSelectedEvidenceError] = useState('');

  const [actionPhaseFilter, setActionPhaseFilter] = useState<'all' | PhaseId>('attacks');
  const [entityQuery, setEntityQuery] = useState('');
  const [actorQuery, setActorQuery] = useState('');
  const [provisionDraft, setProvisionDraft] = useState({ kind: 'human' as 'human' | 'ai_agent', handle: '', role: 'operator', agentName: '', model: '', version: '', authorizedBy: '' });
  const [actorConflict, setActorConflict] = useState('');
  const [actorCredential, setActorCredential] = useState<ActorCredentialResponse | null>(null);
  const [claimResolutionDraft, setClaimResolutionDraft] = useState({ resolution: 'linked' as 'linked' | 'dismissed', sourceActionId: '', notes: '' });
  const [claimConflict, setClaimConflict] = useState('');
  const [findingDraft, setFindingDraft] = useState({ title: '', severity: 'medium', remediation: '', status: 'open' });
  const [findingPromotionDraft, setFindingPromotionDraft] = useState({ title: '', severity: 'medium', remediation: '', status: 'open' });
  const [findingConflict, setFindingConflict] = useState('');
  const [entityConflict, setEntityConflict] = useState('');
  const [promotionConflict, setPromotionConflict] = useState('');
  const [summitStatus, setSummitStatus] = useState<'idle' | 'preflight' | 'exporting' | 'verifying' | 'verified' | 'failed' | 'canceled'>('idle');
  const [summitProgress, setSummitProgress] = useState(0);
  const [summitStep, setSummitStep] = useState('Ready to preflight the bundle.');
  const [summitError, setSummitError] = useState('');
  const [summitReceipt, setSummitReceipt] = useState<null | { verifiedAt: string; snapshotHash: string; pdfSha256: string; manifestHash: string; note: string }>(null);
  const [teardownArmed, setTeardownArmed] = useState(false);
  const [destroyPhrase, setDestroyPhrase] = useState('');
  const [destroyed, setDestroyed] = useState(false);

  const streamAbortRef = useRef<AbortController | null>(null);
  const exportAbortRef = useRef<AbortController | null>(null);
  const evidenceAbortRef = useRef<AbortController | null>(null);
  const reportAbortRef = useRef<AbortController | null>(null);
  const streamCursorRef = useRef<string | null>(null);

  const activeIndex = waypointOrder.indexOf(activePhase);
  const waypoints = useMemo(
    () => waypointOrder.map((phase, index) => ({ id: phase, name: phaseNames[phase], path: phasePath(engagementId, phase), state: stateForIndex(index, activeIndex) })),
    [activeIndex, engagementId],
  );
  const activeWaypoint = waypoints[activeIndex] || waypoints[1];
  const notableAlerts = useMemo(
    () => auditEvents.filter((event) => event.type === 'alert.notable').slice(0, 3),
    [auditEvents],
  );
  const traveled = Math.min(activeIndex + 1, waypointOrder.length);

  const visibleNotes = useMemo(() => {
    const query = guideQuery.trim().toLowerCase();
    return guideNotes.filter((note) => {
      const haystack = [note.phase, note.title, note.what, note.when, note.risks].join(' ').toLowerCase();
      if (!query) {
        return note.phase === activePhase;
      }
      return haystack.includes(query);
    });
  }, [activePhase, guideQuery]);
  const filteredActions = useMemo(() => actions.filter((action) => actionPhaseFilter === 'all' || action.capture.phase === actionPhaseFilter), [actions, actionPhaseFilter]);
  const selectedAction = useMemo(() => filteredActions.find((action) => action.id === selectedActionId) || filteredActions[0] || null, [filteredActions, selectedActionId]);
  const filteredEntities = useMemo(() => {
    const q = entityQuery.trim().toLowerCase();
    return entities.filter((entity) => {
      if (!q) return true;
      return [entity.kind, entity.identifiers.map((identifier) => `${identifier.type} ${identifier.value}`).join(' '), compactJson(entity.attributes)]
        .join(' ')
        .toLowerCase()
        .includes(q);
    });
  }, [entities, entityQuery]);
  const selectedEntity = useMemo(() => filteredEntities.find((entity) => entity.id === selectedEntityId) || filteredEntities[0] || null, [filteredEntities, selectedEntityId]);
  const filteredActors = useMemo(() => {
    const q = actorQuery.trim().toLowerCase();
    return actors.filter((actor) => {
      if (!q) return true;
      return [actor.actor.handle, actor.actor.kind, actor.actor.role, actor.status, actor.actor.agentName || '', actor.actor.model || '', actor.actor.version || '', actor.actor.authorizedBy || '', actor.createdBy, actor.revision.toString()]
        .join(' ')
        .toLowerCase()
        .includes(q);
    });
  }, [actors, actorQuery]);
  const selectedActor = useMemo(() => filteredActors.find((actor) => actor.actor.id === selectedActorId) || filteredActors[0] || null, [filteredActors, selectedActorId]);
  const activeHumanActors = useMemo(() => actors.filter((actor) => actor.status === 'active' && actor.actor.kind === 'human'), [actors]);
  useEffect(() => {
    if (provisionDraft.kind !== 'ai_agent' || !activeHumanActors.length) return;
    if (activeHumanActors.some((actor) => actor.actor.id === provisionDraft.authorizedBy)) return;
    setProvisionDraft((draft) => ({ ...draft, authorizedBy: activeHumanActors[0].actor.id }));
  }, [activeHumanActors, provisionDraft.authorizedBy, provisionDraft.kind]);
  const filteredClaims = useMemo(() => claims, [claims]);
  const selectedClaim = useMemo(() => filteredClaims.find((claim) => claim.id === selectedClaimId) || filteredClaims[0] || null, [filteredClaims, selectedClaimId]);
  const selectedFinding = useMemo(() => findings.find((finding) => finding.id === selectedFindingId) || findings[0] || null, [findings, selectedFindingId]);
  const selectedEntityObservations = selectedEntity?.observations || [];
  const selectedEvidenceRef = selectedEvidenceId && selectedAction ? [selectedAction.evidenceReferences.stdout, selectedAction.evidenceReferences.stderr].find((item) => item.id === selectedEvidenceId) || null : null;

  useEffect(() => {
    document.documentElement.dataset.theme = theme;
    try {
      window.localStorage.setItem('waypoint-theme', theme);
    } catch {
      // ignore
    }
  }, [theme]);

  useEffect(() => {
    try {
      window.localStorage.setItem('waypoint-token', token);
    } catch {
      // ignore
    }
  }, [token]);

  useEffect(() => {
    document.documentElement.dataset.view = view;
    document.title = view === 'report' ? 'Waypoint — report snapshot' : `Waypoint — ${phaseNames[activePhase]}`;
  }, [activePhase, view]);

  useEffect(() => {
    const onPopState = () => {
      const route = getInitialRoute();
      setView(route.view);
      setActivePhase(route.phase);
    };
    window.addEventListener('popstate', onPopState);
    return () => window.removeEventListener('popstate', onPopState);
  }, []);

  useEffect(() => {
    if (view !== 'trail') return;
    setDestroyed(false);
    if (activePhase !== 'summit') {
      setTeardownArmed(false);
      setDestroyPhrase('');
      setSummitReceipt(null);
    }
  }, [activePhase, view]);

  useEffect(() => {
    if (selectedAction && selectedAction.id !== selectedActionId) setSelectedActionId(selectedAction.id);
  }, [selectedAction, selectedActionId]);

  useEffect(() => {
    if (selectedEntity && selectedEntity.id !== selectedEntityId) setSelectedEntityId(selectedEntity.id);
  }, [selectedEntity, selectedEntityId]);

  useEffect(() => {
    if (selectedFinding && selectedFinding.id !== selectedFindingId) setSelectedFindingId(selectedFinding.id);
  }, [selectedFinding, selectedFindingId]);

  useEffect(() => {
    if (!actions.length) return;
    const firstAttack = actions.find((action) => action.capture.phase === 'attacks') || actions[0];
    if (!selectedActionId) setSelectedActionId(firstAttack.id);
  }, [actions, selectedActionId]);

  useEffect(() => {
    if (!entities.length || selectedEntityId) return;
    setSelectedEntityId(entities[0].id);
  }, [entities, selectedEntityId]);

  useEffect(() => {
    if (!findings.length || selectedFindingId) return;
    setSelectedFindingId(findings[0].id);
  }, [findings, selectedFindingId]);

  useEffect(() => {
    if (!actors.length || selectedActorId) return;
    setSelectedActorId(actors[0].actor.id);
  }, [actors, selectedActorId]);

  useEffect(() => {
    if (!claims.length || selectedClaimId) return;
    setSelectedClaimId(claims[0].id);
  }, [claims, selectedClaimId]);

  useEffect(() => {
    if (!selectedClaim) return;
    setClaimResolutionDraft({
      resolution: selectedClaim.status === 'dismissed' ? 'dismissed' : 'linked',
      sourceActionId: selectedClaim.sourceActionId || selectedAction?.id || '',
      notes: selectedClaim.notes || '',
    });
  }, [selectedClaim, selectedAction?.id]);

  const refreshAudit = async (signal?: AbortSignal) => {
    setAuditStatus('loading');
    setAuditError('');
    try {
      const page = await apiJson<ContractPage<AuditEvent>>('/api/v1/audit-events?limit=30', token, signal);
      setAuditEvents(page.items);
      streamCursorRef.current = page.page.highWaterCursor || page.items[0]?.id || null;
      setAuditStatus('ready');
    } catch (error) {
      setAuditStatus('error');
      setAuditError(error instanceof Error ? error.message : 'Unable to load journey log');
    }
  };

  const refreshActions = async (signal?: AbortSignal) => {
    setActionsStatus('loading');
    setActionsError('');
    try {
      const page = await apiJson<ContractPage<ActionItem>>('/api/v1/actions?limit=100', token, signal);
      setActions(page.items);
      setActionsStatus('ready');
    } catch (error) {
      setActionsStatus('error');
      setActionsError(error instanceof Error ? error.message : 'Unable to load attacks');
    }
  };

  const refreshEntities = async (signal?: AbortSignal) => {
    setEntitiesStatus('loading');
    setEntityConflict('');
    try {
      const page = await apiJson<ContractPage<EntityItem>>('/api/v1/entities?limit=100', token, signal);
      setEntities(page.items);
      setEntitiesStatus('ready');
    } catch (error) {
      setEntitiesStatus('error');
      setEntityConflict(error instanceof Error ? error.message : 'Unable to load recon');
    }
  };

  const refreshFindings = async (signal?: AbortSignal) => {
    setFindingsStatus('loading');
    setFindingConflict('');
    try {
      const page = await apiJson<ContractPage<FindingItem>>('/api/v1/findings', token, signal);
      setFindings(page.items);
      setFindingsStatus('ready');
    } catch (error) {
      setFindingsStatus('error');
      setFindingConflict(error instanceof Error ? error.message : 'Unable to load findings');
    }
  };

  const refreshActors = async (signal?: AbortSignal) => {
    setActorsStatus('loading');
    setActorConflict('');
    try {
      const page = await apiJson<ContractPage<ActorLifecycleRecord>>('/api/v1/actors?limit=100', token, signal);
      setActors(page.items);
      setActorsStatus('ready');
    } catch (error) {
      setActorsStatus('error');
      setActorConflict(error instanceof Error ? error.message : 'Unable to load actors');
    }
  };

  const refreshClaims = async (signal?: AbortSignal) => {
    setClaimsStatus('loading');
    setClaimConflict('');
    try {
      const page = await apiJson<ContractPage<OutOfBandClaimItem>>('/api/v1/out-of-band-claims?limit=100', token, signal);
      setClaims(page.items);
      setClaimsStatus('ready');
    } catch (error) {
      setClaimsStatus('error');
      setClaimConflict(error instanceof Error ? error.message : 'Unable to load pending claims');
    }
  };

  const refreshReport = async (signal?: AbortSignal) => {
    setReportStatus('loading');
    setReportError('');
    try {
      const raw = await apiText(reportJsonPath(engagementId), token, signal);
      const snapshot = JSON.parse(raw) as unknown;
      assertReportSnapshot(snapshot);
      setReportRaw(raw);
      setReportSnapshot(snapshot);
      setReportStatus('ready');
    } catch (error) {
      setReportStatus('error');
      setReportError(error instanceof Error ? error.message : 'Unable to load report snapshot');
    }
  };

  const refreshEverything = async () => {
    const controller = new AbortController();
    await Promise.allSettled([
      refreshAudit(controller.signal),
      refreshActions(controller.signal),
      refreshEntities(controller.signal),
      refreshFindings(controller.signal),
      refreshActors(controller.signal),
      refreshClaims(controller.signal),
    ]);
    controller.abort();
    if (view === 'report' || activePhase === 'summit') {
      void refreshReport();
    }
  };

  useEffect(() => {
    if (!token) return;
    const controller = new AbortController();
    void (async () => {
      await refreshEverything();
      if (streamAbortRef.current) streamAbortRef.current.abort();
      streamAbortRef.current = controller;
      let cursor = streamCursorRef.current;
      let backoff = 700;
      while (!controller.signal.aborted) {
        try {
          const streamUrl = new URL('/events', window.location.origin);
          if (cursor) streamUrl.searchParams.set('after', cursor);
          const response = await fetch(streamUrl, {
            headers: { Authorization: `Bearer ${token}`, 'Waypoint-Contract-Version': apiVersion, Accept: 'text/event-stream' },
            signal: controller.signal,
            cache: 'no-store',
          });
          if (!response.ok) throw new Error(await readProblem(response));
          setAuditStatus('ready');
          await parseSseStream(response, controller.signal, (event) => {
            if (event.id) cursor = event.id;
            if (!event.data) return;
            let parsed: any = event.data;
            try {
              parsed = JSON.parse(event.data);
            } catch {
              // leave as text
            }
            const auditEvent: AuditEvent = {
              contractVersion: apiVersion,
              id: event.id || newRequestId(),
              type: event.event,
              engagementId,
              actor: isRecord(parsed.actor) ? parsed.actor : { id: '', kind: 'human', handle: '', role: '' },
              occurredAt: typeof parsed.occurredAt === 'string' ? parsed.occurredAt : new Date().toISOString(),
              origin: isRecord(parsed.origin) ? parsed.origin : { kind: 'rest' },
              subject: isRecord(parsed.subject) ? parsed.subject : { type: 'event', id: '' },
              requestId: typeof parsed.requestId === 'string' ? parsed.requestId : '',
              correlationId: typeof parsed.correlationId === 'string' ? parsed.correlationId : '',
              data: parsed.data ?? parsed,
            };
            setAuditEvents((current) => {
              if (current.some((item) => item.id === auditEvent.id)) return current;
              return [auditEvent, ...current].slice(0, 30);
            });
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
            } else if (event.event.startsWith('out-of-band.')) {
              void refreshClaims();
            }
          });
          backoff = 700;
        } catch (error) {
          if (controller.signal.aborted) return;
          setAuditStatus('error');
          setAuditError(error instanceof Error ? error.message : 'Journey log stream disconnected');
          await new Promise((resolve) => window.setTimeout(resolve, backoff));
          backoff = Math.min(backoff * 2, 6000);
        }
      }
    })();
    return () => controller.abort();
  }, [activePhase, engagementId, token, view]);

  useEffect(() => {
    if (view !== 'report') return;
    if (reportStatus === 'ready' || reportStatus === 'loading') return;
    void refreshReport();
  }, [view]);

  useEffect(() => {
    if (!selectedEvidenceId || !selectedEvidenceRef) return;
    const controller = new AbortController();
    evidenceAbortRef.current?.abort();
    evidenceAbortRef.current = controller;
    setSelectedEvidence(null);
    setSelectedEvidenceContent('');
    setSelectedEvidenceError('');
    void (async () => {
      try {
        const evidence = await apiJson<EvidenceItem>(selectedEvidenceRef.downloadPath.replace('/content', ''), token, controller.signal);
        setSelectedEvidence(evidence);
        if (evidence.mediaType.startsWith('text/') || evidence.mediaType.includes('json') || evidence.mediaType.includes('xml')) {
          const content = await apiText(evidence.contentPath, token, controller.signal);
          setSelectedEvidenceContent(content);
        } else {
          setSelectedEvidenceContent('');
        }
      } catch (error) {
        if (!controller.signal.aborted) setSelectedEvidenceError(error instanceof Error ? error.message : 'Unable to load evidence');
      }
    })();
    return () => controller.abort();
  }, [selectedEvidenceId, selectedEvidenceRef, token]);

  useEffect(() => {
    if (!selectedFinding) return;
    setFindingDraft({ title: selectedFinding.title, severity: selectedFinding.severity.toLowerCase(), remediation: selectedFinding.remediation, status: selectedFinding.status });
  }, [selectedFinding]);

  useEffect(() => {
    if (!selectedAction) return;
    setFindingPromotionDraft((current) =>
      current.title ? current : {
        title: `${selectedAction.capture.command} on ${selectedAction.capture.target.value}`,
        severity: 'medium',
        remediation: '',
        status: 'open',
      },
    );
  }, [selectedAction]);

  const startSummitExport = async () => {
    exportAbortRef.current?.abort();
    const controller = new AbortController();
    exportAbortRef.current = controller;
    setSummitReceipt(null);
    setSummitError('');
    setSummitStatus('preflight');
    setSummitProgress(10);
    setSummitStep('Freezing the live report snapshot.');
    try {
      const raw = reportRaw || (await apiText(reportJsonPath(engagementId), token, controller.signal));
      const parsed = reportSnapshot || JSON.parse(raw);
      assertReportSnapshot(parsed);
      setSummitProgress(28);
      setSummitStep('Streaming the PDF artifact.');
      setSummitStatus('exporting');
      const response = await fetch(reportPdfPath(engagementId), { headers: authHeaders(token, newRequestId()), cache: 'no-store', signal: controller.signal });
      if (!response.ok) throw new Error(await readProblem(response));
      const pdfBytes = await readStreamBytes(response, controller.signal, (loaded, total) => {
        const ratio = total ? loaded / total : 0.5;
        setSummitProgress(Math.min(84, 30 + Math.round(ratio * 54)));
        setSummitStep(`Streaming the PDF artifact (${loaded.toLocaleString()} bytes).`);
      });
      if (pdfBytes[0] !== 0x25 || pdfBytes[1] !== 0x50 || pdfBytes[2] !== 0x44 || pdfBytes[3] !== 0x46) {
        throw new Error('report PDF did not start with a PDF signature');
      }
      setSummitStatus('verifying');
      setSummitProgress(90);
      setSummitStep('Verifying the hash manifest and signature hook.');
      const receipt = {
        verifiedAt: new Date().toISOString(),
        snapshotHash: await sha256Hex(raw),
        pdfSha256: await sha256Hex(pdfBytes),
        manifestHash: 'hash verified, not signed',
        note: 'Hash verified, not signed. The signature hook remains empty.',
      };
      setSummitReceipt(receipt);
      setSummitStatus('verified');
      setSummitProgress(100);
      setSummitStep('Receipt verified and teardown is now guarded.');
    } catch (error) {
      if (controller.signal.aborted) {
        setSummitStatus('canceled');
        setSummitProgress(0);
        setSummitStep('Export canceled before teardown was armed.');
        setSummitError('Export canceled. The live trail stayed intact.');
      } else {
        setSummitStatus('failed');
        setSummitProgress(0);
        setSummitStep('Recovery needed before the next export can run.');
        setSummitError(error instanceof Error ? error.message : 'Export verification failed');
      }
    } finally {
      exportAbortRef.current = null;
    }
  };

  const promoteSelectedAttack = async () => {
    if (!selectedAction) return;
    setPromotionConflict('');
    try {
      const response = await fetch('/api/v1/findings', {
        method: 'POST',
        headers: { ...authHeaders(token, newRequestId()), 'Content-Type': 'application/json' },
        body: JSON.stringify({
          sourceActionId: selectedAction.id,
          title: findingPromotionDraft.title || `${selectedAction.capture.command} on ${selectedAction.capture.target.value}`,
          severity: findingPromotionDraft.severity,
          affectedEntityIds: selectedEntity ? [selectedEntity.id] : [],
          remediation: findingPromotionDraft.remediation || 'Review the underlying control and record the operator decision.',
          status: findingPromotionDraft.status || 'open',
        }),
      });
      if (!response.ok) throw new Error(await readProblem(response));
      const created = (await response.json()) as FindingItem;
      setFindings((current) => [created, ...current.filter((item) => item.id !== created.id)]);
      setSelectedFindingId(created.id);
      setActivePhase('findings');
      navigateToPhase(engagementId, 'findings');
      setView('trail');
      setPromotionConflict('Promoted into Findings and linked to evidence.');
    } catch (error) {
      setPromotionConflict(error instanceof Error ? error.message : 'Promotion failed');
    }
  };

  const saveFinding = async () => {
    if (!selectedFinding) return;
    setFindingConflict('');
    try {
      const response = await fetch(`/api/v1/findings/${selectedFinding.id}`, {
        method: 'PATCH',
        headers: { ...authHeaders(token, newRequestId()), 'Content-Type': 'application/json' },
        body: JSON.stringify({
          expectedRevision: selectedFinding.revision,
          title: findingDraft.title,
          severity: findingDraft.severity,
          remediation: findingDraft.remediation,
          status: findingDraft.status,
          affectedEntityIds: selectedEntity ? [selectedEntity.id] : selectedFinding.affectedEntityIds,
        }),
      });
      if (!response.ok) throw new Error(await readProblem(response));
      const updated = (await response.json()) as FindingItem;
      setFindings((current) => current.map((item) => (item.id === updated.id ? updated : item)));
      setSelectedFindingId(updated.id);
      setFindingConflict('Finding saved with the authoritative revision.');
    } catch (error) {
      setFindingConflict(error instanceof Error ? error.message : 'Unable to save finding');
    }
  };

  const issueActorCredential = async () => {
    setActorConflict('');
    try {
      const response = await fetch('/api/v1/actors', {
        method: 'POST',
        headers: { ...authHeaders(token, newRequestId()), 'Content-Type': 'application/json' },
        body: JSON.stringify({
          kind: provisionDraft.kind,
          handle: provisionDraft.handle,
          role: provisionDraft.role,
          agentName: provisionDraft.kind === 'ai_agent' ? provisionDraft.agentName : undefined,
          model: provisionDraft.kind === 'ai_agent' ? provisionDraft.model : undefined,
          version: provisionDraft.kind === 'ai_agent' ? provisionDraft.version : undefined,
          authorizedBy: provisionDraft.kind === 'ai_agent' ? provisionDraft.authorizedBy || activeHumanActors[0]?.actor.id || '' : undefined,
        }),
      });
      if (!response.ok) throw new Error(await readProblem(response));
      const created = (await response.json()) as ActorCredentialResponse;
      setActorCredential(created);
      setSelectedActorId(created.actorRecord.actor.id);
      setProvisionDraft((draft) => ({ ...draft, handle: '', agentName: '', model: '', version: '', authorizedBy: '' }));
      void refreshActors();
      setActorConflict(`Issued credential v${created.actorRecord.credentialVersion} for ${created.actorRecord.actor.handle}.`);
    } catch (error) {
      setActorConflict(error instanceof Error ? error.message : 'Unable to issue credential');
    }
  };

  const rotateActorCredential = async (actor: ActorLifecycleRecord) => {
    setActorConflict('');
    try {
      const response = await fetch(`/api/v1/actors/${actor.actor.id}/rotate`, {
        method: 'POST',
        headers: { ...authHeaders(token, newRequestId()), 'If-Match': revisionHeader(actor.revision) },
      });
      if (!response.ok) throw new Error(await readProblem(response));
      const rotated = (await response.json()) as ActorCredentialResponse;
      setActorCredential(rotated);
      setSelectedActorId(rotated.actorRecord.actor.id);
      void refreshActors();
      setActorConflict(`Rotated ${rotated.actorRecord.actor.handle} to credential v${rotated.actorRecord.credentialVersion}.`);
    } catch (error) {
      setActorConflict(error instanceof Error ? error.message : 'Unable to rotate credential');
    }
  };

  const revokeActorCredential = async (actor: ActorLifecycleRecord) => {
    setActorConflict('');
    try {
      const response = await fetch(`/api/v1/actors/${actor.actor.id}/revoke`, {
        method: 'POST',
        headers: { ...authHeaders(token, newRequestId()), 'If-Match': revisionHeader(actor.revision) },
      });
      if (!response.ok) throw new Error(await readProblem(response));
      const revoked = (await response.json()) as ActorLifecycleRecord;
      setSelectedActorId(revoked.actor.id);
      void refreshActors();
      setActorConflict(`Revoked ${revoked.actor.handle} at revision ${revoked.revision}.`);
    } catch (error) {
      setActorConflict(error instanceof Error ? error.message : 'Unable to revoke credential');
    }
  };

  const resolveClaim = async () => {
    if (!selectedClaim) return;
    setClaimConflict('');
    try {
      const sourceActionId = claimResolutionDraft.resolution === 'linked' ? (claimResolutionDraft.sourceActionId || selectedAction?.id || '') : undefined;
      if (claimResolutionDraft.resolution === 'linked' && !sourceActionId) {
        throw new Error("Can't link this claim yet — sourceActionId is required when resolution is linked.");
      }
      const response = await fetch(`/api/v1/out-of-band-claims/${selectedClaim.id}/resolve`, {
        method: 'POST',
        headers: { ...authHeaders(token, newRequestId()), 'Content-Type': 'application/json' },
        body: JSON.stringify({
          resolution: claimResolutionDraft.resolution,
          sourceActionId,
          notes: claimResolutionDraft.notes || undefined,
          expectedRevision: selectedClaim.revision,
        }),
      });
      if (!response.ok) throw new Error(await readProblem(response));
      const updated = (await response.json()) as OutOfBandClaimItem;
      setClaims((current) => current.map((item) => (item.id === updated.id ? updated : item)));
      setSelectedClaimId(updated.id);
      setClaimConflict(`Claim ${updated.id} resolved as ${updated.status}.`);
      void refreshClaims();
    } catch (error) {
      setClaimConflict(error instanceof Error ? error.message : 'Unable to resolve claim');
    }
  };

  const runMergePreview = async () => {
    if (!mergeSourceId || !mergeTargetId || mergeSourceId === mergeTargetId) return;
    setEntityConflict('');
    try {
      const response = await fetch(`/api/v1/entities/${mergeSourceId}/merge-preview?targetEntityId=${mergeTargetId}`, { headers: authHeaders(token, newRequestId()), cache: 'no-store' });
      if (!response.ok) throw new Error(await readProblem(response));
      const preview = await response.json();
      setEntityConflict(`Preview: ${preview.source.id} → ${preview.target?.id || mergeTargetId}`);
    } catch (error) {
      setEntityConflict(error instanceof Error ? error.message : 'Merge preview failed');
    }
  };

  const applyMerge = async () => {
    if (!mergeSourceId || !mergeTargetId || mergeSourceId === mergeTargetId) return;
    const source = entities.find((entity) => entity.id === mergeSourceId);
    const target = entities.find((entity) => entity.id === mergeTargetId);
    try {
      const response = await fetch('/api/v1/entities/merge', {
        method: 'POST',
        headers: { ...authHeaders(token, newRequestId()), 'Content-Type': 'application/json' },
        body: JSON.stringify({
          sourceEntityId: mergeSourceId,
          targetEntityId: mergeTargetId,
          preview: false,
          expectedSourceRevision: source?.revision,
          expectedTargetRevision: target?.revision,
        }),
      });
      if (!response.ok) throw new Error(await readProblem(response));
      const applied = await response.json();
      setEntityConflict(`Merged ${applied.source.id} into ${applied.target?.id || mergeTargetId}.`);
      void refreshEntities();
    } catch (error) {
      setEntityConflict(error instanceof Error ? error.message : 'Merge failed');
    }
  };

  const runSplitPreview = async () => {
    if (!selectedEntity || !selectedObservationId) return;
    setEntityConflict('');
    try {
      const response = await fetch(`/api/v1/entities/${selectedEntity.id}/split-provenance?observationId=${selectedObservationId}`, { headers: authHeaders(token, newRequestId()), cache: 'no-store' });
      if (!response.ok) throw new Error(await readProblem(response));
      const preview = await response.json();
      setEntityConflict(`Preview: split observation ${preview.observationId || selectedObservationId} from ${preview.source.id}.`);
    } catch (error) {
      setEntityConflict(error instanceof Error ? error.message : 'Split preview failed');
    }
  };

  const applySplit = async () => {
    if (!selectedEntity || !selectedObservationId) return;
    try {
      const response = await fetch('/api/v1/entities/split', {
        method: 'POST',
        headers: { ...authHeaders(token, newRequestId()), 'Content-Type': 'application/json' },
        body: JSON.stringify({
          entityId: selectedEntity.id,
          preview: false,
          observationId: selectedObservationId,
          expectedSourceRevision: selectedEntity.revision,
        }),
      });
      if (!response.ok) throw new Error(await readProblem(response));
      const applied = await response.json();
      setEntityConflict(`Split observation ${applied.observationId || selectedObservationId} from ${applied.source.id}.`);
      void refreshEntities();
    } catch (error) {
      setEntityConflict(error instanceof Error ? error.message : 'Split failed');
    }
  };

  const openEvidence = (evidenceId: string) => {
    setSelectedEvidenceId(evidenceId);
  };

  const activeGuideNote = visibleNotes[0] || guideNotes.find((note) => note.phase === activePhase) || guideNotes[0];
  const guidePhase = waypointOrder[Math.min(activeIndex + 1, waypointOrder.length - 1)];
  const trailStatusText = `Trail ${Math.min(activeIndex + 1, waypointOrder.length)} / ${waypointOrder.length} · ${phaseNames[activePhase]}`;

  if (view === 'report') {
    return (
      <main className="app-shell report-shell" aria-label="Frozen report snapshot">
        <section className="report-hero artifact">
          <div>
            <p className="eyebrow">Waypoint · frozen report snapshot</p>
            <h1>{reportSnapshot?.title || 'Loading report snapshot'}</h1>
            <p className="subtitle">{reportSnapshot ? `Version ${reportSnapshot.version} · ${reportSnapshot.engagement} · Cutoff ${reportSnapshot.cutoff}` : 'The report is fetched from the authoritative API.'}</p>
          </div>
          <div className="report-toolbar">
            <button type="button" className="secondary-link" onClick={() => { setView('trail'); setActivePhase('summit'); navigateToPhase(engagementId, 'summit'); }}>
              Back to Summit
            </button>
            <button type="button" className="primary-button" onClick={() => window.open(reportPdfPath(engagementId), '_blank', 'noopener')}>
              Open PDF artifact
            </button>
          </div>
        </section>

        {reportStatus === 'loading' ? <div className="live-banner review"><strong>Loading</strong> Authoritative report snapshot in the pack…</div> : null}
        {reportStatus === 'error' ? <div className="live-banner"><strong>Report error</strong> {reportError}</div> : null}

        {reportSnapshot ? (
          <article className="report-page" aria-label="Printable engagement report">
            <section className="report-section">
              <h2>Scope</h2>
              <ul>{reportSnapshot.scope.map((item) => <li key={item}>{item}</li>)}</ul>
            </section>
            <section className="report-section">
              <h2>Methodology</h2>
              <ul>{reportSnapshot.methodology.map((item) => <li key={item}>{item}</li>)}</ul>
            </section>
            <section className="report-section">
              <h2>Findings</h2>
              <div className="report-grid">
                {reportSnapshot.findings.map((finding) => (
                  <article key={finding.title} className="report-card">
                    <p className="report-badge">{finding.severity}</p>
                    <h3>{finding.title}</h3>
                    <p>{finding.status ? `${finding.status} · ` : ''}{finding.promotedBy ? `Promoted by ${finding.promotedBy}` : ''}</p>
                    <p><strong>Evidence:</strong> {finding.evidence.join(', ')}</p>
                    <p><strong>Remediation:</strong> {finding.remediation}</p>
                  </article>
                ))}
              </div>
            </section>
            <section className="report-section">
              <h2>Evidence</h2>
              <div className="report-grid">
                {reportSnapshot.evidence.map((item) => (
                  <article key={item.label} className="report-card">
                    <p className="report-badge">{item.label}</p>
                    <p><strong>Source:</strong> {item.command}</p>
                    <p><strong>Target:</strong> {item.target}</p>
                    <p><strong>Actor:</strong> {item.actor}</p>
                    <p><strong>Host:</strong> {item.host}</p>
                    <p><strong>Attribution:</strong> {item.attribution}</p>
                    <p className="report-snippet">{item.rawStdout}</p>
                  </article>
                ))}
              </div>
            </section>
            <section className="report-section">
              <h2>Attribution</h2>
              <div className="report-grid">
                {reportSnapshot.attribution.map((section) => (
                  <article key={section.title} className="report-card">
                    <h3>{section.title}</h3>
                    <ul>{section.items.map((item) => <li key={item}>{item}</li>)}</ul>
                  </article>
                ))}
              </div>
            </section>
            <section className="report-section">
              <h2>Known capture gaps</h2>
              <ul>{reportSnapshot.knownCaptureGaps.map((item) => <li key={item}>{item}</li>)}</ul>
            </section>
          </article>
        ) : null}
      </main>
    );
  }

  return (
    <main className="app-shell">
      <header className="masthead">
        <div className="masthead-copy">
          <p className="eyebrow">Waypoint · expedition shell</p>
          <h1>Recon / Attacks / Findings</h1>
          <p className="subtitle">A calm trail map for the audit spine, with live data in the workspaces and the guide keeping pace.</p>
        </div>

        <div className="masthead-actions">
          <div className="theme-switcher" role="group" aria-label="Theme selection">
            <button type="button" aria-pressed={theme === 'light'} className={theme === 'light' ? 'is-active' : ''} onClick={() => setTheme('light')}>Light</button>
            <button type="button" aria-pressed={theme === 'dark'} className={theme === 'dark' ? 'is-active' : ''} onClick={() => setTheme('dark')}>Dark</button>
          </div>

          <label className="field-group">
            <span>Operator token</span>
            <input value={token} onChange={(event) => setToken(event.target.value)} placeholder="Bearer token" aria-label="Operator token" />
          </label>

          <div className="progress-pill" aria-label="Trail progress">{trailStatusText}</div>
          <div className="metrics" aria-label="Engagement progress">
            <div className="metric">
              <span className="metric-label">Traveled</span>
              <strong>{traveled} waypoints</strong>
            </div>
            <div className="metric">
              <span className="metric-label">To summit</span>
              <strong>{Math.max(0, waypointOrder.length - traveled)} left</strong>
            </div>
          </div>
        </div>
      </header>

      <div className="layout">
        <section className="map-column">
          <section className="map-card" aria-label="Engagement trail map">
            <div className="map-stage">
              <svg viewBox="0 0 640 300" role="img" aria-label="Trail map with waypoint buttons">
                <rect width="640" height="300" className="map-terrain" />
                <path d="M60 252 C 132 234, 148 194, 206 182 C 270 168, 286 220, 350 204 C 402 190, 420 148, 472 126 C 516 108, 548 84, 590 60" className="trail-path" />
                <path d="M0 84 Q 138 44, 250 76 T 470 62 T 640 84" className="contours" />
                <path d="M0 136 Q 160 94, 286 124 T 640 112" className="contours" />
                <path d="M0 192 Q 170 160, 332 184 T 640 166" className="contours" />
                <g className="trees" aria-hidden="true">
                  <path d="M92 244 L100 224 L108 244 Z" />
                  <path d="M118 250 L128 228 L138 250 Z" />
                  <path d="M182 96 L190 76 L198 96 Z" />
                  <path d="M510 248 L519 230 L528 248 Z" />
                  <path d="M536 110 L546 88 L556 110 Z" />
                </g>
                {waypoints.map((waypoint, index) => (
                  <g key={waypoint.id} className={`waypoint ${waypoint.state} ${waypoint.id === activeId ? 'is-current' : ''}`.trim()}>
                    <circle cx={[72, 228, 430, 586][index]} cy={[248, 182, 128, 64][index]} r={waypoint.state === 'current' ? 17 : 12} className="waypoint-node" />
                    {waypoint.state === 'completed' ? <path d={`M${[72, 228, 430, 586][index] - 4} ${[248, 182, 128, 64][index]} l3 3 l6 -6`} className="checkmark" /> : null}
                    {waypoint.id === activePhase ? <path d={`M${[72, 228, 430, 586][index]} ${[248, 182, 128, 64][index] - 8} c -4 5 -5 8 -2 11 c -3 0 -5 3 -2 5 c 2 2 7 2 9 0 c 3 -2 1 -5 -2 -5 c 3 -3 1 -7 -3 -11`} className="campfire" /> : null}
                    <text x={[72, 228, 430, 586][index]} y={[248, 182, 128, 64][index] + 28} textAnchor="middle" className="waypoint-label">{waypoint.name}</text>
                    <title>{`${waypoint.name} — ${stateLabel(waypoint.state)}`}</title>
                  </g>
                ))}
              </svg>
              <div className="waypoint-overlay" aria-label="Trail waypoint shortcuts">
                {waypoints.map((waypoint, index) => (
                  <button
                    key={waypoint.id}
                    type="button"
                    className={`waypoint-hitbox ${waypoint.id === activeId ? 'is-active' : ''}`}
                    aria-current={waypoint.id === activeId ? 'step' : undefined}
                    aria-label={`${waypoint.name}, ${stateLabel(waypoint.state)}${waypoint.id === activeId ? ', you are here' : ''}`}
                    onClick={() => {
                      setActivePhase(waypoint.id);
                      window.history.pushState({}, '', waypoint.path);
                    }}
                    style={{ left: `${[72, 228, 430, 586][index] / 6.4}%`, top: `${[248, 182, 128, 64][index] / 3}%` }}
                  />
                ))}
              </div>
            </div>
          </section>

          <section className="workspace-panel" aria-label={`${phaseNames[activePhase]} workspace`}>
            <div className="workspace-header">
              <div>
                <p className="workspace-kicker">Stage {activeIndex + 1} of {waypointOrder.length}</p>
                <h2>{phaseNames[activePhase]} workspace</h2>
              </div>
              <div className="workspace-status-stack">
                <p className="workspace-status">Saved 2 min ago</p>
                <StatusPill status={activePhase === 'summit' ? 'review' : 'neutral'}>{phaseNames[activePhase]}</StatusPill>
              </div>
            </div>

            <p className="workspace-lede">{phaseSummary(activePhase)}</p>

            {activePhase === 'recon' ? (
              <section className="findings-shell" aria-label="Recon workspace">
                <div className="finding-list-panel">
                  <div className="panel-heading">
                    <h2>Authoritative entities</h2>
                    <p>Loaded from the API; provenance drill-in, merge, and split stay live.</p>
                  </div>
                  <div className="guide-tools" style={{ marginTop: 0 }}>
                    <label className="guide-search">
                      <span className="sr-only">Search entities</span>
                      <input value={entityQuery} onChange={(event) => setEntityQuery(event.target.value)} placeholder="Search host, FQDN, MAC, SID…" />
                    </label>
                    <button type="button" className="secondary-link" onClick={() => void refreshEntities()}>Refresh</button>
                  </div>
                  {entitiesStatus === 'loading' ? <div className="finding-empty">Loading entities…</div> : null}
                  {entitiesStatus === 'error' ? <div className="live-banner"><strong>Recon error</strong> {entitiesError}</div> : null}
                  {!filteredEntities.length && entitiesStatus === 'ready' ? <EmptyState title="Nothing discovered yet" detail="No entities were returned for this engagement. Keep the workspace accessible and wait for recon to land." /> : null}
                  <div className="finding-list" style={{ marginTop: '12px' }}>
                    {filteredEntities.map((entity) => (
                      <button key={entity.id} type="button" className={`finding-card ${selectedEntity?.id === entity.id ? 'is-selected' : ''}`.trim()} onClick={() => { setSelectedEntityId(entity.id); setMergeSourceId(entity.id); }}>
                        <div className="finding-card-head">
                          <div>
                            <p className="guide-note-kicker">{entity.kind}</p>
                            <h4>{summaryLineForEntity(entity)}</h4>
                          </div>
                          <StatusPill status="neutral">rev {entity.revision}</StatusPill>
                        </div>
                        <p className="finding-card-summary">First seen {formatTime(entity.firstSeen)} · last seen {formatTime(entity.lastSeen)}</p>
                        <dl className="finding-card-meta">
                          <div>
                            <dt>Observations</dt>
                            <dd>{entity.observations.length}</dd>
                          </div>
                          <div>
                            <dt>Action</dt>
                            <dd>{entity.id === mergeSourceId ? 'Source' : entity.id === mergeTargetId ? 'Target' : 'Set merge slot'}</dd>
                          </div>
                        </dl>
                        <div className="detail-foot">
                          <span className="route-status current">Open provenance</span>
                          <span className="route-status fog">Merge / split ready</span>
                        </div>
                      </button>
                    ))}
                  </div>
                </div>

                <div className="finding-editor-card">
                  {selectedEntity ? (
                    <>
                      <div className="detail-head">
                        <div>
                          <p className="guide-note-kicker">Selected entity</p>
                          <h3>{summaryLineForEntity(selectedEntity)}</h3>
                        </div>
                        <StatusPill status="review">{selectedEntity.kind}</StatusPill>
                      </div>
                      <div className="detail-grid provenance-grid">
                        <div><dt>First seen</dt><dd>{formatTime(selectedEntity.firstSeen)}</dd></div>
                        <div><dt>Last seen</dt><dd>{formatTime(selectedEntity.lastSeen)}</dd></div>
                        <div><dt>Revision</dt><dd>{selectedEntity.revision}</dd></div>
                        <div><dt>Identifiers</dt><dd>{selectedEntity.identifiers.map((identifier) => `${identifier.type}:${identifier.value}`).join(' · ') || 'None'}</dd></div>
                      </div>
                      <div className="evidence-split" style={{ marginTop: '12px' }}>
                        <div className="evidence-box">
                          <div className="evidence-head"><h4>Merge preview</h4><span className="evidence-kind">source / target</span></div>
                          <p>Choose a source and target entity, then preview the authoritative merge before you apply it.</p>
                          <div className="guide-tools">
                            <button type="button" className="secondary-link" onClick={() => { setMergeSourceId(selectedEntity.id); }}>Use as source</button>
                            <button type="button" className="secondary-link" onClick={() => { setMergeTargetId(selectedEntity.id); }}>Use as target</button>
                            <button type="button" className="primary-button" onClick={() => void runMergePreview()} disabled={!mergeSourceId || !mergeTargetId || mergeSourceId === mergeTargetId}>Preview merge</button>
                            <button type="button" className="primary-button" onClick={() => void applyMerge()} disabled={!mergeSourceId || !mergeTargetId || mergeSourceId === mergeTargetId}>Apply merge</button>
                          </div>
                          <p className="detail-foot">Source: {mergeSourceId || 'unset'} · Target: {mergeTargetId || 'unset'}</p>
                        </div>
                        <div className="evidence-box">
                          <div className="evidence-head"><h4>Split provenance</h4><span className="evidence-kind">observation</span></div>
                          <p>Pick a provenance observation to split from the merged entity.</p>
                          <label className="field-group">
                            <span>Observation</span>
                            <select value={selectedObservationId} onChange={(event) => setSelectedObservationId(event.target.value)}>
                              <option value="">Choose an observation</option>
                              {selectedEntityObservations.map((observation) => <option key={observation.id} value={observation.id}>{observation.kind || 'observation'} · {observation.id}</option>)}
                            </select>
                          </label>
                          <div className="guide-tools">
                            <button type="button" className="secondary-link" onClick={() => void runSplitPreview()} disabled={!selectedObservationId}>Preview split</button>
                            <button type="button" className="primary-button" onClick={() => void applySplit()} disabled={!selectedObservationId}>Apply split</button>
                          </div>
                        </div>
                      </div>
                      {selectedEntityObservations.length ? (
                        <section className="finding-trace-panel" style={{ marginTop: '12px' }}>
                          <div className="panel-heading compact">
                            <h3>Provenance drill-in</h3>
                            <p>Each observation stays attributable and linked to the source action.</p>
                          </div>
                          <div className="revision-list">
                            {selectedEntityObservations.map((observation) => (
                              <button key={observation.id} type="button" className={`attack-row-button ${selectedObservationId === observation.id ? 'is-selected' : ''}`.trim()} onClick={() => setSelectedObservationId(observation.id)}>
                                <div className="attack-row-top">
                                  <strong>{observation.kind || 'observation'}</strong>
                                  <span className="attack-row-time">{formatTime(observation.observedAt)}</span>
                                </div>
                                <div className="attack-row-main">
                                  <div className="attack-field"><strong>Action</strong><span>{observation.sourceActionId || 'manual observation'}</span></div>
                                  <div className="attack-field"><strong>Status</strong><span>{observation.claimStatus}</span></div>
                                </div>
                              </button>
                            ))}
                          </div>
                        </section>
                      ) : null}
                      {entityConflict ? <div className="live-banner review"><strong>Recon note</strong> {entityConflict}</div> : null}
                    </>
                  ) : <EmptyState title="No entity selected" detail="Choose an entity to inspect provenance, merge candidates, and split points." />}
                </div>
              </section>
            ) : null}

            {activePhase === 'attacks' ? (
              <section className="attack-toolbar" aria-label="Attack filters and promotion controls">
                <div className="attack-group-switcher" role="group" aria-label="Attack phase filter">
                  {(['all', 'recon', 'attacks'] as const).map((phase) => (
                    <button key={phase} type="button" className={actionPhaseFilter === phase ? 'is-active' : ''} onClick={() => setActionPhaseFilter(phase)}>
                      {phase === 'all' ? 'All' : phaseNames[phase]}
                    </button>
                  ))}
                </div>
                <div className="field-group"><span>Promote</span><button type="button" className="primary-button" onClick={() => { if (selectedAction) { setFindingPromotionDraft({ title: `${selectedAction.capture.command} on ${selectedAction.capture.target.value}`, severity: 'medium', remediation: '', status: 'open' }); setActivePhase('findings'); navigateToPhase(engagementId, 'findings'); } }}>Send to Findings</button></div>
                <div className="field-group"><span>Operator note</span><input value={promotionConflict} onChange={() => setPromotionConflict('')} placeholder="Notable alerts and promotion status appear here." aria-label="Promotion message" /></div>
              </section>
            ) : null}

            {activePhase === 'attacks' ? (
              <section className="attack-shell" aria-label="Attacks workspace">
                <div className="attack-list-column">
                  <div className="attack-group">
                    <div className="attack-group-header">
                      <div>
                        <p className="guide-note-kicker">Authoritative actions</p>
                        <h3>{actionsStatus === 'loading' ? 'Loading attacks…' : 'Captured attempts'}</h3>
                      </div>
                      <button type="button" className="secondary-link" onClick={() => void refreshActions()}>Refresh</button>
                    </div>
                    {actionsStatus === 'error' ? <div className="live-banner"><strong>Attack error</strong> {actionsError}</div> : null}
                    {!filteredActions.length && actionsStatus === 'ready' ? <EmptyState title="No attacks yet" detail="There are no captured attempts for this phase. Keep the workspace open and wait for the next action." /> : null}
                    <div className="attack-group-list">
                      {filteredActions.map((action) => (
                        <div key={action.id} className={`attack-row ${selectedAction?.id === action.id ? 'is-selected' : ''}`.trim()}>
                          <button type="button" className="attack-row-button" onClick={() => { setSelectedActionId(action.id); setSelectedEvidenceId(''); }}>
                            <div className="attack-row-top">
                              <strong>{summaryLineForAction(action)}</strong>
                              <StatusPill status={action.capture.parsing.status === 'parsed' ? 'success' : action.capture.parsing.status === 'needs-plugin' ? 'review' : 'neutral'}>{action.capture.parsing.status}</StatusPill>
                            </div>
                            <div className="attack-row-main">
                              <div className="attack-field"><strong>Actor</strong><span>{action.actor.handle}{action.actor.kind === 'ai_agent' ? ` · ${action.actor.model}` : ''}</span></div>
                              <div className="attack-field"><strong>Target</strong><span>{action.capture.target.kind}:{action.capture.target.value}</span></div>
                              <div className="attack-field"><strong>Host</strong><span>{action.capture.network.execHost.address}</span></div>
                              <div className="attack-field"><strong>Result</strong><span>{action.capture.execution.status} {action.capture.execution.exitCode === 0 ? '· ok' : ''}</span></div>
                            </div>
                            <div className="attack-row-foot">
                              <span>{formatTime(action.capture.timing.startedAt)}</span>
                              <span>{action.capture.parsing.plugin?.id || 'raw evidence'}</span>
                            </div>
                          </button>
                        </div>
                      ))}
                    </div>
                  </div>
                </div>

                <div className="attack-detail">
                  {selectedAction ? (
                    <article className="detail-card" aria-label="Selected attack details">
                      <div className="detail-head">
                        <div>
                          <p className="guide-note-kicker">Selected action</p>
                          <h3>{summaryLineForAction(selectedAction)}</h3>
                        </div>
                        <StatusPill status={selectedAction.capture.parsing.status === 'parsed' ? 'success' : selectedAction.capture.parsing.status === 'needs-plugin' ? 'review' : 'neutral'}>{selectedAction.capture.parsing.status}</StatusPill>
                      </div>
                      <div className="detail-grid provenance-grid">
                        <div><dt>Actor</dt><dd>{selectedAction.actor.handle} {selectedAction.actor.kind === 'ai_agent' ? `· ${selectedAction.actor.model}` : ''}</dd></div>
                        <div><dt>Initiated by</dt><dd>{selectedAction.capture.initiatedBy}</dd></div>
                        <div><dt>Exec host</dt><dd>{selectedAction.capture.network.execHost.address}</dd></div>
                        <div><dt>Egress</dt><dd>{selectedAction.capture.network.egress.address || selectedAction.capture.network.egress.mode || 'off'}</dd></div>
                        <div><dt>Target</dt><dd>{selectedAction.capture.target.kind}:{selectedAction.capture.target.value}</dd></div>
                        <div><dt>Phase</dt><dd>{selectedAction.capture.phase}</dd></div>
                      </div>
                      <div className="evidence-split">
                        {['stdout', 'stderr'].map((role) => {
                          const ref = role === 'stdout' ? selectedAction.evidenceReferences.stdout : selectedAction.evidenceReferences.stderr;
                          return (
                            <button key={ref.id} type="button" className="evidence-box" onClick={() => openEvidence(ref.id)}>
                              <div className="evidence-head">
                                <h4>{role.toUpperCase()} evidence</h4>
                                <span className="evidence-kind">{ref.mediaType}</span>
                              </div>
                              <p>{ref.byteLength} bytes · {ref.sha256.slice(0, 12)}…</p>
                              <pre>{ref.downloadPath}</pre>
                            </button>
                          );
                        })}
                      </div>
                      <div className="detail-foot">
                        <span>{selectedAction.capture.command}</span>
                        <span>{selectedAction.capture.argv.join(' ')}</span>
                      </div>
                      {selectedEvidence ? (
                        <section className="finding-trace-panel">
                          <div className="panel-heading compact">
                            <h3>Evidence drill-in</h3>
                            <p>{selectedEvidence.id} · {selectedEvidence.mediaType}</p>
                          </div>
                          {selectedEvidenceError ? <div className="live-banner"><strong>Evidence error</strong> {selectedEvidenceError}</div> : null}
                          {!selectedEvidenceError ? (
                            <div className="evidence-split">
                              <div className="evidence-box">
                                <div className="evidence-head"><h4>Metadata</h4><span className="evidence-kind">{selectedEvidence.role}</span></div>
                                <p><strong>Action:</strong> {selectedEvidence.actionId}</p>
                                <p><strong>SHA-256:</strong> {selectedEvidence.sha256}</p>
                                <p><strong>Created:</strong> {formatTime(selectedEvidence.createdAt)}</p>
                                <p><strong>Path:</strong> {selectedEvidence.contentPath}</p>
                              </div>
                              <div className="evidence-box">
                                <div className="evidence-head"><h4>Raw content</h4><span className="evidence-kind">{selectedEvidence.byteLength} bytes</span></div>
                                <pre>{selectedEvidenceContent || 'Binary evidence or no text preview available.'}</pre>
                              </div>
                            </div>
                          ) : null}
                        </section>
                      ) : null}
                    </article>
                  ) : <EmptyState title="No attack selected" detail="Pick an action to inspect provenance, evidence, and promotion options." />}
                </div>
              </section>
            ) : null}

            {activePhase === 'findings' ? (
              <section className="findings-shell" aria-label="Findings workspace">
                <div className="finding-list-panel">
                  <div className="panel-heading">
                    <h2>Authoritative findings</h2>
                    <p>Promote from an attack, then keep revisions and conflict states visible.</p>
                  </div>
                  {findingsStatus === 'error' ? <div className="live-banner"><strong>Findings error</strong> {findingsError}</div> : null}
                  {!findings.length && findingsStatus === 'ready' ? <EmptyState title="No findings yet" detail="Nothing has been promoted. Select an attack and send it over when the evidence is ready." /> : null}
                  <div className="finding-list">
                    {findings.map((finding) => (
                      <button key={finding.id} type="button" className={`finding-card ${selectedFinding?.id === finding.id ? 'is-selected' : ''}`.trim()} onClick={() => setSelectedFindingId(finding.id)}>
                        <div className="finding-card-head">
                          <div>
                            <p className="guide-note-kicker">{finding.status}</p>
                            <h4>{finding.title}</h4>
                          </div>
                          <StatusPill status={finding.severity.toLowerCase().includes('high') || finding.severity.toLowerCase().includes('critical') ? 'blocked' : finding.severity.toLowerCase().includes('medium') ? 'review' : 'neutral'}>{finding.severity}</StatusPill>
                        </div>
                        <p className="finding-card-summary">{summaryLineForFinding(finding)}</p>
                        <dl className="finding-card-meta">
                          <div><dt>Evidence</dt><dd>{finding.evidenceActionIds.join(', ') || 'none'}</dd></div>
                          <div><dt>Promoted by</dt><dd>{finding.promotedBy}</dd></div>
                        </dl>
                      </button>
                    ))}
                  </div>
                </div>

                <div className="finding-editor-card">
                  <div className="detail-head">
                    <div>
                      <p className="guide-note-kicker">Promotion</p>
                      <h3>{selectedAction ? `Promote ${selectedAction.capture.command}` : 'Select an attack to promote'}</h3>
                    </div>
                    <StatusPill status="review">authoritative</StatusPill>
                  </div>
                  {selectedAction ? (
                    <>
                      <div className="finding-editor-grid">
                        <label className="finding-field"><span>Title</span><input value={findingPromotionDraft.title} onChange={(event) => setFindingPromotionDraft((draft) => ({ ...draft, title: event.target.value }))} /></label>
                        <label className="finding-field"><span>Severity</span><select value={findingPromotionDraft.severity} onChange={(event) => setFindingPromotionDraft((draft) => ({ ...draft, severity: event.target.value }))}><option value="info">Info</option><option value="low">Low</option><option value="medium">Medium</option><option value="high">High</option><option value="critical">Critical</option></select></label>
                        <label className="finding-field finding-field-wide"><span>Remediation</span><textarea value={findingPromotionDraft.remediation} onChange={(event) => setFindingPromotionDraft((draft) => ({ ...draft, remediation: event.target.value }))} /></label>
                        <label className="finding-field"><span>Status</span><input value={findingPromotionDraft.status} onChange={(event) => setFindingPromotionDraft((draft) => ({ ...draft, status: event.target.value }))} /></label>
                      </div>
                      <div className="finding-actions"><button type="button" className="primary-button" onClick={() => void promoteSelectedAttack()}>Promote selected attack</button></div>
                      {promotionConflict ? <div className="live-banner review"><strong>Promotion</strong> {promotionConflict}</div> : null}
                    </>
                  ) : <EmptyState title="No attack selected" detail="Go back to Attacks and choose a captured attempt before promotion." />}

                  {selectedFinding ? (
                    <>
                      <div className="finding-editor-grid" style={{ marginTop: '14px' }}>
                        <label className="finding-field"><span>Title</span><input value={findingDraft.title} onChange={(event) => setFindingDraft((draft) => ({ ...draft, title: event.target.value }))} /></label>
                        <label className="finding-field"><span>Severity</span><select value={findingDraft.severity} onChange={(event) => setFindingDraft((draft) => ({ ...draft, severity: event.target.value }))}><option value="info">Info</option><option value="low">Low</option><option value="medium">Medium</option><option value="high">High</option><option value="critical">Critical</option></select></label>
                        <label className="finding-field finding-field-wide"><span>Remediation</span><textarea value={findingDraft.remediation} onChange={(event) => setFindingDraft((draft) => ({ ...draft, remediation: event.target.value }))} /></label>
                        <label className="finding-field"><span>Status</span><input value={findingDraft.status} onChange={(event) => setFindingDraft((draft) => ({ ...draft, status: event.target.value }))} /></label>
                      </div>
                      <div className="finding-actions"><button type="button" className="secondary-link" onClick={() => void saveFinding()}>Save revision</button></div>
                      {findingConflict ? <div className="live-banner review"><strong>Finding note</strong> {findingConflict}</div> : null}
                      <section className="finding-trace-panel" style={{ marginTop: '14px' }}>
                        <div className="panel-heading compact"><h3>Evidence trace</h3><p>{selectedFinding.evidenceActionIds.join(', ') || 'No evidence linked yet.'}</p></div>
                        <div className="finding-trace-list">
                          {selectedFinding.evidenceActionIds.map((actionId) => <div key={actionId} className="revision-list"><span>{actionId}</span></div>)}
                        </div>
                      </section>
                      <section className="finding-revision-panel" style={{ marginTop: '14px' }}>
                        <div className="panel-heading compact"><h3>Revision history</h3><p>{selectedFinding.id}</p></div>
                        <div className="revision-list">
                          <div><span>Revision</span><small>{selectedFinding.revision}</small></div>
                          <div><span>Promoted</span><small>{formatTime(selectedFinding.promotedAt)}</small></div>
                          <div><span>Updated</span><small>{formatTime(selectedFinding.updatedAt)}</small></div>
                        </div>
                      </section>
                    </>
                  ) : null}
                </div>
              </section>
            ) : null}

            {activePhase === 'summit' ? (
              <section className="summit-flow" aria-label="Summit export and teardown flow">
                <div className="summit-status">
                  <span className={`status-chip ${summitStatus}`}>{summitStatus}</span>
                  <div className="export-meter" role="progressbar" aria-label="Export progress" aria-valuemin={0} aria-valuemax={100} aria-valuenow={summitProgress}><span style={{ width: `${summitProgress}%` }} /></div>
                  <p className="summit-step">{summitStep}</p>
                  <p>{summitStatus === 'verified' ? 'Hash verified, not signed. The teardown guard is available.' : 'The report snapshot comes from the authoritative API before any wipe can happen.'}</p>
                  {summitError ? <p className="summit-error">{summitError}</p> : null}
                </div>
                <div className="summit-controls">
                  <button type="button" className="primary-button" onClick={() => void startSummitExport()} disabled={summitStatus === 'preflight' || summitStatus === 'exporting' || summitStatus === 'verifying'}>{summitStatus === 'verified' ? 'Re-run export verification' : 'Run live export preflight'}</button>
                  <button type="button" className="secondary-link" onClick={() => exportAbortRef.current?.abort()} disabled={summitStatus !== 'preflight' && summitStatus !== 'exporting' && summitStatus !== 'verifying'}>Cancel export</button>
                </div>
                {summitStatus === 'verified' && summitReceipt ? (
                  <article className="receipt-card" aria-label="Verified export receipt">
                    <div className="panel-heading compact"><h3>Receipt verified</h3><p>Capture stayed live while the bundle froze. Hash verified, not signed.</p></div>
                    <dl className="receipt-grid">
                      <div><dt>Verified</dt><dd>{summitReceipt.verifiedAt}</dd></div>
                      <div><dt>Snapshot hash</dt><dd className="report-snippet">{summitReceipt.snapshotHash}</dd></div>
                      <div><dt>PDF hash</dt><dd className="report-snippet">{summitReceipt.pdfSha256}</dd></div>
                      <div><dt>Manifest</dt><dd>{summitReceipt.manifestHash}</dd></div>
                      <div><dt>Note</dt><dd>{summitReceipt.note}</dd></div>
                    </dl>
                  </article>
                ) : null}
                <article className="break-glass-panel" aria-label="Break-glass teardown guard">
                  <div className="panel-heading compact"><h3>Break-glass teardown</h3><p>Export receipt required before the live box can be destroyed.</p></div>
                  <label className="break-glass-toggle"><input type="checkbox" checked={teardownArmed} onChange={(event) => setTeardownArmed(event.target.checked)} /><span>Arm the teardown guard</span></label>
                  <label className="break-glass-input"><span>Type WIPE NOW to confirm</span><input value={destroyPhrase} onChange={(event) => setDestroyPhrase(event.target.value)} placeholder="WIPE NOW" /></label>
                  <button type="button" className="danger-button" disabled={!summitReceipt || !teardownArmed || destroyPhrase.trim().toUpperCase() !== 'WIPE NOW' || destroyed} onClick={() => setDestroyed(true)}>{destroyed ? 'Teardown queued' : 'Destroy disposable instance'}</button>
                  <p className="summit-warning">{destroyed ? 'Break-glass was used after receipt verification. Nothing else should run here.' : 'Guard remains fogged until the verified receipt and break-glass phrase are in place.'}</p>
                </article>
                {reportSnapshot ? <div className="live-banner review"><strong>Report</strong> Snapshot ready for preview in the Summit report view.</div> : null}
              </section>
            ) : null}

            <section className="operations-shell" aria-label="Provisioning and claim review workspace">
              <div className="panel-heading">
                <div>
                  <p className="guide-note-kicker">Operations</p>
                  <h2>Provisioning and review</h2>
                </div>
                <p>One-time secrets are shown once, AI actors must cite an active human authorizer, and pending gaps stay visible until they are linked or dismissed.</p>
              </div>

              <div className="operations-grid">
                <article className="operations-card">
                  <div className="detail-head">
                    <div>
                      <p className="guide-note-kicker">Actor provisioning</p>
                      <h3>Issue a one-time credential</h3>
                    </div>
                    <StatusPill status="review">secret once</StatusPill>
                  </div>
                  <div className="ops-form-grid">
                    <label className="finding-field"><span>Kind</span><select value={provisionDraft.kind} onChange={(event) => setProvisionDraft((draft) => ({ ...draft, kind: event.target.value as 'human' | 'ai_agent', authorizedBy: event.target.value === 'human' ? '' : draft.authorizedBy }))}><option value="human">Human</option><option value="ai_agent">AI agent</option></select></label>
                    <label className="finding-field"><span>Handle</span><input value={provisionDraft.handle} onChange={(event) => setProvisionDraft((draft) => ({ ...draft, handle: event.target.value }))} placeholder="alex.operator" /></label>
                    <label className="finding-field"><span>Role</span><select value={provisionDraft.role} onChange={(event) => setProvisionDraft((draft) => ({ ...draft, role: event.target.value }))}><option value="owner">Owner</option><option value="operator">Operator</option><option value="viewer">Viewer</option></select></label>
                  </div>
                  {provisionDraft.kind === 'ai_agent' ? (
                    <div className="ops-form-grid ops-form-grid-wide" style={{ marginTop: '10px' }}>
                      <label className="finding-field"><span>Agent name</span><input value={provisionDraft.agentName} onChange={(event) => setProvisionDraft((draft) => ({ ...draft, agentName: event.target.value }))} placeholder="Synthetic Field Agent" /></label>
                      <label className="finding-field"><span>Model</span><input value={provisionDraft.model} onChange={(event) => setProvisionDraft((draft) => ({ ...draft, model: event.target.value }))} placeholder="gpt-4.1" /></label>
                      <label className="finding-field"><span>Version</span><input value={provisionDraft.version} onChange={(event) => setProvisionDraft((draft) => ({ ...draft, version: event.target.value }))} placeholder="2025.01" /></label>
                      <label className="finding-field"><span>Authorized by</span>
                        <select value={provisionDraft.authorizedBy} onChange={(event) => setProvisionDraft((draft) => ({ ...draft, authorizedBy: event.target.value }))}>
                          <option value="">Pick an active human authorizer</option>
                          {activeHumanActors.map((actor) => <option key={actor.actor.id} value={actor.actor.id}>{actor.actor.handle} · rev {actor.revision}</option>)}
                        </select>
                      </label>
                      <p className="guide-note-empty ops-inline-note">AI actors need an active human operator on the hook before a credential can be issued.</p>
                    </div>
                  ) : null}
                  <div className="guide-tools" style={{ marginTop: '12px' }}>
                    <button type="button" className="primary-button" onClick={() => void issueActorCredential()} disabled={!provisionDraft.handle.trim() || (provisionDraft.kind === 'ai_agent' && !provisionDraft.authorizedBy)}>Issue one-time credential</button>
                    <button type="button" className="secondary-link" onClick={() => setActorCredential(null)} disabled={!actorCredential}>Burn copy</button>
                  </div>
                  {actorConflict ? <div className="live-banner review"><strong>Credential note</strong> {actorConflict}</div> : null}
                  {actorCredential ? (
                    <div className="secret-token-card" aria-label="One-time credential response">
                      <div className="panel-heading compact"><h4>One-time token</h4><p>{actorCredential.actorRecord.actor.handle} · v{actorCredential.actorRecord.credentialVersion} · issued {formatTime(actorCredential.issuedAt)}</p></div>
                      <div className="secret-token" role="textbox" aria-readonly="true">{actorCredential.token}</div>
                      <div className="guide-tools">
                        <button type="button" className="secondary-link" onClick={() => void navigator.clipboard?.writeText(actorCredential.token)}>Copy token</button>
                        <span className="guide-note-empty">Do not paste this into the audit trail or logs.</span>
                      </div>
                    </div>
                  ) : <p className="guide-note-empty">The plaintext token only appears in this response once. It never returns from list or read endpoints.</p>}
                </article>

                <article className="operations-card">
                  <div className="detail-head">
                    <div>
                      <p className="guide-note-kicker">Actor roster</p>
                      <h3>Rotate or revoke live credentials</h3>
                    </div>
                    <StatusPill status="neutral">{actorsStatus}</StatusPill>
                  </div>
                  <label className="guide-search" style={{ marginTop: 0 }}>
                    <span className="sr-only">Search actors</span>
                    <input value={actorQuery} onChange={(event) => setActorQuery(event.target.value)} placeholder="Search handle, role, revision…" aria-label="Search actors" />
                  </label>
                  {actorsError ? <div className="live-banner"><strong>Actor note</strong> {actorsError}</div> : null}
                  {!filteredActors.length && actorsStatus === 'ready' ? <EmptyState title="No actors yet" detail="Provision a credential above, then rotate or revoke it from the roster." /> : null}
                  <div className="actor-roster">
                    {filteredActors.map((actor) => (
                      <button key={actor.actor.id} type="button" className={`actor-row ${selectedActor?.actor.id === actor.actor.id ? 'is-selected' : ''}`.trim()} onClick={() => setSelectedActorId(actor.actor.id)}>
                        <div className="actor-row-top">
                          <strong>{actor.actor.handle}</strong>
                          <StatusPill status={actor.status === 'active' ? 'success' : 'blocked'}>{actor.status}</StatusPill>
                        </div>
                        <div className="actor-row-main">
                          <span>{actor.actor.kind}{actor.actor.kind === 'ai_agent' ? ` · ${actor.actor.model}` : ''}</span>
                          <span>role {actor.actor.role}</span>
                        </div>
                        <div className="actor-row-foot">
                          <span>cred v{actor.credentialVersion}</span>
                          <span>rev {actor.revision}</span>
                        </div>
                      </button>
                    ))}
                  </div>
                  {selectedActor ? (
                    <div className="secret-token-card" style={{ marginTop: '12px' }}>
                      <div className="panel-heading compact"><h4>{selectedActor.actor.handle}</h4><p>{selectedActor.actor.kind} · {selectedActor.actor.role} · {selectedActor.status}</p></div>
                      <dl className="ops-meta-grid">
                        <div><dt>Created by</dt><dd>{selectedActor.createdBy}</dd></div>
                        <div><dt>Created at</dt><dd>{formatTime(selectedActor.createdAt)}</dd></div>
                        <div><dt>Credential version</dt><dd>{selectedActor.credentialVersion}</dd></div>
                        <div><dt>Revision</dt><dd>{selectedActor.revision}</dd></div>
                        <div><dt>Revision gate</dt><dd>If-Match {revisionHeader(selectedActor.revision)}</dd></div>
                        {selectedActor.actor.kind === 'ai_agent' ? <div><dt>Authorized by</dt><dd>{selectedActor.actor.authorizedBy || '—'}</dd></div> : null}
                        {selectedActor.lastRotatedAt ? <div><dt>Last rotated</dt><dd>{formatTime(selectedActor.lastRotatedAt)}</dd></div> : null}
                        {selectedActor.revokedAt ? <div><dt>Revoked at</dt><dd>{formatTime(selectedActor.revokedAt)}</dd></div> : null}
                      </dl>
                      <div className="guide-tools">
                        <button type="button" className="primary-button" onClick={() => void rotateActorCredential(selectedActor)} disabled={selectedActor.status !== 'active'}>Rotate credential</button>
                        <button type="button" className="secondary-link" onClick={() => void revokeActorCredential(selectedActor)} disabled={selectedActor.status !== 'active'}>Revoke credential</button>
                      </div>
                    </div>
                  ) : <EmptyState title="No actor selected" detail="Provision a credential above, then choose a record to rotate or revoke it." />}
                </article>
              </div>

              <article className="operations-card operations-card-wide">
                <div className="detail-head">
                  <div>
                    <p className="guide-note-kicker">Claim review</p>
                    <h3>Pending out-of-band claims</h3>
                  </div>
                  <StatusPill status={claimsStatus === 'ready' ? 'review' : 'neutral'}>{claimsStatus}</StatusPill>
                </div>
                <p className="workspace-lede">Best-effort claims stay visible until they are linked to a captured action or explicitly dismissed; resolved gaps remain in the trail for audit.</p>
                {claimsError ? <div className="live-banner"><strong>Claim note</strong> {claimsError}</div> : null}
                {!filteredClaims.length && claimsStatus === 'ready' ? <EmptyState title="No pending claims" detail="Best-effort claim review is clear for now." /> : null}
                <div className="claim-review-layout">
                  <div className="claim-review-list">
                    {filteredClaims.map((claim) => (
                      <button key={claim.id} type="button" className={`claim-row ${selectedClaim?.id === claim.id ? 'is-selected' : ''}`.trim()} onClick={() => setSelectedClaimId(claim.id)}>
                        <div className="claim-row-top">
                          <strong>{claim.claimKind} · {claim.claimedSubjectId.slice(0, 8)}…</strong>
                          <StatusPill status={claim.status === 'pending' ? 'review' : claim.status === 'linked' ? 'success' : 'blocked'}>{claim.status}</StatusPill>
                        </div>
                        <div className="claim-row-main">
                          <span>{claim.reason}</span>
                          <span>boundary {claim.detectionBoundary}</span>
                        </div>
                        <div className="claim-row-foot">
                          <span>{formatTime(claim.observedAt)}</span>
                          <span>action {claim.sourceActionId || 'not captured'}</span>
                        </div>
                      </button>
                    ))}
                  </div>
                  <div className="claim-review-detail">
                    {selectedClaim ? (
                      <>
                        <div className="detail-head">
                          <div>
                            <p className="guide-note-kicker">Selected claim</p>
                            <h4>{selectedClaim.claimKind} · {selectedClaim.id}</h4>
                          </div>
                          <StatusPill status={selectedClaim.status === 'pending' ? 'review' : selectedClaim.status === 'linked' ? 'success' : 'blocked'}>{selectedClaim.status}</StatusPill>
                        </div>
                        <dl className="ops-meta-grid">
                          <div><dt>Claimed subject</dt><dd>{selectedClaim.claimedSubjectId}</dd></div>
                          <div><dt>Observed by</dt><dd>{selectedClaim.observedBy.handle}</dd></div>
                          <div><dt>Observed at</dt><dd>{formatTime(selectedClaim.observedAt)}</dd></div>
                          <div><dt>Boundary</dt><dd>{selectedClaim.detectionBoundary}</dd></div>
                          <div><dt>Source action</dt><dd>{selectedClaim.sourceActionId || 'missing'}</dd></div>
                          <div><dt>Revision</dt><dd>{selectedClaim.revision}</dd></div>
                        </dl>
                        {selectedClaim.status === 'pending' ? (
                          <>
                            <div className="finding-editor-grid" style={{ marginTop: '12px' }}>
                              <label className="finding-field"><span>Resolution</span><select value={claimResolutionDraft.resolution} onChange={(event) => setClaimResolutionDraft((draft) => ({ ...draft, resolution: event.target.value as 'linked' | 'dismissed' }))}><option value="linked">Link to captured action</option><option value="dismissed">Dismiss visibly</option></select></label>
                              <label className="finding-field"><span>Source action</span><input value={claimResolutionDraft.sourceActionId} onChange={(event) => setClaimResolutionDraft((draft) => ({ ...draft, sourceActionId: event.target.value }))} placeholder={selectedAction?.id || 'Choose a captured action'} disabled={claimResolutionDraft.resolution === 'dismissed'} /></label>
                              <label className="finding-field finding-field-wide"><span>Notes</span><textarea value={claimResolutionDraft.notes} onChange={(event) => setClaimResolutionDraft((draft) => ({ ...draft, notes: event.target.value }))} placeholder="Explain why the gap is linked or dismissed." /></label>
                            </div>
                            <div className="guide-tools">
                              <button type="button" className="secondary-link" onClick={() => setClaimResolutionDraft((draft) => ({ ...draft, sourceActionId: selectedAction?.id || draft.sourceActionId }))} disabled={!selectedAction}>Use selected attack</button>
                              <button type="button" className="primary-button" onClick={() => void resolveClaim()}>Resolve claim</button>
                            </div>
                          </>
                        ) : <p className="guide-note-empty">This claim is already resolved; the review view keeps the gap visible for audit purposes.</p>}
                        <dl className="ops-meta-grid" style={{ marginTop: '12px' }}>
                          <div><dt>Detection boundary</dt><dd>{selectedClaim.detectionBoundary}</dd></div>
                          <div><dt>Reason</dt><dd>{selectedClaim.reason}</dd></div>
                        </dl>
                        {claimConflict ? <div className="live-banner review"><strong>Review note</strong> {claimConflict}</div> : null}
                      </>
                    ) : <EmptyState title="No claim selected" detail="Choose a pending gap to link it to a captured action or dismiss it plainly." />}
                  </div>
                </div>
              </article>
            </section>

            <div className="workspace-footer">
              <a className="secondary-link" href={phasePath(engagementId, waypointOrder[Math.max(0, activeIndex - 1)])} onClick={(event) => { event.preventDefault(); const prev = waypointOrder[Math.max(0, activeIndex - 1)]; setActivePhase(prev); navigateToPhase(engagementId, prev); }}>Back to {phaseNames[waypointOrder[Math.max(0, activeIndex - 1)]]}</a>
              <a className="primary-button" href={activePhase === 'summit' ? reportPath(engagementId) : phasePath(engagementId, waypointOrder[Math.min(waypointOrder.length - 1, activeIndex + 1)])} onClick={(event) => {
                event.preventDefault();
                if (activePhase === 'summit') {
                  setView('report');
                  navigateToReport(engagementId);
                } else {
                  const next = waypointOrder[Math.min(waypointOrder.length - 1, activeIndex + 1)];
                  setActivePhase(next);
                  navigateToPhase(engagementId, next);
                }
              }}>{activePhase === 'summit' ? 'Open report preview →' : `Continue to ${phaseNames[waypointOrder[Math.min(waypointOrder.length - 1, activeIndex + 1)]]} →`}</a>
            </div>
          </section>
        </section>

        <aside className="sidebar" aria-label="Guide and trail details">
          <section className="log-panel" aria-label="Notable alerts">
            <div className="panel-heading compact">
              <h2>⚑ Notable alerts</h2>
              <p>Alerts arrive from the live SSE stream and stay attached to the audit trail.</p>
            </div>
            {auditStatus === 'loading' ? <div className="guide-note-empty">Loading notable alerts…</div> : null}
            {auditStatus === 'error' ? <div className="live-banner"><strong>Alert feed error</strong> {auditError}</div> : null}
            {!notableAlerts.length && auditStatus === 'ready' ? <div className="guide-note-empty">No notable alerts yet. The trail is still open for live capture.</div> : null}
            <ul>
              {notableAlerts.map((entry, index) => {
                const data = isRecord(entry.data) ? entry.data : {};
                return (
                  <li key={entry.id} className={index === 0 ? 'is-current' : ''}>
                    <strong>{data.ruleId || 'trail alert'}</strong> · {data.title || buildAuditSummary(entry)}
                    <br />
                    {entry.actor.handle} · {formatTime(entry.occurredAt)}{data.sourceActionId ? ` · action ${data.sourceActionId}` : ''}
                  </li>
                );
              })}
            </ul>
          </section>

          <nav className="route-nav" aria-label="Engagement waypoints">
            <div className="panel-heading">
              <h2>Waypoints</h2>
              <p>All phases stay accessible; fog means no data discovered yet.</p>
            </div>
            <ol>
              {waypoints.map((waypoint, index) => (
                <li key={waypoint.id}>
                  <a href={waypoint.path} className={`route-link ${waypoint.id === activeId ? 'is-active' : ''}`.trim()} aria-current={waypoint.id === activeId ? 'step' : undefined} onClick={(event) => { event.preventDefault(); setActivePhase(waypoint.id); navigateToPhase(engagementId, waypoint.id); }}>
                    <span className="route-link-copy"><strong>{waypoint.name}</strong><span>Stage {index + 1} of {waypointOrder.length}</span></span>
                    <span className={`route-status ${waypoint.state}`}>{shortStateLabel(waypoint.state)}</span>
                  </a>
                </li>
              ))}
            </ol>
          </nav>

          <section className="guide-panel artifact" aria-label="Guide's note">
            <div className="panel-icon" aria-hidden="true">🧭</div>
            <div className="guide-copy">
              <h2>Guide's note</h2>
              <p>{activeGuideNote.what}</p>
              <article id={activeGuideNote.id} className="guide-note-card">
                <p className="guide-note-kicker">{phaseNames[activePhase]} · reviewed phase briefing</p>
                <h3>{activeGuideNote.title}</h3>
                <dl>
                  <div><dt>What</dt><dd>{activeGuideNote.what}</dd></div>
                  <div><dt>When</dt><dd>{activeGuideNote.when}</dd></div>
                  <div><dt>Risks</dt><dd>{activeGuideNote.risks}</dd></div>
                </dl>
              </article>
              <div className="guide-tools">
                <label className="guide-search">
                  <span className="sr-only">Search reviewed guide notes</span>
                  <input value={guideQuery} onChange={(event) => setGuideQuery(event.target.value)} placeholder="Search reviewed phases and techniques" aria-label="Search reviewed guide notes" />
                </label>
                <button type="button" className="primary-button" onClick={() => { setActivePhase(guidePhase); navigateToPhase(engagementId, guidePhase); }}>
                  Continue to {phaseNames[guidePhase]} →
                </button>
              </div>
              <div className="guide-note-list" aria-label="Reviewed guide notes">
                {visibleNotes.length ? visibleNotes.map((note) => (
                  <article key={note.id} id={note.id} className="guide-note-card">
                    <p className="guide-note-kicker">{phaseNames[note.phase]} · reviewed note</p>
                    <h3>{note.title}</h3>
                    <dl>
                      <div><dt>What</dt><dd>{note.what}</dd></div>
                      <div><dt>When</dt><dd>{note.when}</dd></div>
                      <div><dt>Risks</dt><dd>{note.risks}</dd></div>
                    </dl>
                  </article>
                )) : <p className="guide-note-empty">No reviewed notes match this search.</p>}
              </div>
            </div>
          </section>

          <section className="log-panel" aria-label="Journey log">
            <div className="panel-heading compact">
              <h2>📖 Journey log</h2>
              <p>The audit trail is the journey log — one entry per meaningful action.</p>
            </div>
            {auditStatus === 'loading' ? <div className="guide-note-empty">Loading the latest trail entries…</div> : null}
            {auditStatus === 'error' ? <div className="live-banner"><strong>Journey log error</strong> {auditError}</div> : null}
            {!auditEvents.length && auditStatus === 'ready' ? <div className="guide-note-empty">No trail entries yet. The workspace stays open and ready.</div> : null}
            <ul>
              {auditEvents.map((entry, index) => (
                <li key={entry.id} className={index === 0 ? 'is-current' : ''}>
                  <strong>{entry.type}</strong> · {buildAuditSummary(entry)}
                  <br />
                  {entry.actor.handle} · {entry.origin.kind} · {formatTime(entry.occurredAt)}
                </li>
              ))}
            </ul>
          </section>

          <section className="route-summary" aria-label="Route summary">
            <div>
              <p className="metric-label">Current waypoint</p>
              <strong>{activeWaypoint.name}</strong>
            </div>
            <p>{phaseSummary(activePhase)}</p>
          </section>
        </aside>
      </div>
    </main>
  );
}
