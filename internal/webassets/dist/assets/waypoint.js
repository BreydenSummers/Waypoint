const root = document.getElementById('root');

const phaseOrder = ['recon', 'attacks', 'findings', 'summit'];
const phaseData = {
  recon: {
    name: 'Recon',
    label: 'Recon',
    path: '/engagements/demo/recon',
    x: 72,
    y: 248,
    note: 'Imported records, host notes, and discovery output stay in your pack here.',
    workspaceTitle: 'Recon workspace',
    workspaceLede: 'Collect raw signals, preserve provenance, and keep the audit spine instant to query.',
    cards: [
      { title: 'Pack', items: ['Scope and operator tokens', 'Imported records', 'Entity dedup hints'] },
      { title: 'Trail rules', items: ['Every capture is attributed', 'Unknown tools land raw first', 'Nothing is ever dropped'] },
    ],
  },
  attacks: {
    name: 'Attacks',
    label: 'Attacks',
    path: '/engagements/demo/attacks',
    x: 228,
    y: 182,
    note: 'Captured actions land here with source, host, egress IP, and outcome.',
    workspaceTitle: 'Attacks workspace',
    workspaceLede: 'Run commands through the wrapper, keep the path to each attempt obvious, and preserve evidence.',
    cards: [
      { title: 'Capture lane', items: ['Command + argv', 'stdout / stderr refs', 'Exit status and timing'] },
      { title: 'Attribution', items: ['Operator identity', 'Exec host IP', 'Public egress IP + pivot chain'] },
    ],
  },
  findings: {
    name: 'Findings',
    label: 'Findings',
    path: '/engagements/demo/findings',
    x: 430,
    y: 128,
    note: 'No promoted results yet — this waypoint stays in fog until the first finding lands.',
    workspaceTitle: 'Findings workspace',
    workspaceLede: 'Promote confirmed results, keep evidence links intact, and draft the report straight from the trail.',
    cards: [
      { title: 'Promotion', items: ['Attack → finding', 'Severity and remediation fields', 'Evidence stays linked'] },
      { title: 'Report copy', items: ['Client-readable summary', 'Machine-readable bundle', 'Hash manifest hook'] },
    ],
  },
  summit: {
    name: 'Summit',
    label: 'Summit',
    path: '/engagements/demo/summit',
    x: 586,
    y: 64,
    note: 'Reach the summit to export the engagement bundle and prepare teardown.',
    workspaceTitle: 'Summit workspace',
    workspaceLede: 'Final review, export, and bundle integrity checks live here before the box is wiped cleanly.',
    cards: [
      { title: 'Export bundle', items: ['Database dump', 'Evidence artifacts', 'SHA-256 manifest'] },
      { title: 'Teardown', items: ['Confirm the report reconstructs', 'Leave the instance disposable', 'No lingering state'] },
    ],
  },
};

