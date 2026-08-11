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
    note: 'Grouped attempts stay searchable by technique, target, and host. Raw output is always rendered safely.',
    workspaceTitle: 'Attacks workspace',
    workspaceLede: 'Group attempts by technique, target, and host; inspect status, raw output, and structured evidence without ever rendering tool HTML.',
    cards: [
      { title: 'Capture lane', items: ['Technique, target, host, and status', 'Raw stdout / stderr refs', 'Parse state and timing'] },
      { title: 'Attribution', items: ['Actor identity', 'Exec host + pivot chain', 'Public egress IP when known'] },
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
      { title: 'Report copy', items: ['Client-readable summary', 'Machine-readable bundle', 'Empty signatures hook'] },
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
      { title: 'Export preflight', items: ['Capture keeps flowing during export', 'Hash manifest and receipt are checked', 'Failure can be retried from the last clean step'] },
      { title: 'Verified receipt', items: ['Receipt ID is archived with the report', 'Manifest hash is pinned to the snapshot', 'Evidence and PDF stay attributable'] },
      { title: 'Break-glass teardown', items: ['Destroy only after receipt verification', 'Interactive confirmation is required', 'Guarded destroy keeps the audit trail honest'] },
    ],
  },
};

const reportPath = '/engagements/demo/summit/report';
const reportSnapshot = {
  version: 'v1',
  title: 'Frozen report snapshot',
  engagement: 'Q3 launch',
  cutoff: '2025-01-10T09:00:00Z',
  scope: ['10.10.12.0/24', 'corp.local', 'mail01.internal', 'jumpbox-01'],
  methodology: [
    'Recon: preserve raw discovery and entity provenance.',
    'Attacks: capture every attempt with command, host, IPs, timing, and outcome.',
    'Findings: promote only confirmed results and keep evidence linked.',
    'Export: freeze the snapshot before PDF rendering, bundle manifest generation, and restore verification.',
  ],
  findings: [
    {
      title: 'SMB relay attempt blocked by signing',
      severity: 'Medium',
      evidence: ['Action 103'],
      remediation: 'Keep SMB signing on and review relay exposure on the target segment.',
      summary: 'Relay was stopped by SMB signing on mail01.internal.',
    },
    {
      title: 'AI-authored kerberoast probe stayed attributed',
      severity: 'Low',
      evidence: ['Action 104'],
      remediation: 'Keep AI actor authorization recorded with the same rigor as human operators.',
      summary: 'An AI-initiated action remained linked to the human authorizer and source host.',
    },
  ],
  evidence: [
    {
      label: 'Action 101',
      source: 'nmap -sn 10.10.12.0/24',
      actor: 'alex.operator',
      host: 'jumpbox-01',
      attribution: '10.0.0.12 → 203.0.113.26',
      rawSnippet: 'nmap -sn 10.10.12.0/24\nHost is up',
      note: 'Discovery output preserved as text.',
    },
    {
      label: 'Action 103',
      source: 'ntlmrelayx --target mail01.internal',
      actor: 'alex.operator',
      host: 'jumpbox-01',
      attribution: '10.0.0.12 → 203.0.113.26',
      rawSnippet: 'Relay refused: SMB signing required\n<script>alert("x")</script>',
      note: 'Unsafe raw output stays escaped in the printable snapshot.',
    },
    {
      label: 'Action 104',
      source: 'GetUserSPNs.py corp.local',
      actor: 'field-agent-7 (AI)',
      host: 'field-agent-7',
      attribution: '10.0.0.12 → 203.0.113.26',
      rawSnippet: 'Found 2 service principals',
      note: 'AI action stays attributed.',
    },
  ],
  bundle: {
    payloads: [
      { path: 'bundle/database/engagement.dump', size: 8421, sha256: '9c0f5f1f7f8df02f38b1c7f5129e4f3e6dc44a5b47fdb2fefb8af8a7b4f4d201' },
      { path: 'bundle/evidence/evidence.tar.zst', size: 16384, sha256: '2f2b8e7b40c2b1af5b7c1c2b6e2a0cfd7f4b5d61f7cf7d1de8b8f2f8a9b0c114' },
      { path: 'bundle/report/frozen-report.pdf', size: 24576, sha256: '1e3b1a6f0dfd7c4a1e6e2c9d2b4b7b8f9f6c5d4e3a2b1c0d9e8f7a6b5c4d3e2f' },
      { path: 'bundle/report/report-snapshot.json', size: 6184, sha256: '3f6f9e8d7c5b4a29181716151413121110ffeeddbbccaa998877665544332211' },
      { path: 'bundle/metadata/export-metadata.json', size: 1224, sha256: '7c8d9e0f102132435465768798a9bacbdcedfe0f1e2d3c4b5a69788766554433' },
      { path: 'bundle/tools/verify-restore.mjs', size: 4096, sha256: '5a6b7c8d9eafb0c1d2e3f405162738495a5b6c7d8e9fafb0c1d2e3f4051627384' },
      { path: 'bundle/tools/regenerate-report.mjs', size: 4096, sha256: '4b5c6d7e8f90a1b2c3d4e5f60718293a4b5c6d7e8f90a1b2c3d4e5f60718293a' },
    ],
    outerArchiveSha256: '8e0f1d2c3b4a59687766554433221100ffeeddccbbaa99887766554433221100',
    signatures: {
      version: 'v1',
      items: [],
    },
    restore: {
      tools: ['bundle/tools/verify-restore.mjs', 'bundle/tools/regenerate-report.mjs'],
      cleanRoom: [
        'Verify the outer archive hash before restore.',
        'Reject the manifest if any payload path is missing, duplicated, or traverses upward.',
        'Regenerate the report from the frozen snapshot rather than live queries.',
      ],
      maliciousPaths: ['../escape.dump', '/absolute/report.pdf', 'bundle/../metadata/export-metadata.json'],
    },
  },
  attribution: [
    { title: 'Operator', items: ['alex.operator'] },
    { title: 'AI actor', items: ['field-agent-7 · model gpt-4.1 · authorized by alex.operator'] },
    { title: 'Exec host IP', items: ['10.0.0.12'] },
    { title: 'Public egress IP', items: ['203.0.113.26'] },
  ],
  knownCaptureGaps: [
    'One capture recorded egress as off, so the report notes the gap instead of inventing a public IP.',
    'Unknown tools remain raw-first; a missing parser does not drop evidence.',
  ],
  receipt: {
    id: 'receipt-q3-2025-01-10',
    verifiedAt: '2025-01-10T09:02:14Z',
    captureState: 'Capture remained live while export froze a clean snapshot.',
    manifestHash: '8e0f1d2c3b4a59687766554433221100ffeeddccbbaa99887766554433221100',
    note: 'Verified export receipt kept alongside the bundle so teardown stays defensible.',
  },
};

const guideExplainers = [
  {
    id: 'guide-recon-dns',
    phase: 'recon',
    title: 'DNS discovery',
    summary: 'Use reviewed DNS notes to map hostnames, aliases, and service records before you touch the target.',
    whenToUse: 'Best when names or service patterns will steer the next pass.',
    risks: 'DNS changes quickly; confirm anything important against live evidence before you promote it.',
    contextLabel: 'Open Recon context note',
    contextHref: '#guide-recon-dns',
    keywords: ['dns', 'records', 'names', 'recon'],
  },
  {
    id: 'guide-recon-dedup',
    phase: 'recon',
    title: 'Host deduplication',
    summary: 'Prefer stable identifiers such as MAC, AD SID, or FQDN when multiple sightings point at the same host.',
    whenToUse: 'Best when DHCP churn or duplicated hostnames make the pack ambiguous.',
    risks: 'Never collapse evidence by hand; keep the observation trail visible until the merge is deliberate.',
    contextLabel: 'Review dedup guidance',
    contextHref: '#guide-recon-dedup',
    keywords: ['host', 'dedup', 'merge', 'fqdns'],
  },
  {
    id: 'guide-attacks-smb-signing',
    phase: 'attacks',
    title: 'SMB signing',
    summary: 'Use this to reason about relay risk and why unsigned sessions matter in Windows-heavy environments.',
    whenToUse: 'Best when you are checking share access or auth surfaces.',
    risks: 'Pair the note with the captured wrapper output; the same behaviour can mean different things across hosts.',
    contextLabel: 'Open attacks context note',
    contextHref: '#guide-attacks-smb-signing',
    keywords: ['smb', 'relay', 'signing', 'attacks'],
  },
  {
    id: 'guide-attacks-safe-output',
    phase: 'attacks',
    title: 'Safe output rendering',
    summary: 'Keep raw tool output escaped so hostile HTML, ANSI, or scripts never take over the page.',
    whenToUse: 'Best whenever a tool prints untrusted output or a parser fails.',
    risks: 'Rendered output should stay text-only; the raw artefact belongs in evidence, not the DOM.',
    contextLabel: 'Review output handling',
    contextHref: '#guide-attacks-safe-output',
    keywords: ['output', 'rendering', 'raw', 'safe'],
  },
  {
    id: 'guide-findings-linking',
    phase: 'findings',
    title: 'Evidence-linked promotion',
    summary: 'Promote only confirmed results and keep the source action linked so the report stays defensible.',
    whenToUse: 'Best when an attack has enough proof to become a finding.',
    risks: 'Never drop the action trail; a finding without evidence is just a claim.',
    contextLabel: 'Open promotion note',
    contextHref: '#guide-findings-linking',
    keywords: ['finding', 'evidence', 'promotion', 'report'],
  },
  {
    id: 'guide-summit-manifest',
    phase: 'summit',
    title: 'Bundle manifest',
    summary: 'Export the bundle, verify the hash manifest, and only then tear down the disposable box.',
    whenToUse: 'Best at the final review before the engagement closes.',
    risks: 'If the manifest does not match, stop and inspect the artefacts before wiping anything.',
    contextLabel: 'Review export manifest',
    contextHref: '#guide-summit-manifest',
    keywords: ['bundle', 'manifest', 'export', 'summit'],
  },
];

function makeAttack(id, occurredAt, payload) {
  const actor = payload.actor || { id: 'actor-default', kind: 'human', handle: 'alex.operator', role: 'operator' };
  const origin = payload.origin || { kind: 'collector', service: 'wrapper' };
  const target = payload.target || 'unknown target';
  const host = payload.host || origin.service || actor.handle;
  const technique = payload.technique || 'Unspecified technique';
  const status = payload.status || 'success';
  const rawOutput = payload.rawOutput || payload.summary || `${technique} against ${target}`;
  const structured = payload.structured || {};
  const parseStatus = payload.parseStatus || (Object.keys(structured).length ? 'parsed' : 'raw');
  const command = payload.command || '';
  const evidence = payload.evidence || {};
  const resultLabel = payload.resultLabel || statusToLabel(status);
  const resultDetail = payload.resultDetail || payload.summary || resultLabel;
  const byteLength = payload.byteLength || Math.max(64, rawOutput.length + 120);
  const subjectType = payload.subjectType || 'action';
  const subjectId = payload.subjectId || `attack-${id}`;

  return {
    id,
    contractVersion: '1.0.0',
    type: payload.type || `attack.${technique.toLowerCase().replace(/[^a-z0-9]+/g, '-')}`,
    occurredAt,
    engagementId: 'demo',
    actor,
    origin,
    subject: { type: subjectType, id: subjectId, revision: payload.revision || 1 },
    requestId: payload.requestId || `req-${id}`,
    correlationId: payload.correlationId || `corr-${id}`,
    data: {
      phase: 'attacks',
      source: payload.source || origin.service || origin.kind,
      technique,
      target,
      host,
      status,
      summary: payload.summary || resultDetail,
      command,
      rawOutput,
      structured,
      result: { kind: status, label: resultLabel, detail: resultDetail },
      evidence: {
        kind: evidence.kind || payload.evidenceKind || 'stdout',
        sha256: evidence.sha256 || hashFor(id),
        byteLength: evidence.byteLength || byteLength,
        mediaType: evidence.mediaType || payload.mediaType || 'text/plain',
        safePreview: evidence.safePreview || rawOutput,
      },
      parseStatus,
      pluginId: payload.pluginId || structured.pluginId || '',
      argv: payload.argv || [],
      cwd: payload.cwd || '/home/operator/engagement',
      execHostIp: payload.execHostIp || '10.0.0.12',
      egressPublicIp: payload.egressPublicIp || '198.51.100.14',
      pivotChain: payload.pivotChain || [],
    },
  };
}

function hashFor(id) {
  return String(id).padStart(64, '0').slice(0, 64);
}

const demoRows = [
  makeAttack('101', '2025-01-10T08:12:00Z', {
    actor: { id: 'a1', kind: 'human', handle: 'alex.operator', role: 'operator' },
    origin: { kind: 'collector', service: 'wrapper' },
    technique: 'Discovery sweep',
    target: '10.10.12.0/24',
    host: 'jumpbox-01',
    status: 'success',
    resultDetail: '240 hosts discovered',
    summary: 'Network sweep captured without a parser.',
    command: 'nmap -sn 10.10.12.0/24',
    rawOutput: 'nmap -sn 10.10.12.0/24\nHost is up (0.045s latency)\n240 hosts are up',
    structured: { pluginId: 'plugin.nmap', range: '10.10.12.0/24', hostsUp: 240 },
    argv: ['nmap', '-sn', '10.10.12.0/24'],
    execHostIp: '10.0.0.12',
    egressPublicIp: '203.0.113.26',
    parseStatus: 'parsed',
  }),
  makeAttack('102', '2025-01-10T08:18:00Z', {
    actor: { id: 'a1', kind: 'human', handle: 'alex.operator', role: 'operator' },
    origin: { kind: 'collector', service: 'wrapper' },
    technique: 'DNS enumeration',
    target: 'corp.local',
    host: 'jumpbox-01',
    status: 'success',
    resultDetail: 'Zone records surfaced',
    summary: 'DNS answers stayed text-only and easy to review.',
    command: 'dig axfr corp.local @10.10.12.53',
    rawOutput: 'SOA ns1.corp.local.\nA fileserver02.corp.local 10.10.12.40\nTXT "lab only"',
    structured: { pluginId: 'plugin.dns', records: 3, zoneTransfer: false },
    argv: ['dig', 'axfr', 'corp.local', '@10.10.12.53'],
    execHostIp: '10.0.0.12',
    egressPublicIp: '203.0.113.26',
    parseStatus: 'parsed',
  }),
  makeAttack('103', '2025-01-10T08:24:00Z', {
    actor: { id: 'a1', kind: 'human', handle: 'alex.operator', role: 'operator' },
    origin: { kind: 'collector', service: 'wrapper' },
    technique: 'NTLM relay',
    target: 'mail01.internal',
    host: 'jumpbox-01',
    status: 'blocked',
    resultLabel: 'Blocked',
    resultDetail: 'SMB signing prevented relay',
    summary: 'Attempt logged with a clear failure state.',
    command: 'ntlmrelayx --target mail01.internal',
    rawOutput: 'Relay refused: SMB signing required\nNo credentials were replayed.',
    structured: { pluginId: 'plugin.impacket', policy: 'smb-signing-required', relayPossible: false },
    argv: ['ntlmrelayx', '--target', 'mail01.internal'],
    execHostIp: '10.0.0.12',
    egressPublicIp: '203.0.113.26',
    parseStatus: 'parsed',
  }),
  makeAttack('104', '2025-01-10T08:31:00Z', {
    actor: { id: 'a3', kind: 'ai_agent', handle: 'field-agent-7', role: 'operator', agentName: 'Waypoint', model: 'gpt-4.1', version: '1.0', authorizedBy: 'alex.operator' },
    origin: { kind: 'mcp', service: 'waypoint-core' },
    technique: 'Kerberoast probe',
    target: 'svc/sql-01',
    host: 'field-agent-7',
    status: 'needs-review',
    resultLabel: 'Needs review',
    resultDetail: 'Ticket material needs human confirmation',
    summary: 'AI-authored action kept the human authorizer visible.',
    command: 'GetUserSPNs.py corp.local',
    rawOutput: 'Found 2 service principals\nOne candidate ticket may be roastable',
    structured: { pluginId: 'plugin.impacket', spns: 2, candidate: 'svc/sql-01' },
    argv: ['GetUserSPNs.py', 'corp.local'],
    execHostIp: '10.0.0.37',
    egressPublicIp: '198.51.100.14',
    parseStatus: 'parsed',
  }),
  makeAttack('105', '2025-01-10T08:37:00Z', {
    actor: { id: 'a1', kind: 'human', handle: 'alex.operator', role: 'operator' },
    origin: { kind: 'collector', service: 'remote-agent' },
    technique: 'RDP probe',
    target: 'svc/rdp-3389',
    host: 'remote-agent',
    status: 'success',
    resultDetail: 'New reachable segment confirmed',
    summary: 'Remote agent synced a capture after reconnect.',
    command: 'rdp-check svc/rdp-3389',
    rawOutput: 'RDP banner confirmed\nTLS handshake completed\nNLA accepted',
    structured: { pluginId: 'plugin.rdp', banner: true, nla: true },
    argv: ['rdp-check', 'svc/rdp-3389'],
    execHostIp: '10.10.14.33',
    egressPublicIp: '198.51.100.18',
    parseStatus: 'parsed',
  }),
  makeAttack('106', '2025-01-10T08:43:00Z', {
    actor: { id: 'a2', kind: 'human', handle: 'mira.ops', role: 'analyst' },
    origin: { kind: 'rest', service: 'waypoint-core' },
    technique: 'SMB enumeration',
    target: 'fileserver02',
    host: 'jumpbox-02',
    status: 'success',
    resultDetail: 'Shares and signing policy captured',
    summary: 'Enumeration stayed safely text-only.',
    command: 'smbclient -L //fileserver02 -U student\\alex',
    rawOutput: 'Domain=[LAB] OS=[Windows Server]\nSharename       Type      Comment\nadmin$          Disk      Remote Admin',
    structured: { pluginId: 'plugin.smb', shares: 6, signing: 'required' },
    argv: ['smbclient', '-L', '//fileserver02', '-U', 'student\\alex'],
    execHostIp: '10.0.0.21',
    egressPublicIp: '203.0.113.26',
    parseStatus: 'parsed',
  }),
  makeAttack('107', '2025-01-10T08:48:00Z', {
    actor: { id: 'a1', kind: 'human', handle: 'alex.operator', role: 'operator' },
    origin: { kind: 'collector', service: 'wrapper' },
    technique: 'HTTP auth spray',
    target: 'portal.corp.local',
    host: 'jumpbox-01',
    status: 'blocked',
    resultDetail: 'Rate limit tripped',
    summary: 'Spray attempt blocked and recorded for review.',
    command: 'http-spray portal.corp.local',
    rawOutput: '429 too many requests\nBackoff enforced by target',
    structured: { pluginId: 'plugin.http', rateLimited: true, attempts: 5 },
    argv: ['http-spray', 'portal.corp.local'],
    execHostIp: '10.0.0.12',
    egressPublicIp: '203.0.113.26',
    parseStatus: 'parsed',
  }),
  makeAttack('108', '2025-01-10T08:54:00Z', {
    actor: { id: 'a2', kind: 'human', handle: 'mira.ops', role: 'analyst' },
    origin: { kind: 'collector', service: 'wrapper' },
    technique: 'Privilege check',
    target: 'jumpbox-02',
    host: 'jumpbox-02',
    status: 'queued',
    resultDetail: 'Follow-up command queued',
    summary: 'A follow-up action is waiting on operator confirmation.',
    command: 'whoami /all',
    rawOutput: 'Integrity level: Medium\nLocal groups: Remote Desktop Users',
    structured: { pluginId: 'plugin.windows', elevation: false, groups: 2 },
    argv: ['whoami', '/all'],
    execHostIp: '10.0.0.21',
    egressPublicIp: '198.51.100.18',
    parseStatus: 'parsed',
  }),
  makeAttack('109', '2025-01-10T09:02:00Z', {
    actor: { id: 'a3', kind: 'ai_agent', handle: 'field-agent-7', role: 'operator', agentName: 'Waypoint', model: 'gpt-4.1', version: '1.0', authorizedBy: 'alex.operator' },
    origin: { kind: 'mcp', service: 'waypoint-core' },
    technique: 'Lateral hop check',
    target: '10.10.15.0/24',
    host: 'field-agent-7',
    status: 'success',
    resultDetail: 'Pivot chain confirmed',
    summary: 'Pivot awareness stayed attached to the attempt.',
    command: 'traceroute 10.10.15.20',
    rawOutput: '1 10.0.0.1\n2 10.10.15.1\n3 10.10.15.20',
    structured: { pluginId: 'plugin.trace', hops: 3, pivotChain: ['10.0.0.1', '10.10.15.1'] },
    argv: ['traceroute', '10.10.15.20'],
    execHostIp: '10.0.0.37',
    egressPublicIp: '198.51.100.14',
    parseStatus: 'parsed',
  }),
];