const demoRows = [
  {
    id: '101',
    contractVersion: '1.0.0',
    type: 'capture.received',
    occurredAt: '2025-01-10T08:12:00Z',
    engagementId: 'demo',
    actor: { id: 'a1', kind: 'human', handle: 'alex.operator', role: 'operator' },
    origin: { kind: 'rest', service: 'collector' },
    subject: { type: 'action', id: 'capture-101', revision: 1 },
    requestId: 'req-101',
    correlationId: 'corr-101',
    data: {
      phase: 'recon',
      source: 'collector',
      target: '10.10.12.0/24',
      result: { kind: 'success', label: 'Success', detail: '240 hosts discovered' },
      summary: 'Network sweep captured without parser help.',
      evidence: {
        kind: 'stdout',
        sha256: 'b6f7a6caa28f7a6e9d9f3e1f0d56f0b1f5bd9f9b4b44b7f2ef2f6dbf5a0a9c01',
        byteLength: 1843,
        mediaType: 'text/plain',
        safePreview: 'nmap -sn 10.10.12.0/24\nHost is up (0.045s latency)\n240 hosts are up',
      },
    },
  },
  {
    id: '102',
    contractVersion: '1.0.0',
    type: 'audit.note.added',
    occurredAt: '2025-01-10T08:16:00Z',
    engagementId: 'demo',
    actor: { id: 'a2', kind: 'human', handle: 'mira.ops', role: 'analyst' },
    origin: { kind: 'rest', service: 'waypoint-core' },
    subject: { type: 'action', id: 'note-102', revision: 1 },
    requestId: 'req-102',
    correlationId: 'corr-102',
    data: {
      phase: 'recon',
      source: 'waypoint-core',
      target: 'scope note',
      result: { kind: 'success', label: 'Noted', detail: 'Operator note attached' },
      summary: 'Scope note captured as trail metadata.',
      evidence: { kind: 'attachment', sha256: '45d5bb7451a1dc43d3f8e0e77f2f22d1b6e39f86d01a5c9e3d35b06d2ff7d010', byteLength: 512, mediaType: 'application/json', safePreview: '{\n  "scope": "students network",\n  "note": "stay inside approved range"\n}' },
    },
  },
  {
    id: '103',
    contractVersion: '1.0.0',
    type: 'capture.received',
    occurredAt: '2025-01-10T08:22:00Z',
    engagementId: 'demo',
    actor: { id: 'a1', kind: 'human', handle: 'alex.operator', role: 'operator' },
    origin: { kind: 'rest', service: 'collector' },
    subject: { type: 'action', id: 'capture-103', revision: 1 },
    requestId: 'req-103',
    correlationId: 'corr-103',
    data: {
      phase: 'attacks',
      source: 'collector',
      target: 'mail01.internal',
      result: { kind: 'blocked', label: 'Blocked', detail: 'SMB signing prevented relay' },
      summary: 'Attempt logged with clear failure state.',
      evidence: {
        kind: 'stderr',
        sha256: 'b32e6f9f5d9b1f8c735f0d3c56a5c1b5c6f25f0aa27d8c0c43d1e8b4c26a4ab5',
        byteLength: 211,
        mediaType: 'text/plain',
        safePreview: 'Relay refused: SMB signing required\nNo credentials were replayed.',
      },
    },
  },
  {
    id: '104',
    contractVersion: '1.0.0',
    type: 'audit.alert.raised',
    occurredAt: '2025-01-10T08:28:00Z',
    engagementId: 'demo',
    actor: { id: 'a3', kind: 'ai_agent', handle: 'field-agent-7', role: 'operator', agentName: 'Waypoint', model: 'gpt-4.1', version: '1.0', authorizedBy: 'alex.operator' },
    origin: { kind: 'mcp', service: 'waypoint-core' },
    subject: { type: 'action', id: 'ai-104', revision: 1 },
    requestId: 'req-104',
    correlationId: 'corr-104',
    data: {
      phase: 'attacks',
      source: 'mcp',
      target: 'svc/rdp-3389',
      result: { kind: 'success', label: 'Success', detail: 'New reachable segment confirmed' },
      summary: 'AI-authored action preserved with human authorization metadata.',
      evidence: {
        kind: 'screenshot',
        sha256: 'f5f0f24f6b3c1d2e1a44f71bb2f7dd3d5bcf3b58d079d7adf7a2cc0a6b4d6a2f',
        byteLength: 15524,
        mediaType: 'image/png',
        safePreview: '[screenshot omitted]\nRDP banner confirmed, credentials not shown.',
      },
    },
  },
  {
    id: '105',
    contractVersion: '1.0.0',
    type: 'capture.received',
    occurredAt: '2025-01-10T08:37:00Z',
    engagementId: 'demo',
    actor: { id: 'a1', kind: 'human', handle: 'alex.operator', role: 'operator' },
    origin: { kind: 'collector', service: 'remote-agent' },
    subject: { type: 'action', id: 'capture-105', revision: 1 },
    requestId: 'req-105',
    correlationId: 'corr-105',
    data: {
      phase: 'attacks',
      source: 'remote-agent',
      target: 'fileserver02',
      result: { kind: 'success', label: 'Parsed', detail: 'plugin matched smb enumeration' },
      summary: 'Remote agent synced an outbound capture after reconnect.',
      evidence: {
        kind: 'stdout',
        sha256: 'aa9d8ad4f99f1df28cbcb2abf16b617ce5bcbd2dfe1cc9fe0a0fa10c9e5f7ae7',
        byteLength: 3094,
        mediaType: 'text/plain',
        safePreview: 'smbclient -L //fileserver02 -U student\\alex\nDomain=[LAB] OS=[Windows Server] ...',
      },
    },
  },
  {
    id: '106',
    contractVersion: '1.0.0',
    type: 'audit.review.flagged',
    occurredAt: '2025-01-10T08:44:00Z',
    engagementId: 'demo',
    actor: { id: 'a2', kind: 'human', handle: 'mira.ops', role: 'analyst' },
    origin: { kind: 'rest', service: 'waypoint-core' },
    subject: { type: 'finding', id: 'find-106', revision: 1 },
    requestId: 'req-106',
    correlationId: 'corr-106',
    data: {
      phase: 'findings',
      source: 'waypoint-core',
      target: 'finding draft',
      result: { kind: 'needs-review', label: 'Needs review', detail: 'Evidence is linked but severity is missing' },
      summary: 'A draft is ready, but still fogged until severity is filled in.',
      evidence: { kind: 'attachment', sha256: '49de56e18f4e5f7fd5d1b92a5f9d2e9c7f5f2a8f15cd4d8a1d6b4a7c1b9b7a4f', byteLength: 788, mediaType: 'application/json', safePreview: '{\n  "finding": "draft",\n  "severity": null\n}' },
    },
  },
  {
    id: '107',
    contractVersion: '1.0.0',
    type: 'audit.export.requested',
    occurredAt: '2025-01-10T08:51:00Z',
    engagementId: 'demo',
    actor: { id: 'a1', kind: 'human', handle: 'alex.operator', role: 'operator' },
    origin: { kind: 'rest', service: 'waypoint-core' },
    subject: { type: 'export', id: 'bundle-107', revision: 1 },
    requestId: 'req-107',
    correlationId: 'corr-107',
    data: {
      phase: 'summit',
      source: 'waypoint-core',
      target: 'engagement bundle',
      result: { kind: 'queued', label: 'Queued', detail: 'Manifest and report snapshot are being assembled' },
      summary: 'Safe export is staged without exposing raw bytes in the row.',
      evidence: { kind: 'bundle', sha256: '8bc432f5c27f8f4b2f13a1ff3cb36d27f0a8f23dca9d469b1edb7c3f49db0c11', byteLength: 24612, mediaType: 'application/zip', safePreview: 'bundle manifest pending\npaths and hashes verified separately' },
    },
  },
  {
    id: '108',
    contractVersion: '1.0.0',
    type: 'audit.teardown.blocked',
    occurredAt: '2025-01-10T08:55:00Z',
    engagementId: 'demo',
    actor: { id: 'a2', kind: 'human', handle: 'mira.ops', role: 'analyst' },
    origin: { kind: 'rest', service: 'waypoint-core' },
    subject: { type: 'destroy', id: 'destroy-108', revision: 1 },
    requestId: 'req-108',
    correlationId: 'corr-108',
    data: {
      phase: 'summit',
      source: 'waypoint-core',
      target: 'destroy guard',
      result: { kind: 'blocked', label: 'Blocked', detail: 'Export receipt required before teardown' },
      summary: 'Guarded teardown kept the instance disposable but not destructible yet.',
      evidence: { kind: 'attachment', sha256: '3c7f8e9d9c44a6f2b8e13d9b1d9f0cf26b0e55c9d5ad0ebf5a2bc77c4a7c61bb', byteLength: 394, mediaType: 'application/json', safePreview: '{\n  "receipt": false,\n  "action": "destroy"\n}' },
    },
  },
  {
    id: '109',
    contractVersion: '1.0.0',
    type: 'audit.note.added',
    occurredAt: '2025-01-10T09:02:00Z',
    engagementId: 'demo',
    actor: { id: 'a3', kind: 'ai_agent', handle: 'field-agent-7', role: 'operator', agentName: 'Waypoint', model: 'gpt-4.1', version: '1.0', authorizedBy: 'alex.operator' },
    origin: { kind: 'mcp', service: 'waypoint-core' },
    subject: { type: 'finding', id: 'find-109', revision: 2 },
    requestId: 'req-109',
    correlationId: 'corr-109',
    data: {
      phase: 'findings',
      source: 'mcp',
      target: 'remediation note',
      result: { kind: 'success', label: 'Saved', detail: 'Operator-approved wording added' },
      summary: 'AI actor note kept the human authorizer visible.',
      evidence: { kind: 'attachment', sha256: '0dfc9a4e7f3d1c5b99f2a7f6e1e4f1ef2e08d7c1c3db7d4b9f21c9a4c6d2e3f1', byteLength: 602, mediaType: 'application/json', safePreview: '{\n  "remediation": "Review segmentation and SMB exposure"\n}' },
    },
  },
];