const demoPageSize = 4;
const initialRoute = getInitialRoute();
const state = {
  theme: getInitialTheme(),
  view: initialRoute.view,
  activePhase: initialRoute.phase,
  rows: [],
  visibleRows: [],
  selectedId: null,
  pageCursor: null,
  highWaterCursor: null,
  hasMore: false,
  filters: { technique: 'all', target: 'all', host: 'all', status: 'all', q: '' },
  guideQuery: '',
  mode: 'demo',
  liveToken: safeStorageGet('waypoint-audit-token') || '',
  resyncLink: '',
  loading: false,
  streamAbort: null,
  reconnectTimer: null,
  demoCursor: 0,
  demoPulseTimer: null,
  renderScheduled: false,
  banner: '',
  summitExportStatus: 'idle',
  breakGlassArmed: false,
  destroyPhrase: '',
  teardownState: 'idle',
  summitTimers: [],
};

function safeStorageGet(key) {
  try {
    return window.localStorage.getItem(key) || '';
  } catch {
    return '';
  }
}

function safeStorageSet(key, value) {
  try {
    if (value === '' || value == null) {
      window.localStorage.removeItem(key);
      return;
    }
    window.localStorage.setItem(key, value);
  } catch {
    // ignore
  }
}

function getInitialTheme() {
  if (typeof window === 'undefined') return 'light';
  const stored = safeStorageGet('waypoint-theme');
  if (stored === 'light' || stored === 'dark') return stored;
  return window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light';
}

function getInitialRoute() {
  if (typeof window === 'undefined') return { view: 'trail', phase: 'attacks' };
  if (/^\/engagements\/[^/]+\/summit\/report\/?$/.test(window.location.pathname)) {
    return { view: 'report', phase: 'summit' };
  }
  const match = window.location.pathname.match(/^\/engagements\/[^/]+\/(recon|attacks|findings|summit)\/?$/);
  return { view: 'trail', phase: match ? match[1] : 'attacks' };
}

function phasePath(phase) {
  return phaseData[phase].path;
}

function reportPathFor() {
  return reportPath;
}

function navigateToPhase(phase, replace = false) {
  const path = phasePath(phase);
  if (replace) {
    window.history.replaceState({}, '', path);
    return;
  }
  window.history.pushState({}, '', path);
}

function navigateToReport(replace = false) {
  const path = reportPathFor();
  if (replace) {
    window.history.replaceState({}, '', path);
    return;
  }
  window.history.pushState({}, '', path);
}

function clearSummitTimers() {
  for (const timer of state.summitTimers) {
    clearTimeout(timer);
  }
  state.summitTimers = [];
}

function startSummitExport() {
  clearSummitTimers();
  state.summitExportStatus = 'preflight';
  state.teardownState = 'idle';
  const preflightTimer = setTimeout(() => {
    if (state.summitExportStatus === 'preflight') {
      state.summitExportStatus = 'exporting';
      scheduleRender();
      const exportTimer = setTimeout(() => {
        if (state.summitExportStatus === 'exporting') {
          state.summitExportStatus = 'verified';
          scheduleRender();
        }
      }, 1200);
      state.summitTimers.push(exportTimer);
    }
    scheduleRender();
  }, 420);
  state.summitTimers.push(preflightTimer);
  scheduleRender();
}