const demoPageSize = 4;
const state = {
  theme: getInitialTheme(),
  activePhase: getInitialPhase(),
  rows: [],
  visibleRows: [],
  selectedId: null,
  pageCursor: null,
  highWaterCursor: null,
  hasMore: false,
  filters: { actor: 'all', source: 'all', target: 'all', result: 'all', q: '' },
  mode: 'demo',
  liveToken: safeStorageGet('waypoint-audit-token') || '',
  resyncLink: '',
  loading: false,
  streamAbort: null,
  reconnectTimer: null,
  demoCursor: 0,
  renderScheduled: false,
};

function getInitialTheme() {
  if (typeof window === 'undefined') return 'light';
  const stored = safeStorageGet('waypoint-theme');
  if (stored === 'light' || stored === 'dark') return stored;
  return window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light';
}

function getInitialPhase() {
  if (typeof window === 'undefined') return 'attacks';
  const match = window.location.pathname.match(/^\/engagements\/[^/]+\/(recon|attacks|findings|summit)\/?$/);
  return match ? match[1] : 'attacks';
}

function safeStorageGet(key) {
  try {
    return window.localStorage.getItem(key);
  } catch {
    return '';
  }
}

function safeStorageSet(key, value) {
  try {
    window.localStorage.setItem(key, value);
  } catch {
    /* noop */
  }
}

function escapeHtml(value) {
  return String(value ?? '')
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#39;');
}

function phasePath(phase) {
  return phaseData[phase].path;
}

function navigateToPhase(phase, replace = false) {
  const path = phasePath(phase);
  if (replace) {
    window.history.replaceState({}, '', path);
  } else {
    window.history.pushState({}, '', path);
  }
}