function formatTime(iso) {
  return new Intl.DateTimeFormat(undefined, { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' }).format(new Date(iso));
}

function escapeHtml(value) {
  return String(value ?? '')
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#39;');
}

function parseData(value) {
  if (typeof value === 'string') {
    try {
      return JSON.parse(value);
    } catch {
      return { raw: value };
    }
  }
  return value && typeof value === 'object' ? value : {};
}

function statusToLabel(status) {
  switch (String(status || '').toLowerCase()) {
    case 'success':
      return 'Success';
    case 'blocked':
      return 'Blocked';
    case 'needs-review':
    case 'review':
      return 'Needs review';
    case 'queued':
      return 'Queued';
    case 'raw':
      return 'Raw';
    default:
      return String(status || 'Unknown')
        .replace(/[-_]+/g, ' ')
        .replace(/\b\w/g, (match) => match.toUpperCase());
  }
}

function statusClass(status) {
  switch (String(status || '').toLowerCase()) {
    case 'success':
      return 'success';
    case 'blocked':
      return 'blocked';
    case 'needs-review':
    case 'review':
      return 'review';
    case 'queued':
      return 'queued';
    case 'raw':
      return 'raw';
    default:
      return 'neutral';
  }
}

function normalizeAttempt(row) {
  const data = parseData(row.data);
  const evidence = parseData(data.evidence);
  const structured = parseData(data.structured);
  const target = String(data.target || row.subject?.id || 'unknown target');
  const host = String(data.host || data.execHost || row.origin?.service || row.actor?.handle || 'unknown host');
  const technique = String(data.technique || data.tactic || data.command || row.type || 'Unspecified technique');
  const status = String(data.status || data.result?.kind || 'success').toLowerCase();
  const rawOutput = String(data.rawOutput || evidence.safePreview || data.summary || 'No raw output captured.');
  const summary = String(data.summary || data.result?.detail || `${technique} against ${target}`);
  const result = data.result && typeof data.result === 'object'
    ? data.result
    : { kind: status, label: statusToLabel(status), detail: summary };

  return {
    ...row,
    data,
    evidence: {
      kind: String(evidence.kind || 'stdout'),
      sha256: String(evidence.sha256 || hashFor(row.id)),
      byteLength: Number(evidence.byteLength || Math.max(64, rawOutput.length + 120)),
      mediaType: String(evidence.mediaType || 'text/plain'),
      safePreview: String(evidence.safePreview || rawOutput),
    },
    structured,
    technique,
    target,
    host,
    status,
    statusLabel: statusToLabel(status),
    summary,
    rawOutput,
    result,
    command: String(data.command || ''),
    parseStatus: String(data.parseStatus || (Object.keys(structured).length ? 'parsed' : 'raw')),
    pluginId: String(data.pluginId || structured.pluginId || ''),
  };
}

function hashFor(id) {
  return String(id).padStart(64, '0').slice(0, 64);
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

function setRows(rows, meta = {}) {
  const normalized = rows.map(normalizeAttempt);
  state.rows = meta.append ? dedupeRows([...state.rows, ...normalized]) : normalized;
  state.pageCursor = meta.pageCursor ?? state.pageCursor;
  state.highWaterCursor = meta.highWaterCursor ?? state.highWaterCursor;
  state.hasMore = Boolean(meta.hasMore);
  state.selectedId = state.selectedId || state.rows[0]?.id || null;
  refreshVisibleRows();
}

function uniqueValues(selector) {
  return [...new Set(state.rows.map(selector).filter(Boolean))].sort((a, b) => String(a).localeCompare(String(b)));
}

function refreshVisibleRows() {
  const q = state.filters.q.trim().toLowerCase();
  state.visibleRows = state.rows.filter((row) => {
    if (state.filters.technique !== 'all' && row.technique.toLowerCase() !== state.filters.technique) return false;
    if (state.filters.target !== 'all' && row.target.toLowerCase() !== state.filters.target) return false;
    if (state.filters.host !== 'all' && row.host.toLowerCase() !== state.filters.host) return false;
    if (state.filters.status !== 'all' && row.status.toLowerCase() !== state.filters.status) return false;
    if (q) {
      const structured = JSON.stringify(row.structured || {}).toLowerCase();
      const haystack = [
        row.actor.handle,
        row.actor.kind,
        row.actor.agentName,
        row.technique,
        row.target,
        row.host,
        row.statusLabel,
        row.summary,
        row.result?.detail,
        row.command,
        row.rawOutput,
        structured,
      ]
        .filter(Boolean)
        .join(' ')
        .toLowerCase();
      if (!haystack.includes(q)) return false;
    }
    return true;
  });

  if (!state.visibleRows.some((row) => row.id === state.selectedId)) {
    state.selectedId = state.visibleRows[0]?.id || state.rows[0]?.id || null;
  }
}

function selectedAttempt() {
  return state.visibleRows.find((row) => row.id === state.selectedId) || state.rows.find((row) => row.id === state.selectedId) || state.visibleRows[0] || state.rows[0] || null;
}

function groupAttempts(rows) {
  const buckets = new Map();
  for (const row of rows) {
    const key = row.technique || 'Unspecified technique';
    if (!buckets.has(key)) {
      buckets.set(key, { key, rows: [], targets: new Set(), hosts: new Set(), counts: new Map(), latest: row });
    }
    const bucket = buckets.get(key);
    bucket.rows.push(row);
    bucket.targets.add(row.target);
    bucket.hosts.add(row.host);
    bucket.counts.set(row.status, (bucket.counts.get(row.status) || 0) + 1);
    if (Number(row.id) > Number(bucket.latest.id)) bucket.latest = row;
  }

  return [...buckets.values()]
    .map((bucket) => ({
      ...bucket,
      rows: bucket.rows.sort((a, b) => Number(b.id) - Number(a.id)),
    }))
    .sort((a, b) => Number(b.latest.id) - Number(a.latest.id));
}

function feedStatusLabel() {
  if (state.loading) return 'Loading';
  if (state.mode === 'connecting') return 'Connecting';
  if (state.mode === 'live') return `Live SSE · ${state.highWaterCursor || 'cursor pending'}`;
  if (state.mode === 'reconnecting') return `Reconnecting · cursor ${state.highWaterCursor || 'pending'}`;
  if (state.mode === 'resync') return 'Resync required';
  if (state.mode === 'error') return 'Live feed unavailable';
  if (state.mode === 'demo-live') return `Demo feed · live updates`;
  return `Demo feed · ${state.rows.length} attempts`;
}

function feedStatusTone() {
  if (state.mode === 'live') return 'success';
  if (state.mode === 'reconnecting') return 'queued';
  if (state.mode === 'resync') return 'review';
  if (state.mode === 'error') return 'blocked';
  if (state.mode === 'demo-live') return 'success';
  return 'neutral';
}

function scheduleRender() {
  if (state.renderScheduled) return;
  state.renderScheduled = true;
  requestAnimationFrame(() => {
    state.renderScheduled = false;
    render();
  });
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
  if (state.demoPulseTimer) {
    clearTimeout(state.demoPulseTimer);
    state.demoPulseTimer = null;
  }
}

function scheduleDemoPulse() {
  if (state.mode !== 'demo' && state.mode !== 'demo-live') return;
  if (state.demoPulseTimer) clearTimeout(state.demoPulseTimer);
  state.demoPulseTimer = setTimeout(() => {
    if (state.mode !== 'demo' && state.mode !== 'demo-live') return;
    const next = demoRows[state.demoCursor];
    if (!next) return;
    state.demoCursor += 1;
    setRows([next], { append: true, highWaterCursor: next.id, pageCursor: next.id, hasMore: state.demoCursor < demoRows.length });
    state.mode = 'demo-live';
    state.banner = `Live update: ${next.data?.technique || 'an attempt'} against ${next.data?.target || 'a target'}`;
    state.selectedId = next.id;
    scheduleRender();
    scheduleDemoPulse();
  }, 2800);
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
    highWaterCursor: demoRows[demoPageSize - 1]?.id || demoRows[0]?.id || null,
    hasMore: demoRows.length > demoPageSize,
  });
  state.demoCursor = demoPageSize;
  state.banner = 'Demo feed primed; live updates will trickle in.';
  scheduleRender();
  scheduleDemoPulse();
}

function loadNextBatch() {
  if (state.loading) return;
  if (state.mode === 'demo' || state.mode === 'demo-live') {
    const slice = demoRows.slice(state.demoCursor, state.demoCursor + demoPageSize);
    if (!slice.length) return;
    state.demoCursor += slice.length;
    setRows(slice, {
      append: true,
      pageCursor: state.demoCursor < demoRows.length ? demoRows[state.demoCursor - 1]?.id : demoRows[demoRows.length - 1]?.id,
      highWaterCursor: demoRows[demoRows.length - 1]?.id,
      hasMore: state.demoCursor < demoRows.length,
    });
    state.banner = `Loaded ${slice.length} more attempts from the trail.`;
    scheduleRender();
    return;
  }
  if (state.pageCursor) {
    loadAuditPage(state.pageCursor, true);
  }
}

async function connectLive() {
  cleanupStream();
  if (!state.liveToken.trim()) {
    state.mode = 'error';
    state.resyncLink = '';
    state.banner = 'Paste a bearer token to connect live, or stay in demo mode.';
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
    if (!token) {
      state.loading = false;
      primeDemoFeed(true);
      return;
    }
    const resp = await fetch(url, { headers });
    if (!resp.ok) {
      if (resp.status === 410) {
        const problem = await resp.json();
        state.mode = 'resync';
        state.resyncLink = problem.resync || '';
        state.banner = 'Cursor gap detected; resync history before reconnecting.';
        state.loading = false;
        scheduleRender();
        return;
      }
      throw new Error(`${resp.status}`);
    }
    const page = await resp.json();
    const items = Array.isArray(page.items) ? page.items : [];
    setRows(items, {
      append,
      pageCursor: page.page?.nextCursor || items[items.length - 1]?.id || after || null,
      highWaterCursor: page.page?.highWaterCursor || items[items.length - 1]?.id || after || null,
      hasMore: Boolean(page.page?.hasMore),
    });
    state.mode = liveMode ? 'live' : 'demo';
    state.banner = liveMode ? 'Live feed connected.' : 'Historical attempts loaded.';
    state.loading = false;
    scheduleRender();
    if (liveMode) {
      startSse(page.page?.highWaterCursor || page.page?.nextCursor || items[items.length - 1]?.id || after || null);
    }
  } catch {
    state.loading = false;
    state.mode = 'error';
    state.resyncLink = '';
    state.banner = 'Live feed unavailable; demo trail remains ready.';
    scheduleRender();
    if (!token) primeDemoFeed(true);
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

  fetch(url, { headers, signal: controller.signal })
    .then(async (resp) => {
      if (resp.status === 410) {
        const problem = await resp.json();
        state.mode = 'resync';
        state.resyncLink = problem.resync || '';
        state.loading = false;
        state.banner = 'Cursor gap detected; resync history before reconnecting.';
        scheduleRender();
        return;
      }
      if (!resp.ok || !resp.body) {
        throw new Error(`stream ${resp.status}`);
      }
      state.mode = 'live';
      state.loading = false;
      state.banner = 'Live SSE connected.';
      scheduleRender();
      await consumeSse(resp.body, (item) => {
        const normalized = normalizeAttempt(item);
        state.rows = dedupeRows([...state.rows, normalized]);
        state.highWaterCursor = normalized.id;
        state.selectedId = normalized.id;
        refreshVisibleRows();
        state.banner = `Live update: ${normalized.technique} against ${normalized.target}`;
        scheduleRender();
      });
      if (!controller.signal.aborted) {
        scheduleReconnect(state.highWaterCursor);
      }
    })
    .catch(() => {
      if (controller.signal.aborted) return;
      state.mode = 'reconnecting';
      state.loading = false;
      state.banner = 'Connection hiccup; retrying with the latest cursor.';
      scheduleRender();
      scheduleReconnect(state.highWaterCursor);
    });
}

async function consumeSse(stream, onItem) {
  const reader = stream.getReader();
  const decoder = new TextDecoder();
  let buffer = '';
  while (true) {
    const { value, done } = await reader.read();
    if (done) break;
    buffer += decoder.decode(value, { stream: true });
    let boundary = buffer.indexOf('\n\n');
    while (boundary >= 0) {
      const frame = buffer.slice(0, boundary);
      buffer = buffer.slice(boundary + 2);
      const parsed = parseSseFrame(frame);
      if (parsed.data) {
        try {
          onItem(JSON.parse(parsed.data));
        } catch {
          // ignore malformed payloads
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
    state.banner = 'History refreshed; reconnecting live.';
    scheduleRender();
    startSse(page.page?.highWaterCursor || state.highWaterCursor || null);
  } catch {
    state.loading = false;
    state.mode = 'error';
    state.banner = 'Resync failed; keep using demo mode or retry the token.';
    scheduleRender();
  }
}

function buildOptions(current, values, labelBuilder = (value) => value) {
  return [`<option value="all"${current === 'all' ? ' selected' : ''}>All</option>`]
    .concat(values.map((value) => `<option value="${escapeHtml(value)}"${current === value ? ' selected' : ''}>${escapeHtml(labelBuilder(value))}</option>`))
    .join('');
}

function renderWaypoints(waypoints, activeId) {
  return waypoints.map((waypoint) => {
    if (waypoint.state === 'current') {
      return `
        <g class="waypoint campfire-node is-current" role="button" tabindex="0" data-action="phase" data-phase="${waypoint.id}" aria-current="step" aria-label="${escapeHtml(waypoint.name)}, current stage. You are here.">
          <circle cx="${waypoint.x}" cy="${waypoint.y}" r="17" fill="#EF9F27" stroke="#FAC775" stroke-width="3" />
          <path d="M${waypoint.x} ${waypoint.y - 8} c -4 5 -5 8 -2 11 c -3 0 -5 3 -2 5 c 2 2 7 2 9 0 c 3 -2 1 -5 -2 -5 c 3 -3 1 -7 -3 -11" class="campfire" />
          <text x="${waypoint.x}" y="${waypoint.y + 34}" text-anchor="middle" font-size="11" font-weight="600" fill="#633806">${escapeHtml(waypoint.name)} — you are here</text>
        </g>`;
    }
    if (waypoint.state === 'completed') {
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

function attackSummaryCards(rows) {
  const techniques = new Set(rows.map((row) => row.technique));
  const targets = new Set(rows.map((row) => row.target));
  const hosts = new Set(rows.map((row) => row.host));
  const blocked = rows.filter((row) => row.status === 'blocked').length;
  return `
    <div class="attack-summary-grid" aria-label="Attack summary">
      <div class="metric"><span class="metric-label">Attempts</span><strong>${rows.length}</strong></div>
      <div class="metric"><span class="metric-label">Techniques</span><strong>${techniques.size}</strong></div>
      <div class="metric"><span class="metric-label">Targets</span><strong>${targets.size}</strong></div>
      <div class="metric"><span class="metric-label">Blocked</span><strong>${blocked}</strong></div>
      <div class="metric"><span class="metric-label">Hosts</span><strong>${hosts.size}</strong></div>
    </div>
  `;
}

function attackRowMarkup(row, selected) {
  return `
    <li class="attack-row ${selected ? 'is-selected' : ''}">
      <button type="button" class="attack-row-button" data-action="select-attempt" data-row-id="${escapeHtml(row.id)}"${selected ? ' aria-current="true"' : ''}>
        <div class="attack-row-top">
          <span class="attack-row-time">${escapeHtml(formatTime(row.occurredAt))}</span>
          <span class="attack-row-actor">${escapeHtml(row.actor.handle)}</span>
          <span class="actor-chip ${escapeHtml(row.actor.kind)}">${escapeHtml(row.actor.kind)}</span>
        </div>
        <div class="attack-row-main">
          <span class="attack-field"><strong>Technique</strong><span>${escapeHtml(row.technique)}</span></span>
          <span class="attack-field"><strong>Target</strong><span>${escapeHtml(row.target)}</span></span>
          <span class="attack-field"><strong>Host</strong><span>${escapeHtml(row.host)}</span></span>
          <span class="attack-field"><strong>Status</strong><span class="status-pill ${statusClass(row.status)}">${escapeHtml(row.statusLabel)}</span></span>
        </div>
        <div class="attack-row-note">${escapeHtml(row.summary)}</div>
        <div class="attack-row-foot">
          <span>${escapeHtml(row.command || row.origin.service || row.origin.kind)}</span>
          <span>Cursor ${escapeHtml(row.id)} · ${escapeHtml(row.requestId)}</span>
        </div>
      </button>
    </li>
  `;
}

function attackGroupsMarkup(rows, selectedId) {
  const groups = groupAttempts(rows);
  if (!groups.length) {
    return '<p class="empty-state">Fog on the trail — relax a filter or reconnect the feed to reveal attempts.</p>';
  }
  return groups.map((group) => {
    const statusSummary = [...group.counts.entries()]
      .sort((a, b) => a[0].localeCompare(b[0]))
      .map(([status, count]) => `<span class="status-pill ${statusClass(status)}">${escapeHtml(statusToLabel(status))} ${count}</span>`)
      .join('');
    return `
      <section class="attack-group">
        <header class="attack-group-header">
          <div>
            <p class="workspace-kicker">Technique</p>
            <h3>${escapeHtml(group.key)}</h3>
            <p>${group.rows.length} attempts · ${group.targets.size} targets · ${group.hosts.size} hosts</p>
          </div>
          <div class="attack-group-meta">${statusSummary}</div>
        </header>
        <ol class="attack-group-list">
          ${group.rows.map((row) => attackRowMarkup(row, row.id === selectedId)).join('')}
        </ol>
      </section>
    `;
  }).join('');
}

function attemptDetailMarkup(row) {
  if (!row) {
    return '<p class="empty-state">No attempt selected yet. Pick a row to open its raw and structured evidence.</p>';
  }
  const structuredJson = JSON.stringify(row.structured || {}, null, 2);
  return `
    <div class="detail-card">
      <div class="detail-head">
        <div>
          <p class="workspace-kicker">Attempt drill-in</p>
          <h3>${escapeHtml(row.technique)}</h3>
          <p>${escapeHtml(row.summary)}</p>
        </div>
        <span class="status-pill ${statusClass(row.status)}">${escapeHtml(row.statusLabel)}</span>
      </div>
      <dl class="detail-grid">
        <div><dt>Actor</dt><dd>${escapeHtml(row.actor.kind)} · ${escapeHtml(row.actor.handle)}${row.actor.agentName ? ` · ${escapeHtml(row.actor.agentName)}` : ''}</dd></div>
        <div><dt>Target</dt><dd>${escapeHtml(row.target)}</dd></div>
        <div><dt>Host</dt><dd>${escapeHtml(row.host)}</dd></div>
        <div><dt>Source</dt><dd>${escapeHtml(row.origin.kind)}${row.origin.service ? ` · ${escapeHtml(row.origin.service)}` : ''}</dd></div>
        <div><dt>Command</dt><dd>${escapeHtml(row.command || 'Not captured')}</dd></div>
        <div><dt>Parse</dt><dd>${escapeHtml(row.parseStatus)}${row.pluginId ? ` · ${escapeHtml(row.pluginId)}` : ''}</dd></div>
        <div><dt>Timing</dt><dd>${escapeHtml(formatTime(row.occurredAt))}</dd></div>
        <div><dt>Request</dt><dd>${escapeHtml(row.requestId)}</dd></div>
      </dl>
      <div class="evidence-split">
        <section class="evidence-box">
          <div class="evidence-head">
            <h4>Raw output</h4>
            <span class="evidence-kind">${escapeHtml(row.evidence.kind)}</span>
          </div>
          <p>Rendered as escaped text only. No raw HTML from the target can execute here.</p>
          <pre>${escapeHtml(row.rawOutput || row.evidence.safePreview)}</pre>
        </section>
        <section class="evidence-box">
          <div class="evidence-head">
            <h4>Structured result</h4>
            <button type="button" class="secondary-link" data-action="copy-hash" data-hash="${escapeHtml(row.evidence.sha256)}">Copy hash</button>
          </div>
          <pre>${escapeHtml(structuredJson || '{}')}</pre>
          <div class="evidence-meta">
            <span>${escapeHtml(row.evidence.byteLength)} bytes</span>
            <span>${escapeHtml(row.evidence.mediaType)}</span>
            <span>${escapeHtml(row.result.detail || row.result.label)}</span>
          </div>
          <code>${escapeHtml(row.evidence.sha256)}</code>
        </section>
      </div>
      <div class="detail-foot">
        <span>Subject ${escapeHtml(row.subject.type)} · ${escapeHtml(row.subject.id)} · v${escapeHtml(row.subject.revision)}</span>
        <span>Cursor ${escapeHtml(row.id)}</span>
      </div>
    </div>
  `;
}

function attackWorkspaceMarkup(active, currentIndex, rows, selected) {
  const filters = state.filters;
  const techniqueValues = uniqueValues((row) => row.technique.toLowerCase());
  const targetValues = uniqueValues((row) => row.target.toLowerCase());
  const hostValues = uniqueValues((row) => row.host.toLowerCase());
  const statusValues = uniqueValues((row) => row.status.toLowerCase());

  return `
    <section class="workspace-panel attacks-workspace" aria-label="Attacks workspace">
      <div class="workspace-header">
        <div>
          <p class="workspace-kicker">Stage ${currentIndex + 1} of ${phaseOrder.length}</p>
          <h2>${escapeHtml(active.workspaceTitle)}</h2>
        </div>
        <div class="workspace-status-stack">
          <p class="workspace-status">${escapeHtml(feedStatusLabel())}</p>
          <span class="status-pill ${feedStatusTone()}">${escapeHtml(feedStatusTone() === 'review' ? 'Needs review' : feedStatusTone() === 'blocked' ? 'Blocked' : feedStatusTone() === 'queued' ? 'Queued' : feedStatusTone() === 'success' ? 'Live' : 'Idle')}</span>
        </div>
      </div>

      <p class="workspace-lede">${escapeHtml(active.workspaceLede)}</p>

      ${state.banner ? `<div class="live-banner ${statusClass(state.mode === 'reconnecting' ? 'queued' : state.mode === 'error' ? 'blocked' : state.mode === 'resync' ? 'needs-review' : 'success')}" role="status">${escapeHtml(state.banner)}</div>` : ''}

      <div class="attack-toolbar">
        <label class="field-group"><span>Technique</span><select data-action="filter" data-filter="technique">${buildOptions(filters.technique, techniqueValues, (value) => value)}</select></label>
        <label class="field-group"><span>Target</span><select data-action="filter" data-filter="target">${buildOptions(filters.target, targetValues, (value) => value)}</select></label>
        <label class="field-group"><span>Host</span><select data-action="filter" data-filter="host">${buildOptions(filters.host, hostValues, (value) => value)}</select></label>
        <label class="field-group"><span>Status</span><select data-action="filter" data-filter="status">${buildOptions(filters.status, statusValues, (value) => statusToLabel(value))}</select></label>
        <label class="field-group field-search"><span>Search</span><input type="search" data-action="filter" data-filter="q" value="${escapeHtml(filters.q)}" placeholder="Search technique, target, host, status, output" /></label>
        <label class="field-group field-token"><span>Live token</span><input type="password" data-action="token" value="${escapeHtml(state.liveToken)}" placeholder="Paste bearer token to connect live" autocomplete="off" /></label>
      </div>

      ${attackSummaryCards(rows)}

      ${state.resyncLink ? '<div class="live-banner review">Cursor gap detected. Resync the trail from persisted history, then reconnect live.</div>' : ''}

      <div class="attack-shell">
        <div class="attack-list-column">
          ${attackGroupsMarkup(rows, selected?.id || null)}
        </div>
        <aside class="attack-detail" aria-live="polite">
          ${attemptDetailMarkup(selected)}
        </aside>
      </div>

      <div class="workspace-footer">
        <button type="button" class="secondary-link" data-action="demo">Use demo</button>
        <button type="button" class="primary-button" data-action="connect-live">Connect live</button>
        <button type="button" class="secondary-link" data-action="load-more" ${state.loading || !state.hasMore ? 'disabled' : ''}>${state.loading ? 'Loading…' : state.hasMore ? 'Load next batch' : 'No more attempts'}</button>
        <button type="button" class="secondary-link" data-action="resync" ${state.resyncLink ? '' : 'disabled'}>Resync</button>
      </div>
    </section>
  `;
}

function renderSidebarLog(active) {
  const recent = [...state.visibleRows].sort((a, b) => Number(b.id) - Number(a.id)).slice(0, 3);
  if (active.id === 'attacks') {
    return `
      <section class="log-panel" aria-label="Trail updates">
        <div class="panel-heading compact">
          <div>
            <h2>📖 Trail updates</h2>
            <p>The audit trail is the journey log — exact technique, target, host, and result stay in view.</p>
          </div>
          <div class="status-pill ${feedStatusTone()}">${escapeHtml(feedStatusLabel())}</div>
        </div>
        <ul class="recent-attempts">
          ${recent.map((row) => `<li><strong>${escapeHtml(row.technique)}</strong><span>${escapeHtml(row.target)} · ${escapeHtml(row.host)} · ${escapeHtml(row.statusLabel)}</span></li>`).join('')}
        </ul>
      </section>
    `;
  }

  const journeyLog = [
    'Day 1 — Basecamp set. Project named, team invited, scope loaded.',
    'Day 2 — Creek crossed. 240 records packed into the trail log.',
    `Now — Made camp in ${active.name}. The audit trail is live and attributed.`,
  ];

  return `
    <section class="log-panel" aria-label="Journey log">
      <div class="panel-heading compact">
        <div>
          <h2>📖 Journey log</h2>
          <p>The audit trail is the journey log — one entry per meaningful action.</p>
        </div>
      </div>
      <ul class="journey-list">
        ${journeyLog.map((entry, index) => `<li class="${index === 2 ? 'is-current' : ''}">${escapeHtml(entry)}</li>`).join('')}
      </ul>
    </section>
  `;
}

function render() {
  const active = phaseData[state.activePhase];
  const currentIndex = phaseOrder.indexOf(state.activePhase);
  const waypoints = phaseOrder.map((phase, index) => ({ ...phaseData[phase], id: phase, state: index < currentIndex ? 'completed' : index === currentIndex ? 'current' : 'fog' }));
  const selected = selectedAttempt();
  const guideQuery = state.guideQuery.trim().toLowerCase();
  const visibleGuideExplainers = guideExplainers.filter((explanation) => {
    const haystack = [
      explanation.phase,
      explanation.title,
      explanation.summary,
      explanation.whenToUse,
      explanation.risks,
      explanation.keywords.join(' '),
    ].join(' ').toLowerCase();

    if (!guideQuery) {
      return explanation.phase === state.activePhase;
    }

    return haystack.includes(guideQuery);
  });

  document.documentElement.dataset.theme = state.theme;
  document.documentElement.dataset.view = state.view;

  if (state.view === 'report') {
    document.title = 'Waypoint — report snapshot';
    root.innerHTML = `
      <main class="app-shell report-shell" aria-label="Frozen report snapshot">
        <section class="report-hero artifact">
          <div>
            <p class="eyebrow">Waypoint · frozen report snapshot</p>
            <h1>${escapeHtml(reportSnapshot.title)}</h1>
            <p class="subtitle">Version ${escapeHtml(reportSnapshot.version)} · ${escapeHtml(reportSnapshot.engagement)} · Cutoff ${escapeHtml(reportSnapshot.cutoff)}</p>
          </div>
          <div class="report-toolbar">
            <button type="button" class="secondary-link" data-action="report-back">Back to Summit</button>
            <button type="button" class="primary-button" data-action="report-print">Print PDF</button>
          </div>
        </section>

        <article class="report-page" aria-label="Printable engagement report">
          <section class="report-section">
            <h2>Scope</h2>
            <ul>${reportSnapshot.scope.map((item) => `<li>${escapeHtml(item)}</li>`).join('')}</ul>
          </section>

          <section class="report-section">
            <h2>Methodology</h2>
            <ul>${reportSnapshot.methodology.map((item) => `<li>${escapeHtml(item)}</li>`).join('')}</ul>
          </section>

          <section class="report-section">
            <h2>Findings</h2>
            <div class="report-grid">
              ${reportSnapshot.findings.map((finding) => `
                <article class="report-card">
                  <p class="report-badge">${escapeHtml(finding.severity)}</p>
                  <h3>${escapeHtml(finding.title)}</h3>
                  <p>${escapeHtml(finding.summary)}</p>
                  <p><strong>Evidence:</strong> ${finding.evidence.map((item) => escapeHtml(item)).join(', ')}</p>
                  <p><strong>Remediation:</strong> ${escapeHtml(finding.remediation)}</p>
                </article>
              `).join('')}
            </div>
          </section>

          <section class="report-section">
            <h2>Evidence</h2>
            <div class="report-grid">
              ${reportSnapshot.evidence.map((item) => `
                <article class="report-card">
                  <p class="report-badge">${escapeHtml(item.label)}</p>
                  <p><strong>Source:</strong> ${escapeHtml(item.source)}</p>
                  <p><strong>Actor:</strong> ${escapeHtml(item.actor)}</p>
                  <p><strong>Host:</strong> ${escapeHtml(item.host)}</p>
                  <p><strong>Attribution:</strong> ${escapeHtml(item.attribution)}</p>
                  <p class="report-snippet">${escapeHtml(item.rawSnippet)}</p>
                  <p>${escapeHtml(item.note)}</p>
                </article>
              `).join('')}
            </div>
          </section>

          <section class="report-section">
            <h2>Bundle manifest</h2>
            <div class="report-grid">
              ${reportSnapshot.bundle.payloads.map((payload) => `
                <article class="report-card">
                  <h3>${escapeHtml(payload.path)}</h3>
                  <p><strong>Size:</strong> ${payload.size} bytes</p>
                  <p class="report-snippet">${escapeHtml(payload.sha256)}</p>
                </article>
              `).join('')}
            </div>
            <div class="report-grid" style="margin-top: 12px;">
              <article class="report-card">
                <h3>Archive hash</h3>
                <p class="report-snippet">${escapeHtml(reportSnapshot.bundle.outerArchiveSha256)}</p>
              </article>
              <article class="report-card">
                <h3>Signature hook</h3>
                <p>${escapeHtml(reportSnapshot.bundle.signatures.version)}</p>
                <p>${reportSnapshot.bundle.signatures.items.length ? escapeHtml(reportSnapshot.bundle.signatures.items.join(', ')) : 'empty'}</p>
              </article>
            </div>
          </section>

          <section class="report-section">
            <h2>Verified export receipt</h2>
            <div class="report-grid">
              <article class="report-card">
                <h3>Receipt ID</h3>
                <p class="report-snippet">${escapeHtml(reportSnapshot.receipt.id)}</p>
                <p>${escapeHtml(reportSnapshot.receipt.note)}</p>
              </article>
              <article class="report-card">
                <h3>Receipt state</h3>
                <p>${escapeHtml(reportSnapshot.receipt.captureState)}</p>
                <p><strong>Verified at:</strong> ${escapeHtml(reportSnapshot.receipt.verifiedAt)}</p>
              </article>
              <article class="report-card">
                <h3>Receipt manifest hash</h3>
                <p class="report-snippet">${escapeHtml(reportSnapshot.receipt.manifestHash)}</p>
              </article>
            </div>
          </section>

          <section class="report-section">
            <h2>Restore and regenerate</h2>
            <div class="report-grid">
              <article class="report-card">
                <h3>Offline tools</h3>
                <ul>${reportSnapshot.bundle.restore.tools.map((tool) => `<li>${escapeHtml(tool)}</li>`).join('')}</ul>
              </article>
              <article class="report-card">
                <h3>Clean-room checks</h3>
                <ul>${reportSnapshot.bundle.restore.cleanRoom.map((item) => `<li>${escapeHtml(item)}</li>`).join('')}</ul>
              </article>
              <article class="report-card">
                <h3>Malicious paths</h3>
                <ul>${reportSnapshot.bundle.restore.maliciousPaths.map((item) => `<li>${escapeHtml(item)}</li>`).join('')}</ul>
              </article>
            </div>
          </section>

          <section class="report-section">
            <h2>Attribution</h2>
            <div class="report-grid">
              ${reportSnapshot.attribution.map((section) => `
                <article class="report-card">
                  <h3>${escapeHtml(section.title)}</h3>
                  <ul>${section.items.map((item) => `<li>${escapeHtml(item)}</li>`).join('')}</ul>
                </article>
              `).join('')}
            </div>
          </section>

          <section class="report-section">
            <h2>Known capture gaps</h2>
            <ul>${reportSnapshot.knownCaptureGaps.map((item) => `<li>${escapeHtml(item)}</li>`).join('')}</ul>
          </section>
        </article>
      </main>
    `;
    bindUI();
    return;
  }

  document.title = `Waypoint — ${active.name}`;

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
          <div class="progress-pill" aria-label="Trail progress">Trail ${Math.min(currentIndex + 1, phaseOrder.length)} / ${phaseOrder.length} · ${escapeHtml(active.name)}</div>
          <div class="metrics" aria-label="Engagement progress">
            <div class="metric"><span class="metric-label">Traveled</span><strong>${Math.min(currentIndex + 1, phaseOrder.length)} waypoints</strong></div>
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
                ${renderWaypoints(waypoints, state.activePhase)}
              </svg>
              <div class="waypoint-overlay" aria-label="Trail waypoint shortcuts">
                ${waypoints.map((waypoint) => `
                  <button type="button" class="waypoint-hitbox ${waypoint.id === state.activePhase ? 'is-active' : ''}" data-action="phase" data-phase="${waypoint.id}"${waypoint.id === state.activePhase ? ' aria-current="step"' : ''} aria-label="${escapeHtml(waypoint.name)}, ${escapeHtml(waypoint.state)}${waypoint.id === state.activePhase ? ', you are here' : ''}" style="left:${(waypoint.x / 640) * 100}%;top:${(waypoint.y / 300) * 100}%;"></button>
                `).join('')}
              </div>
            </div>
          </section>

          ${state.activePhase === 'attacks'
            ? attackWorkspaceMarkup(active, currentIndex, state.visibleRows, selected)
            : `
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
                  ${active.cards.map((card) => `
                    <section class="skeleton-card">
                      <h3>${escapeHtml(card.title)}</h3>
                      <ul>${card.items.map((item) => `<li>${escapeHtml(item)}</li>`).join('')}</ul>
                    </section>
                  `).join('')}
                </div>
                ${state.activePhase === 'summit' ? `
                  <section class="summit-flow" aria-label="Summit export and teardown flow">
                    <div class="summit-status">
                      <span class="status-chip ${state.summitExportStatus}">${escapeHtml(state.summitExportStatus)}</span>
                      <p>${escapeHtml({
                        idle: 'Preflight the bundle before you freeze the snapshot.',
                        preflight: 'Preflight running. Capture stays live while the bundle is checked.',
                        exporting: 'Exporting now. The live audit trail keeps running while the snapshot is frozen.',
                        verified: 'Verified export receipt recorded. Teardown is now guarded by the receipt.',
                        failed: 'Checksum drift detected. Re-run the preflight after fixing the bundle.',
                        canceled: 'Export canceled. Nothing was torn down, and capture remained intact.',
                      }[state.summitExportStatus])}</p>
                    </div>
                    <div class="summit-controls">
                      <button type="button" class="primary-button" data-action="summit-preflight"${state.summitExportStatus === 'preflight' || state.summitExportStatus === 'exporting' ? ' disabled' : ''}>Run export preflight</button>
                      <button type="button" class="secondary-link" data-action="summit-cancel"${state.summitExportStatus !== 'preflight' && state.summitExportStatus !== 'exporting' ? ' disabled' : ''}>Cancel export</button>
                      <button type="button" class="secondary-link" data-action="summit-fail">Simulate checksum failure</button>
                    </div>
                    ${state.summitExportStatus === 'verified' ? `
                      <article class="receipt-card" aria-label="Verified export receipt">
                        <div class="panel-heading compact">
                          <h3>Receipt verified</h3>
                          <p>Capture stayed live while the bundle froze.</p>
                        </div>
                        <dl class="receipt-grid">
                          <div><dt>Receipt</dt><dd>${escapeHtml(reportSnapshot.receipt.id)}</dd></div>
                          <div><dt>Verified</dt><dd>${escapeHtml(reportSnapshot.receipt.verifiedAt)}</dd></div>
                          <div><dt>Manifest</dt><dd class="report-snippet">${escapeHtml(reportSnapshot.receipt.manifestHash)}</dd></div>
                        </dl>
                      </article>
                    ` : ''}
                    ${state.summitExportStatus === 'failed' ? `
                      <article class="receipt-card is-failed" aria-label="Export failure recovery">
                        <div class="panel-heading compact">
                          <h3>Export needs recovery</h3>
                          <p>Rerun the preflight after fixing the bundle or evidence mismatch.</p>
                        </div>
                        <button type="button" class="primary-button" data-action="summit-retry">Retry export preflight</button>
                      </article>
                    ` : ''}
                    <article class="break-glass-panel" aria-label="Break-glass teardown guard">
                      <div class="panel-heading compact">
                        <h3>Break-glass teardown</h3>
                        <p>Export receipt required before the live box can be destroyed.</p>
                      </div>
                      <label class="break-glass-toggle">
                        <input type="checkbox" data-action="summit-arm"${state.breakGlassArmed ? ' checked' : ''} />
                        Arm the teardown guard
                      </label>
                      <label class="break-glass-input">
                        <span>Type WIPE NOW to confirm</span>
                        <input type="text" data-action="summit-phrase" value="${escapeHtml(state.destroyPhrase)}" placeholder="WIPE NOW" />
                      </label>
                      <button type="button" class="danger-button" data-action="summit-destroy"${state.summitExportStatus !== 'verified' || !state.breakGlassArmed || state.destroyPhrase.trim().toUpperCase() !== 'WIPE NOW' || state.teardownState === 'destroyed' ? ' disabled' : ''}>${state.teardownState === 'destroyed' ? 'Teardown queued' : 'Destroy disposable instance'}</button>
                      <p class="summit-warning">${escapeHtml(state.teardownState === 'destroyed' ? 'Break-glass was used after receipt verification. Nothing else should run here.' : state.summitExportStatus === 'verified' && state.breakGlassArmed && state.destroyPhrase.trim().toUpperCase() === 'WIPE NOW' ? 'Guard armed. The instance can be destroyed deliberately.' : 'Guard remains locked until the verified receipt and break-glass phrase are in place.')}</p>
                    </article>
                  </section>
                ` : ''}
                <div class="workspace-footer">
                  <a class="secondary-link" href="${phasePath(phaseOrder[Math.max(0, currentIndex - 1)])}" data-action="phase" data-phase="${phaseOrder[Math.max(0, currentIndex - 1)]}">Back to ${escapeHtml(phaseData[phaseOrder[Math.max(0, currentIndex - 1)]].name)}</a>
                  <a class="primary-button" href="${state.activePhase === 'summit' ? reportPathFor() : phasePath(phaseOrder[Math.min(phaseOrder.length - 1, currentIndex + 1)])}" data-action="${state.activePhase === 'summit' ? 'report' : 'phase'}"${state.activePhase === 'summit' ? '' : ` data-phase="${phaseOrder[Math.min(phaseOrder.length - 1, currentIndex + 1)]}"`}>${state.activePhase === 'summit' ? 'Open report preview →' : `Continue to ${escapeHtml(phaseData[phaseOrder[Math.min(phaseOrder.length - 1, currentIndex + 1)]].name)} →`}</a>
                </div>
              </section>
            `}
        </section>

        <aside class="sidebar" aria-label="Guide and trail details">
          <nav class="route-nav" aria-label="Engagement waypoints">
            <div class="panel-heading">
              <h2>Waypoints</h2>
              <p>All phases stay accessible; fog means no data discovered yet.</p>
            </div>
            <ol>
              ${waypoints.map((waypoint, index) => `
                <li>
                  <button type="button" class="route-link ${waypoint.id === state.activePhase ? 'is-active' : ''}" data-action="phase" data-phase="${waypoint.id}"${waypoint.id === state.activePhase ? ' aria-current="step"' : ''}>
                    <span class="route-link-copy"><strong>${escapeHtml(waypoint.name)}</strong><span>Stage ${index + 1} of ${phaseOrder.length}</span></span>
                    <span class="route-status ${waypoint.state}">${escapeHtml(waypoint.state === 'fog' ? 'Fog' : waypoint.state === 'current' ? 'Here' : 'Done')}</span>
                  </button>
                </li>
              `).join('')}
            </ol>
          </nav>

          <section class="guide-panel artifact" aria-label="Guide's note">
            <div class="panel-icon" aria-hidden="true">🧭</div>
            <div class="guide-copy">
              <h2>Guide's note</h2>
              <p>${escapeHtml(active.note)}</p>
              <div class="guide-tools">
                <label class="guide-search"><span class="sr-only">Search reviewed guide notes</span><input type="search" data-action="guide-search" value="${escapeHtml(state.guideQuery)}" placeholder="Search reviewed notes and context" aria-label="Search reviewed guide notes" /></label>
                <button type="button" class="primary-button" data-action="${state.activePhase === 'summit' ? 'report' : 'phase'}"${state.activePhase === 'summit' ? '' : ` data-phase="${phaseOrder[Math.min(phaseOrder.length - 1, currentIndex + 1)]}"`}>${state.activePhase === 'summit' ? 'Open report preview →' : currentIndex < phaseOrder.length - 1 ? `Continue into ${escapeHtml(phaseData[phaseOrder[currentIndex + 1]].name)} →` : `Return to ${escapeHtml(phaseData[phaseOrder[currentIndex - 1]].name)} →`}</button>
              </div>
              <div class="guide-note-list" aria-label="Reviewed guide notes">
                ${visibleGuideExplainers.length ? visibleGuideExplainers.map((explanation) => `
                  <article class="guide-note-card" id="${escapeHtml(explanation.id)}">
                    <p class="guide-note-kicker">${escapeHtml(phaseData[explanation.phase].name)} · reviewed note</p>
                    <h3>${escapeHtml(explanation.title)}</h3>
                    <p>${escapeHtml(explanation.summary)}</p>
                    <dl>
                      <div><dt>When</dt><dd>${escapeHtml(explanation.whenToUse)}</dd></div>
                      <div><dt>Risks</dt><dd>${escapeHtml(explanation.risks)}</dd></div>
                    </dl>
                    <a href="${escapeHtml(explanation.contextHref)}">${escapeHtml(explanation.contextLabel)}</a>
                  </article>
                `).join('') : '<p class="guide-note-empty">No reviewed notes match this search.</p>'}
              </div>
            </div>
          </section>

          ${renderSidebarLog(active)}

          <section class="route-summary" aria-label="Route summary">
            <div>
              <p class="metric-label">Current waypoint</p>
              <strong>${escapeHtml(active.name)}</strong>
            </div>
            <p>${escapeHtml(active.id === 'attacks' ? 'What have we tried, what worked, and what is still fogged? This view keeps the answer obvious.' : active.id === 'findings' ? 'Promotions stay defensible and linked to evidence.' : active.id === 'summit' ? 'Export, verify the manifest, then wipe the disposable box.' : 'Collect signals and keep the pack tidy.' )}</p>
          </section>
        </aside>
      </div>
    </main>
  `;

  bindUI();
}

function bindUI() {
  root.querySelectorAll('[data-action="theme"]').forEach((button) => {
    button.addEventListener('click', () => {
      state.theme = button.dataset.theme;
      safeStorageSet('waypoint-theme', state.theme);
      scheduleRender();
    });
  });

  root.querySelectorAll('[data-action="report"]').forEach((button) => {
    button.addEventListener('click', () => {
      state.view = 'report';
      state.activePhase = 'summit';
      navigateToReport();
      scheduleRender();
    });
  });

  root.querySelectorAll('[data-action="report-back"]').forEach((button) => {
    button.addEventListener('click', () => {
      state.view = 'trail';
      state.activePhase = 'summit';
      navigateToPhase('summit');
      scheduleRender();
    });
  });

  root.querySelectorAll('[data-action="report-print"]').forEach((button) => {
    button.addEventListener('click', () => window.print());
  });

  root.querySelectorAll('[data-action="phase"]').forEach((button) => {
    button.addEventListener('click', () => {
      const phase = button.dataset.phase;
      if (!phase || phase === state.activePhase) return;
      if (state.activePhase === 'summit' && phase !== 'summit') {
        clearSummitTimers();
      }
      state.view = 'trail';
      state.activePhase = phase;
      navigateToPhase(phase);
      scheduleRender();
    });
  });

  root.querySelectorAll('[data-action="filter"]').forEach((control) => {
    control.addEventListener('input', onFilterChange);
    control.addEventListener('change', onFilterChange);
  });

  root.querySelector('[data-action="token"]')?.addEventListener('input', (event) => {
    state.liveToken = event.target.value;
  });

  root.querySelector('[data-action="guide-search"]')?.addEventListener('input', (event) => {
    state.guideQuery = event.target.value;
    scheduleRender();
  });

  root.querySelectorAll('[data-action="summit-preflight"]').forEach((button) => {
    button.addEventListener('click', () => startSummitExport());
  });

  root.querySelectorAll('[data-action="summit-cancel"]').forEach((button) => {
    button.addEventListener('click', () => {
      clearSummitTimers();
      state.summitExportStatus = 'canceled';
      scheduleRender();
    });
  });

  root.querySelectorAll('[data-action="summit-fail"]').forEach((button) => {
    button.addEventListener('click', () => {
      clearSummitTimers();
      state.summitExportStatus = 'failed';
      scheduleRender();
    });
  });

  root.querySelectorAll('[data-action="summit-retry"]').forEach((button) => {
    button.addEventListener('click', () => startSummitExport());
  });

  root.querySelectorAll('[data-action="summit-arm"]').forEach((control) => {
    control.addEventListener('change', (event) => {
      state.breakGlassArmed = event.target.checked;
      scheduleRender();
    });
  });

  root.querySelectorAll('[data-action="summit-phrase"]').forEach((control) => {
    control.addEventListener('input', (event) => {
      state.destroyPhrase = event.target.value;
      scheduleRender();
    });
  });

  root.querySelectorAll('[data-action="summit-destroy"]').forEach((button) => {
    button.addEventListener('click', () => {
      if (state.summitExportStatus !== 'verified' || !state.breakGlassArmed || state.destroyPhrase.trim().toUpperCase() !== 'WIPE NOW') return;
      state.teardownState = 'destroyed';
      scheduleRender();
    });
  });

  root.querySelector('[data-action="connect-live"]')?.addEventListener('click', () => connectLive());
  root.querySelector('[data-action="demo"]')?.addEventListener('click', () => primeDemoFeed(true));
  root.querySelector('[data-action="load-more"]')?.addEventListener('click', () => loadNextBatch());
  root.querySelector('[data-action="resync"]')?.addEventListener('click', () => resyncFromGap());

  root.querySelectorAll('[data-action="select-attempt"]').forEach((button) => {
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

window.addEventListener('popstate', () => {
  const route = getInitialRoute();
  state.view = route.view;
  state.activePhase = route.phase;
  scheduleRender();
});

window.addEventListener('beforeunload', cleanupStream);

function boot() {
  document.documentElement.dataset.theme = state.theme;
  document.documentElement.dataset.view = state.view;
  document.title = state.view === 'report' ? 'Waypoint — report snapshot' : `Waypoint — ${phaseData[state.activePhase].name}`;
  primeDemoFeed(false);
  render();
  if (state.liveToken.trim()) {
    connectLive();
  }
}

boot();