function formatTime(iso) {
  return new Intl.DateTimeFormat(undefined, { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' }).format(new Date(iso));
}

function parseData(data) {
  if (typeof data === 'string') {
    try {
      return JSON.parse(data);
    } catch {
      return { raw: data };
    }
  }
  return data && typeof data === 'object' ? data : {};
}

function normalizeRow(row) {
  const data = parseData(row.data);
  const result = data.result && typeof data.result === 'object' ? data.result : { kind: data.result || row.type, label: String(data.result || row.type), detail: '' };
  const evidence = data.evidence && typeof data.evidence === 'object' ? data.evidence : {};
  const source = data.source || row.origin.service || row.origin.kind;
  const target = data.target || row.subject.id;
  const summary = data.summary || `${row.actor.handle} · ${source} · ${target}`;
  return {
    ...row,
    data,
    source,
    target,
    summary,
    result,
    evidence: {
      kind: evidence.kind || 'attachment',
      sha256: evidence.sha256 || '',
      byteLength: Number(evidence.byteLength || 0),
      mediaType: evidence.mediaType || 'text/plain',
      safePreview: String(evidence.safePreview || data.safePreview || 'No safe preview captured.'),
    },
  };
}

function resultClass(kind) {
  switch (String(kind || '').toLowerCase()) {
    case 'success':
      return 'success';
    case 'blocked':
      return 'blocked';
    case 'needs-review':
    case 'review':
      return 'review';
    case 'queued':
      return 'queued';
    case 'redacted':
      return 'redacted';
    default:
      return 'neutral';
  }
}

function actorFilterValue(row) {
  return `${row.actor.kind}::${row.actor.handle}`;
}

function sourceFilterValue(row) {
  return String(row.source || '').toLowerCase();
}

function targetFilterValue(row) {
  return String(row.target || '').toLowerCase();
}

function resultFilterValue(row) {
  return String(row.result?.kind || row.result?.label || '').toLowerCase();
}

function currentRouteLabel() {
  return phaseData[state.activePhase].name;
}

function setTheme(theme) {
  state.theme = theme;
  document.documentElement.dataset.theme = theme;
  safeStorageSet('waypoint-theme', theme);
  scheduleRender();
}

function setPhase(phase) {
  state.activePhase = phase;
  navigateToPhase(phase);
  document.title = `Waypoint — ${phaseData[phase].name}`;
  scheduleRender();
}

function setRows(rows, meta = {}) {
  const normalized = rows.map(normalizeRow);
  state.rows = meta.append ? dedupeRows([...state.rows, ...normalized]) : normalized;
  state.pageCursor = meta.pageCursor ?? state.pageCursor;
  state.highWaterCursor = meta.highWaterCursor ?? state.highWaterCursor;
  state.hasMore = Boolean(meta.hasMore);
  state.selectedId = state.selectedId || state.rows[0]?.id || null;
  refreshVisibleRows();
}

function dedupeRows(rows) {
  const seen = new Set();
  const out = [];
  for (const row of rows) {
    if (seen.has(row.id)) continue;
    seen.add(row.id);
    out.push(row);
  }
  return out.sort((a, b) => Number(a.id) - Number(b.id));
}

function refreshVisibleRows() {
  const q = state.filters.q.trim().toLowerCase();
  state.visibleRows = state.rows.filter((row) => {
    if (state.filters.actor !== 'all' && actorFilterValue(row) !== state.filters.actor) return false;
    if (state.filters.source !== 'all' && sourceFilterValue(row) !== state.filters.source) return false;
    if (state.filters.target !== 'all' && targetFilterValue(row) !== state.filters.target) return false;
    if (state.filters.result !== 'all' && resultFilterValue(row) !== state.filters.result) return false;
    if (q) {
      const haystack = [row.actor.handle, row.actor.kind, row.source, row.target, row.summary, row.result?.label, row.result?.detail, row.evidence.safePreview].join(' ').toLowerCase();
      if (!haystack.includes(q)) return false;
    }
    return true;
  });
  if (!state.visibleRows.some((row) => row.id === state.selectedId)) {
    state.selectedId = state.visibleRows[0]?.id || state.rows[0]?.id || null;
  }
}

function selectedRow() {
  return state.rows.find((row) => row.id === state.selectedId) || state.visibleRows[0] || state.rows[0] || null;
}

function scheduleRender() {
  if (state.renderScheduled) return;
  state.renderScheduled = true;
  requestAnimationFrame(() => {
    state.renderScheduled = false;
    render();
  });
}

function buildSelectOptions(current, values, labelBuilder = (v) => v) {
  return [`<option value="all"${current === 'all' ? ' selected' : ''}>All</option>`].concat(
    values.map((value) => `<option value="${escapeHtml(value)}"${current === value ? ' selected' : ''}>${escapeHtml(labelBuilder(value))}</option>`),
  ).join('');
}

function uniqueValues(selector) {
  return [...new Set(state.rows.map(selector).filter(Boolean))].sort((a, b) => a.localeCompare(b));
}

function feedStatusLabel() {
  if (state.loading) return 'Loading';
  if (state.mode === 'connecting') return 'Connecting';
  if (state.mode === 'live') return `Live SSE · ${state.highWaterCursor || 'cursor pending'}`;
  if (state.mode === 'reconnecting') return `Reconnecting · cursor ${state.highWaterCursor || 'pending'}`;
  if (state.mode === 'resync') return 'Resync required';
  if (state.mode === 'error') return 'Live feed unavailable';
  return `Demo feed · ${state.rows.length} rows`;
}

function feedStatusTone() {
  return state.mode === 'live' ? 'success' : state.mode === 'reconnecting' ? 'queued' : state.mode === 'resync' ? 'review' : state.mode === 'error' ? 'blocked' : 'neutral';
}

function render() {
  const active = phaseData[state.activePhase];
  const currentIndex = phaseOrder.indexOf(state.activePhase);
  const waypoints = phaseOrder.map((phase, index) => {
    const item = phaseData[phase];
    const stateName = index < currentIndex ? 'completed' : index === currentIndex ? 'current' : 'fog';
    return { ...item, id: phase, stateName };
  });
  const selected = selectedRow();
  const actorValues = uniqueValues(actorFilterValue);
  const sourceValues = uniqueValues(sourceFilterValue);
  const targetValues = uniqueValues(targetFilterValue);
  const resultValues = uniqueValues(resultFilterValue);

  root.innerHTML = `
    <main class="app-shell">
      <header class="masthead">
        <div class="masthead-copy">
          <p class="eyebrow">Waypoint · expedition shell</p>
          <h1>Recon / Attacks / Findings</h1>
          <p class="subtitle">A calm trail map for the audit spine, with route skeletons beneath each waypoint.</p>
        </div>
        <div class="masthead-actions">
          <div class="theme-switcher" role="group" aria-label="Theme selection">
            <button type="button" data-action="theme" data-theme="light" aria-pressed="${state.theme === 'light'}" class="${state.theme === 'light' ? 'is-active' : ''}">Light</button>
            <button type="button" data-action="theme" data-theme="dark" aria-pressed="${state.theme === 'dark'}" class="${state.theme === 'dark' ? 'is-active' : ''}">Dark</button>
          </div>
          <div class="progress-pill" aria-label="Trail progress">Trail ${currentIndex + 1} / ${phaseOrder.length} · ${escapeHtml(active.name)}</div>
          <div class="metrics" aria-label="Engagement progress">
            <div class="metric"><span class="metric-label">Traveled</span><strong>${currentIndex + 1} waypoints</strong></div>
            <div class="metric"><span class="metric-label">To summit</span><strong>${Math.max(0, phaseOrder.length - currentIndex - 1)} left</strong></div>
          </div>
        </div>
      </header>

      <div class="layout">
        <section class="map-column">
          <section class="map-card" aria-label="Engagement trail map">
            <div class="map-stage">
              <svg viewBox="0 0 640 300" role="img" aria-label="Trail map with waypoint buttons">
                <rect width="640" height="300" class="map-terrain" />
                <path d="M60 252 C 132 234, 148 194, 206 182 C 270 168, 286 220, 350 204 C 402 190, 420 148, 472 126 C 516 108, 548 84, 590 60" class="trail-path" />
                <path d="M0 84 Q 138 44, 250 76 T 470 62 T 640 84" class="contours" />
                <path d="M0 136 Q 160 94, 286 124 T 640 112" class="contours" />
                <path d="M0 192 Q 170 160, 332 184 T 640 166" class="contours" />
                <g class="trees" aria-hidden="true">
                  <path d="M92 244 L100 224 L108 244 Z" />
                  <path d="M118 250 L128 228 L138 250 Z" />
                  <path d="M182 96 L190 76 L198 96 Z" />
                  <path d="M510 248 L519 230 L528 248 Z" />
                  <path d="M536 110 L546 88 L556 110 Z" />
                </g>
                ${waypointsMarkup(waypoints, state.activePhase)}
              </svg>
              <div class="waypoint-overlay" aria-label="Trail waypoint shortcuts">
                ${waypoints.map((waypoint) => `<button type="button" class="waypoint-hitbox ${waypoint.id === state.activePhase ? 'is-active' : ''}" data-action="phase" data-phase="${waypoint.id}"${waypoint.id === state.activePhase ? ' aria-current="step"' : ''} aria-label="${escapeHtml(waypoint.name)}, ${waypoint.stateName}${waypoint.id === state.activePhase ? ', you are here' : ''}" style="left:${(waypoint.x / 640) * 100}%;top:${(waypoint.y / 300) * 100}%;"></button>`).join('')}
              </div>
            </div>
          </section>

          <section class="workspace-panel" aria-label="${escapeHtml(active.name)} route skeleton">
            <div class="workspace-header">
              <div>
                <p class="workspace-kicker">Stage ${currentIndex + 1} of ${phaseOrder.length}</p>
                <h2>${escapeHtml(active.workspaceTitle)}</h2>
              </div>
              <p class="workspace-status">Saved 2 min ago</p>
            </div>
            <p class="workspace-lede">${escapeHtml(active.workspaceLede)}</p>
            <div class="workspace-grid">
              ${active.cards.map((card) => `<section class="skeleton-card"><h3>${escapeHtml(card.title)}</h3><ul>${card.items.map((item) => `<li>${escapeHtml(item)}</li>`).join('')}</ul></section>`).join('')}
            </div>
            <div class="workspace-footer">
              <button type="button" class="secondary-link" data-action="phase" data-phase="${phaseOrder[Math.max(0, currentIndex - 1)]}">Back to ${escapeHtml(phaseData[phaseOrder[Math.max(0, currentIndex - 1)]].name)}</button>
              <button type="button" class="primary-button" data-action="phase" data-phase="${phaseOrder[Math.min(phaseOrder.length - 1, currentIndex + 1)]}">Continue to ${escapeHtml(phaseData[phaseOrder[Math.min(phaseOrder.length - 1, currentIndex + 1)]].name)} →</button>
            </div>
          </section>
        </section>

        <aside class="sidebar" aria-label="Guide and trail details">
          <nav class="route-nav" aria-label="Engagement waypoints">
            <div class="panel-heading">
              <h2>Waypoints</h2>
              <p>All phases stay accessible; fog means no data discovered yet.</p>
            </div>
            <ol>
              ${waypoints.map((waypoint, index) => `<li><button type="button" class="route-link ${waypoint.id === state.activePhase ? 'is-active' : ''}" data-action="phase" data-phase="${waypoint.id}"${waypoint.id === state.activePhase ? ' aria-current="step"' : ''}><span class="route-link-copy"><strong>${escapeHtml(waypoint.name)}</strong><span>Stage ${index + 1} of ${phaseOrder.length}</span></span><span class="route-status ${waypoint.stateName}">${waypoint.stateName === 'fog' ? 'Fog' : waypoint.stateName === 'current' ? 'Here' : 'Done'}</span></button></li>`).join('')}
            </ol>
          </nav>

          <section class="guide-panel artifact" aria-label="Guide's note">
            <div class="panel-icon" aria-hidden="true">🧭</div>
            <div>
              <h2>Guide's note</h2>
              <p>${escapeHtml(active.note)}</p>
              <button type="button" class="primary-button" data-action="phase" data-phase="${phaseOrder[Math.min(phaseOrder.length - 1, currentIndex + 1)]}">${currentIndex < phaseOrder.length - 1 ? `Continue into ${escapeHtml(phaseData[phaseOrder[currentIndex + 1]].name)} →` : `Return to ${escapeHtml(phaseData[phaseOrder[currentIndex - 1]].name)} →`}</button>
            </div>
          </section>

          <section class="log-panel feed-panel" aria-label="Journey log">
            <div class="panel-heading compact">
              <div>
                <h2>📖 Journey log</h2>
                <p>The audit trail is the journey log — exact actor, source, target, and result stay in view.</p>
              </div>
              <div class="status-pill ${feedStatusTone()}">${escapeHtml(feedStatusLabel())}</div>
            </div>

            <div class="feed-toolbar">
              <label class="field-group">
                <span>Actor</span>
                <select data-action="filter" data-filter="actor">${buildSelectOptions(state.filters.actor, actorValues, (v) => v.replace('::', ' · '))}</select>
              </label>
              <label class="field-group">
                <span>Source</span>
                <select data-action="filter" data-filter="source">${buildSelectOptions(state.filters.source, sourceValues)}</select>
              </label>
              <label class="field-group">
                <span>Target</span>
                <select data-action="filter" data-filter="target">${buildSelectOptions(state.filters.target, targetValues)}</select>
              </label>
              <label class="field-group">
                <span>Result</span>
                <select data-action="filter" data-filter="result">${buildSelectOptions(state.filters.result, resultValues, (v) => v)}</select>
              </label>
              <label class="field-group field-search">
                <span>Search</span>
                <input type="search" data-action="filter" data-filter="q" value="${escapeHtml(state.filters.q)}" placeholder="Search actor, target, result, or preview" />
              </label>
              <label class="field-group field-token">
                <span>Live token</span>
                <input type="password" data-action="token" value="${escapeHtml(state.liveToken)}" placeholder="Paste bearer token to connect live" autocomplete="off" />
              </label>
              <div class="feed-actions">
                <button type="button" class="secondary-link" data-action="demo">Use demo</button>
                <button type="button" class="primary-button" data-action="connect-live">Connect live</button>
              </div>
            </div>

            ${state.resyncLink ? `<div class="feed-banner review">Cursor gap detected. Resync the trail from persisted history, then reconnect live.</div>` : ''}

            <div class="feed-shell">
              <ol class="audit-feed" aria-label="Audit rows" role="list">
                ${state.visibleRows.map((row) => auditRowMarkup(row, row.id === state.selectedId)).join('')}
              </ol>
              <aside class="audit-detail" aria-live="polite">
                ${selected ? auditDetailMarkup(selected) : '<p class="empty-state">No audit rows are visible yet. Relax a filter or connect the feed.</p>'}
              </aside>
            </div>

            <div class="feed-footer">
              <button type="button" class="secondary-link" data-action="load-more" ${state.loading || !state.hasMore ? 'disabled' : ''}>${state.loading ? 'Loading…' : state.hasMore ? 'Load next batch' : 'No more historical rows'}</button>
              <button type="button" class="secondary-link" data-action="resync" ${state.resyncLink ? '' : 'disabled'}>Resync</button>
              <span class="feed-note">${state.mode === 'live' ? 'Live rows stream in via SSE; reconnect keeps the latest cursor.' : 'Safe previews only — raw bytes stay out of the row and in the bundle.'}</span>
            </div>
          </section>

          <section class="route-summary" aria-label="Route summary">
            <div>
              <p class="metric-label">Current waypoint</p>
              <strong>${escapeHtml(active.name)}</strong>
            </div>
            <p>Audit rows stay attributable while the expedition shell keeps the woodland chrome in the margins.</p>
          </section>
        </aside>
      </div>
    </main>
  `;

  bindUI();
}

function waypointsMarkup(waypoints, activePhase) {
  return waypoints.map((waypoint) => {
    if (waypoint.stateName === 'current') {
      return `
        <g class="waypoint campfire-node is-current" role="button" tabindex="0" data-action="phase" data-phase="${waypoint.id}" aria-current="step" aria-label="${escapeHtml(waypoint.name)}, current stage. You are here.">
          <circle cx="${waypoint.x}" cy="${waypoint.y}" r="17" fill="#EF9F27" stroke="#FAC775" stroke-width="3" />
          <path d="M${waypoint.x} ${waypoint.y - 8} c -4 5 -5 8 -2 11 c -3 0 -5 3 -2 5 c 2 2 7 2 9 0 c 3 -2 1 -5 -2 -5 c 3 -3 1 -7 -3 -11" class="campfire" />
          <text x="${waypoint.x}" y="${waypoint.y + 34}" text-anchor="middle" font-size="11" font-weight="600" fill="#633806">${escapeHtml(waypoint.name)} — you are here</text>
        </g>`;
    }
    if (waypoint.stateName === 'completed') {
      return `
        <g class="waypoint" role="button" tabindex="0" data-action="phase" data-phase="${waypoint.id}" aria-label="${escapeHtml(waypoint.name)}, completed. Open to revisit.">
          <circle cx="${waypoint.x}" cy="${waypoint.y}" r="12" fill="#639922" />
          <path d="M${waypoint.x - 4} ${waypoint.y} l3 3 l6 -6" class="checkmark" />
          <text x="${waypoint.x}" y="${waypoint.y + 28}" text-anchor="middle" font-size="11" fill="#5F5E5A">${escapeHtml(waypoint.name)}</text>
        </g>`;
    }
    return `
      <g class="waypoint locked" opacity="0.55" data-action="phase" data-phase="${waypoint.id}" aria-label="${escapeHtml(waypoint.name)}, fogged until data lands here.">
        <circle cx="${waypoint.x}" cy="${waypoint.y}" r="12" fill="#B4A78C" stroke="#8B7355" stroke-width="1.5" />
        <path d="M${waypoint.x - 4} ${waypoint.y - 4} v8 M${waypoint.x - 4} ${waypoint.y - 4} h7 l-2.5 2.5 l2.5 2.5 h-7" stroke="#4A3A28" stroke-width="1.5" fill="none" />
        <text x="${waypoint.x}" y="${waypoint.y + 28}" text-anchor="middle" font-size="11" fill="#8B8069">${escapeHtml(waypoint.name)}</text>
      </g>`;
  }).join('');
}

function auditRowMarkup(row, selected) {
  const evidence = row.evidence;
  return `
    <li class="audit-row ${selected ? 'is-selected' : ''}">
      <button type="button" class="audit-row-button" data-action="select-row" data-row-id="${escapeHtml(row.id)}"${selected ? ' aria-current="true"' : ''}>
        <div class="audit-row-top">
          <span class="audit-row-time">${escapeHtml(formatTime(row.occurredAt))}</span>
          <span class="audit-row-actor">${escapeHtml(row.actor.handle)}</span>
          <span class="audit-chip ${escapeHtml(row.actor.kind)}">${escapeHtml(row.actor.kind)}</span>
        </div>
        <div class="audit-row-main">
          <span class="audit-field"><strong>Source</strong><span>${escapeHtml(row.source)}</span></span>
          <span class="audit-field"><strong>Target</strong><span>${escapeHtml(row.target)}</span></span>
          <span class="audit-field"><strong>Result</strong><span class="result-pill ${resultClass(row.result.kind)}">${escapeHtml(row.result.label)}</span></span>
        </div>
        <div class="audit-row-note">${escapeHtml(row.summary)}</div>
        <div class="audit-row-foot">
          <span>Subject ${escapeHtml(row.subject.type)} · ${escapeHtml(row.subject.id)} · v${escapeHtml(row.subject.revision)}</span>
          <span>Request ${escapeHtml(row.requestId)} · Cursor ${escapeHtml(row.id)}</span>
        </div>
      </button>
    </li>
  `;
}

function auditDetailMarkup(row) {
  const evidence = row.evidence;
  return `
    <div class="detail-card">
      <div class="detail-head">
        <div>
          <p class="workspace-kicker">Audit-row parity</p>
          <h3>${escapeHtml(row.actor.handle)} · ${escapeHtml(row.result.label)}</h3>
        </div>
        <span class="result-pill ${resultClass(row.result.kind)}">${escapeHtml(row.result.label)}</span>
      </div>
      <dl class="detail-grid">
        <div><dt>Actor</dt><dd>${escapeHtml(row.actor.kind)} · ${escapeHtml(row.actor.handle)}${row.actor.agentName ? ` · ${escapeHtml(row.actor.agentName)}` : ''}</dd></div>
        <div><dt>Source</dt><dd>${escapeHtml(row.source)}</dd></div>
        <div><dt>Target</dt><dd>${escapeHtml(row.target)}</dd></div>
        <div><dt>Result</dt><dd>${escapeHtml(row.result.detail || row.result.label)}</dd></div>
        <div><dt>Origin</dt><dd>${escapeHtml(row.origin.kind)}${row.origin.service ? ` · ${escapeHtml(row.origin.service)}` : ''}</dd></div>
        <div><dt>Timing</dt><dd>${escapeHtml(formatTime(row.occurredAt))}</dd></div>
        <div><dt>Request</dt><dd>${escapeHtml(row.requestId)}</dd></div>
        <div><dt>Correlation</dt><dd>${escapeHtml(row.correlationId)}</dd></div>
      </dl>
      <div class="evidence-box">
        <div class="evidence-head">
          <h4>Safe evidence</h4>
          <button type="button" class="secondary-link" data-action="copy-hash" data-hash="${escapeHtml(evidence.sha256)}" ${evidence.sha256 ? '' : 'disabled'}>Copy hash</button>
        </div>
        <p>Raw bytes never render here. The row shows a safe preview, hash, and size only.</p>
        <pre>${escapeHtml(evidence.safePreview || 'No safe preview captured.')}</pre>
        <div class="evidence-meta">
          <span>${escapeHtml(evidence.kind)}</span>
          <span>${evidence.byteLength ? `${evidence.byteLength} bytes` : 'Size unknown'}</span>
          <span>${escapeHtml(evidence.mediaType)}</span>
        </div>
        <code>${escapeHtml(evidence.sha256 || 'No hash')}</code>
      </div>
      <div class="detail-foot">
        <span>Contract ${escapeHtml(row.contractVersion)}</span>
        <span>Subject revision ${escapeHtml(row.subject.revision)}</span>
      </div>
    </div>
  `;
}

function bindUI() {
  root.querySelectorAll('[data-action="theme"]').forEach((button) => {
    button.addEventListener('click', () => setTheme(button.dataset.theme));
  });
  root.querySelectorAll('[data-action="phase"]').forEach((button) => {
    button.addEventListener('click', () => setPhase(button.dataset.phase));
    button.addEventListener('keydown', (event) => {
      if (event.key === 'Enter' || event.key === ' ') {
        event.preventDefault();
        setPhase(button.dataset.phase);
      }
    });
  });
  root.querySelectorAll('[data-action="filter"]').forEach((control) => {
    control.addEventListener('input', onFilterChange);
    control.addEventListener('change', onFilterChange);
  });
  root.querySelector('[data-action="token"]')?.addEventListener('input', (event) => {
    state.liveToken = event.target.value;
  });
  root.querySelector('[data-action="connect-live"]')?.addEventListener('click', () => connectLive());
  root.querySelector('[data-action="demo"]')?.addEventListener('click', () => primeDemoFeed(true));
  root.querySelector('[data-action="load-more"]')?.addEventListener('click', () => loadNextBatch());
  root.querySelector('[data-action="resync"]')?.addEventListener('click', () => resyncFromGap());
  root.querySelectorAll('[data-action="select-row"]').forEach((button) => {
    button.addEventListener('click', () => {
      state.selectedId = button.dataset.rowId;
      scheduleRender();
    });
  });
  root.querySelectorAll('[data-action="copy-hash"]').forEach((button) => {
    button.addEventListener('click', async () => {
      const hash = button.dataset.hash || '';
      if (!hash) return;
      try {
        await navigator.clipboard.writeText(hash);
        button.textContent = 'Copied';
        setTimeout(() => scheduleRender(), 900);
      } catch {
        button.textContent = 'Copy failed';
        setTimeout(() => scheduleRender(), 900);
      }
    });
  });
}

function onFilterChange(event) {
  const { filter } = event.target.dataset;
  if (!filter) return;
  state.filters[filter] = event.target.value;
  refreshVisibleRows();
  scheduleRender();
}

function primeDemoFeed(clearToken = false) {
  cleanupStream();
  state.mode = 'demo';
  if (clearToken) {
    state.liveToken = '';
    safeStorageSet('waypoint-audit-token', '');
  }
  setRows(demoRows.slice(0, demoPageSize), {
    append: false,
    pageCursor: demoRows[demoPageSize - 1]?.id || demoRows[0]?.id || null,
    highWaterCursor: demoRows[demoRows.length - 1]?.id || null,
    hasMore: demoRows.length > demoPageSize,
  });
  state.demoCursor = demoPageSize;
  scheduleRender();
}

function loadNextBatch() {
  if (state.loading) return;
  if (state.mode === 'demo') {
    const slice = demoRows.slice(state.demoCursor, state.demoCursor + demoPageSize);
    if (!slice.length) return;
    state.demoCursor += slice.length;
    setRows(slice, {
      append: true,
      pageCursor: state.demoCursor < demoRows.length ? demoRows[state.demoCursor - 1]?.id : demoRows[demoRows.length - 1]?.id,
      highWaterCursor: demoRows[demoRows.length - 1]?.id,
      hasMore: state.demoCursor < demoRows.length,
    });
    scheduleRender();
    return;
  }
  if (state.pageCursor) {
    loadAuditPage(state.pageCursor, true);
  }
}

function cleanupStream() {
  if (state.streamAbort) {
    state.streamAbort.abort();
    state.streamAbort = null;
  }
  if (state.reconnectTimer) {
    clearTimeout(state.reconnectTimer);
    state.reconnectTimer = null;
  }
}

async function connectLive() {
  cleanupStream();
  if (!state.liveToken.trim()) {
    state.mode = 'error';
    state.resyncLink = '';
    scheduleRender();
    return;
  }
  safeStorageSet('waypoint-audit-token', state.liveToken.trim());
  await loadAuditPage(null, false, true);
}

async function loadAuditPage(after, append, liveMode = false) {
  state.loading = true;
  state.mode = liveMode ? 'connecting' : state.mode;
  scheduleRender();

  const token = state.liveToken.trim();
  const headers = new Headers({ 'Waypoint-Contract-Version': '1.0.0', 'X-Request-ID': `waypoint-${Date.now()}` });
  if (token) headers.set('Authorization', `Bearer ${token}`);
  const url = new URL('/api/v1/audit-events', window.location.origin);
  url.searchParams.set('limit', '8');
  if (after) url.searchParams.set('after', after);

  try {
    const resp = token ? await fetch(url, { headers }) : null;
    if (!token || !resp || !resp.ok) {
      if (!token) {
        state.loading = false;
        primeDemoFeed(true);
        return;
      }
      if (resp && resp.status === 410) {
        const problem = await resp.json();
        state.mode = 'resync';
        state.resyncLink = problem.resync || '';
        state.loading = false;
        scheduleRender();
        return;
      }
      throw new Error(resp ? `${resp.status}` : 'offline');
    }
    const page = await resp.json();
    const items = Array.isArray(page.items) ? page.items : [];
    setRows(items, {
      append,
      pageCursor: page.page?.nextCursor || (items[items.length - 1] && items[items.length - 1].id) || after || null,
      highWaterCursor: page.page?.highWaterCursor || (items[items.length - 1] && items[items.length - 1].id) || null,
      hasMore: Boolean(page.page?.hasMore),
    });
    state.mode = liveMode ? 'live' : 'demo';
    state.loading = false;
    scheduleRender();
    if (liveMode) {
      startSse(page.page?.highWaterCursor || page.page?.nextCursor || items[items.length - 1]?.id || after || null);
    }
  } catch (error) {
    if (!token) {
      primeDemoFeed(true);
      return;
    }
    state.loading = false;
    state.mode = 'error';
    state.resyncLink = '';
    scheduleRender();
  }
}

function startSse(after) {
  cleanupStream();
  if (!state.liveToken.trim()) return;
  const controller = new AbortController();
  state.streamAbort = controller;
  const headers = new Headers({ 'Waypoint-Contract-Version': '1.0.0', 'X-Request-ID': `waypoint-stream-${Date.now()}`, Authorization: `Bearer ${state.liveToken.trim()}` });
  const url = new URL('/events', window.location.origin);
  if (after) url.searchParams.set('after', after);
  fetch(url, { headers, signal: controller.signal }).then(async (resp) => {
    if (resp.status === 410) {
      const problem = await resp.json();
      state.mode = 'resync';
      state.resyncLink = problem.resync || '';
      state.loading = false;
      scheduleRender();
      return;
    }
    if (!resp.ok || !resp.body) {
      throw new Error(`stream ${resp.status}`);
    }
    state.mode = 'live';
    state.loading = false;
    scheduleRender();
    await consumeSse(resp.body, (item) => {
      const normalized = normalizeRow(item);
      state.rows = dedupeRows([...state.rows, normalized]);
      state.highWaterCursor = normalized.id;
      state.selectedId = normalized.id;
      refreshVisibleRows();
      scheduleRender();
    });
    if (!controller.signal.aborted) {
      scheduleReconnect(state.highWaterCursor);
    }
  }).catch(() => {
    if (controller.signal.aborted) return;
    state.mode = 'reconnecting';
    state.loading = false;
    scheduleRender();
    scheduleReconnect(state.highWaterCursor);
  });
}

async function consumeSse(stream, onItem) {
  const reader = stream.getReader();
  const decoder = new TextDecoder();
  let buffer = '';
  let current = { id: '', event: '', data: '' };
  while (true) {
    const { value, done } = await reader.read();
    if (done) break;
    buffer += decoder.decode(value, { stream: true });
    let boundary = buffer.indexOf('\n\n');
    while (boundary >= 0) {
      const frame = buffer.slice(0, boundary);
      buffer = buffer.slice(boundary + 2);
      current = parseSseFrame(frame);
      if (current.data) {
        try {
          onItem(JSON.parse(current.data));
        } catch {
          /* ignore malformed payload */
        }
      }
      boundary = buffer.indexOf('\n\n');
    }
  }
}

function parseSseFrame(frame) {
  const out = { id: '', event: '', data: '' };
  for (const line of frame.split('\n')) {
    if (!line || line.startsWith(':')) continue;
    const index = line.indexOf(':');
    if (index === -1) continue;
    const key = line.slice(0, index).trim();
    const value = line.slice(index + 1).trimStart();
    if (key === 'id') out.id = value;
    if (key === 'event') out.event = value;
    if (key === 'data') out.data += (out.data ? '\n' : '') + value;
  }
  return out;
}

function scheduleReconnect(after) {
  if (state.reconnectTimer) clearTimeout(state.reconnectTimer);
  state.mode = 'reconnecting';
  scheduleRender();
  state.reconnectTimer = setTimeout(() => {
    if (!state.liveToken.trim()) return;
    startSse(after);
  }, 1500);
}

async function resyncFromGap() {
  if (!state.resyncLink || !state.liveToken.trim()) return;
  state.loading = true;
  scheduleRender();
  const headers = new Headers({ 'Waypoint-Contract-Version': '1.0.0', 'X-Request-ID': `waypoint-resync-${Date.now()}`, Authorization: `Bearer ${state.liveToken.trim()}` });
  try {
    const resp = await fetch(state.resyncLink, { headers });
    if (!resp.ok) throw new Error(`resync ${resp.status}`);
    const page = await resp.json();
    setRows(page.items || [], { append: true, pageCursor: page.page?.nextCursor || null, highWaterCursor: page.page?.highWaterCursor || null, hasMore: Boolean(page.page?.hasMore) });
    state.resyncLink = '';
    state.mode = 'live';
    state.loading = false;
    scheduleRender();
    startSse(page.page?.highWaterCursor || state.highWaterCursor || null);
  } catch {
    state.loading = false;
    state.mode = 'error';
    scheduleRender();
  }
}

window.addEventListener('popstate', () => {
  const match = window.location.pathname.match(/^\/engagements\/[^/]+\/(recon|attacks|findings|summit)\/?$/);
  if (match) {
    state.activePhase = match[1];
    scheduleRender();
  }
});

window.addEventListener('beforeunload', cleanupStream);

function boot() {
  document.documentElement.dataset.theme = state.theme;
  document.title = `Waypoint — ${phaseData[state.activePhase].name}`;
  primeDemoFeed(false);
  render();
  if (state.liveToken.trim()) {
    connectLive();
  }
}

boot();
