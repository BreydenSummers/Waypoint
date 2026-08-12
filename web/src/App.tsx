import { useEffect, useMemo, useState } from 'react';

type ThemeMode = 'light' | 'dark';
type WaypointState = 'completed' | 'current' | 'fog';
type PhaseId = 'recon' | 'attacks' | 'findings' | 'summit';
type RouteView = 'trail' | 'report';
type SummitExportStatus = 'idle' | 'preflight' | 'exporting' | 'verified' | 'failed' | 'canceled';
type TeardownState = 'idle' | 'armed' | 'destroyed';

type ReportSection = {
  title: string;
  items: string[];
};

type BundlePayload = {
  path: string;
  size: number;
  sha256: string;
};

type BundleManifest = {
  payloads: BundlePayload[];
  outerArchiveSha256: string;
  signatures: {
    version: string;
    items: string[];
  };
  restore: {
    tools: string[];
    cleanRoom: string[];
    maliciousPaths: string[];
  };
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
    summary: string;
  }>;
  evidence: Array<{
    label: string;
    source: string;
    actor: string;
    host: string;
    attribution: string;
    rawSnippet: string;
    note: string;
  }>;
  bundle: BundleManifest;
  receipt: {
    id: string;
    verifiedAt: string;
    captureState: string;
    manifestHash: string;
    note: string;
  };
  attribution: ReportSection[];
  knownCaptureGaps: string[];
};

type Waypoint = {
  id: PhaseId;
  name: string;
  label: string;
  path: string;
  state: WaypointState;
  x: number;
  y: number;
  note: string;
  workspaceTitle: string;
  workspaceLede: string;
  cards: Array<{ title: string; items: string[] }>;
};

type GuideExplainer = {
  id: string;
  phase: PhaseId;
  title: string;
  summary: string;
  whenToUse: string;
  risks: string;
  contextLabel: string;
  contextHref: string;
  keywords: string[];
};

const engagementPath = '/engagements/demo';
const reportPath = `${engagementPath}/summit/report`;
const reportPdfPath = `${reportPath}.pdf`;
const waypointOrder: PhaseId[] = ['recon', 'attacks', 'findings', 'summit'];
const reportSnapshot: ReportSnapshot = {
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
  receipt: {
    id: 'receipt-q3-2025-01-10',
    verifiedAt: '2025-01-10T09:02:14Z',
    captureState: 'Capture remained live while export froze a clean snapshot.',
    manifestHash: '8e0f1d2c3b4a59687766554433221100ffeeddccbbaa99887766554433221100',
    note: 'Verified export receipt kept alongside the bundle so teardown stays defensible.',
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
};

const waypointDetails: Record<PhaseId, Omit<Waypoint, 'state'>> = {
  recon: {
    id: 'recon',
    name: 'Recon',
    label: 'Recon',
    path: `${engagementPath}/recon`,
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
    id: 'attacks',
    name: 'Attacks',
    label: 'Attacks',
    path: `${engagementPath}/attacks`,
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
    id: 'findings',
    name: 'Findings',
    label: 'Findings',
    path: `${engagementPath}/findings`,
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
    id: 'summit',
    name: 'Summit',
    label: 'Summit',
    path: `${engagementPath}/summit`,
    x: 586,
    y: 64,
    note: 'Reach the summit to export the engagement bundle and prepare teardown.',
    workspaceTitle: 'Summit workspace',
    workspaceLede: 'Final review, export, and bundle integrity checks live here before the box is wiped cleanly.',
    cards: [
      { title: 'Export preflight', items: ['Capture keeps flowing during export', 'Hash manifest and receipt are checked', 'Failure can be retried from the last clean step'] },
      { title: 'Verified receipt', items: ['Receipt ID is archived with the report', 'Manifest hash is pinned to the snapshot', 'Evidence and PDF stay attributable'] },
      { title: 'Break glass teardown', items: ['Destroy only after receipt verification', 'Interactive confirmation is required', 'Guarded destroy keeps the audit trail honest'] },
    ],
  },
};

const journeyLog = [
  'Day 1 — Basecamp set. Project named, team invited, scope loaded.',
  'Day 2 — Creek crossed. 240 records packed into the trail log.',
  'Now — Made camp in Attacks. The audit trail is live and attributed.',
];

const guideExplainers: GuideExplainer[] = [
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

function getInitialTheme(): ThemeMode {
  if (typeof window === 'undefined') {
    return 'light';
  }

  try {
    const stored = window.localStorage.getItem('waypoint-theme');
    if (stored === 'light' || stored === 'dark') {
      return stored;
    }
  } catch {
    // Ignore storage failures and fall back to the system theme.
  }

  return window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light';
}

function routeFromPath(pathname: string): { view: RouteView; phase: PhaseId } {
  if (/^\/engagements\/[^/]+\/summit\/report\/?$/.test(pathname)) {
    return { view: 'report', phase: 'summit' };
  }

  const match = pathname.match(/^\/engagements\/[^/]+\/(recon|attacks|findings|summit)\/?$/);
  if (match) {
    return { view: 'trail', phase: match[1] as PhaseId };
  }

  return { view: 'trail', phase: 'attacks' };
}

function getInitialRoute(): { view: RouteView; phase: PhaseId } {
  if (typeof window === 'undefined') {
    return { view: 'trail', phase: 'attacks' };
  }

  return routeFromPath(window.location.pathname);
}

function pathForPhase(phase: PhaseId): string {
  return waypointDetails[phase].path;
}

function pathForReport(): string {
  return reportPath;
}

function stateForIndex(index: number, activeIndex: number): WaypointState {
  if (index < activeIndex) return 'completed';
  if (index === activeIndex) return 'current';
  return 'fog';
}

function stateLabel(state: WaypointState): string {
  switch (state) {
    case 'completed':
      return 'completed';
    case 'current':
      return 'current';
    case 'fog':
      return 'fog';
  }
}

function shortStateLabel(state: WaypointState): string {
  switch (state) {
    case 'completed':
      return 'Done';
    case 'current':
      return 'Here';
    case 'fog':
      return 'Fog';
  }
}

function workspaceStepLabel(index: number): string {
  return `Stage ${index + 1} of ${waypointOrder.length}`;
}

function navigateToPhase(phase: PhaseId, replace = false) {
  const path = pathForPhase(phase);
  if (replace) {
    window.history.replaceState({}, '', path);
    return;
  }

  window.history.pushState({}, '', path);
}

function navigateToReport(replace = false) {
  if (replace) {
    window.history.replaceState({}, '', pathForReport());
    return;
  }

  window.history.pushState({}, '', pathForReport());
}

export function App() {
  const [theme, setTheme] = useState<ThemeMode>(getInitialTheme);
  const initialRoute = getInitialRoute();
  const [view, setView] = useState<RouteView>(initialRoute.view);
  const [activeId, setActiveId] = useState<PhaseId>(initialRoute.phase);
  const [guideQuery, setGuideQuery] = useState('');
  const [summitExportStatus, setSummitExportStatus] = useState<SummitExportStatus>('idle');
  const [breakGlassArmed, setBreakGlassArmed] = useState(false);
  const [destroyPhrase, setDestroyPhrase] = useState('');
  const [teardownState, setTeardownState] = useState<TeardownState>('idle');

  useEffect(() => {
    document.documentElement.dataset.theme = theme;

    try {
      window.localStorage.setItem('waypoint-theme', theme);
    } catch {
      // Ignore storage failures; theme still applies for this session.
    }
  }, [theme]);

  useEffect(() => {
    const onPopState = () => {
      const route = routeFromPath(window.location.pathname);
      setView(route.view);
      setActiveId(route.phase);
    };
    window.addEventListener('popstate', onPopState);
    return () => window.removeEventListener('popstate', onPopState);
  }, []);

  useEffect(() => {
    document.title = view === 'report' ? 'Waypoint — report snapshot' : `Waypoint — ${waypointDetails[activeId].name}`;
    document.documentElement.dataset.view = view;
  }, [activeId, view]);

  useEffect(() => {
    if (view !== 'trail' || activeId !== 'summit') {
      setTeardownState('idle');
      return;
    }

    if (summitExportStatus !== 'preflight' && summitExportStatus !== 'exporting') {
      return;
    }

    const timers: number[] = [];
    if (summitExportStatus === 'preflight') {
      timers.push(window.setTimeout(() => setSummitExportStatus('exporting'), 420));
    }
    if (summitExportStatus === 'exporting') {
      timers.push(window.setTimeout(() => setSummitExportStatus('verified'), 1200));
    }

    return () => {
      timers.forEach((timer) => window.clearTimeout(timer));
    };
  }, [activeId, summitExportStatus, view]);

  if (view === 'report') {
    return (
      <main className="app-shell report-shell" aria-label="Frozen report snapshot">
        <section className="report-hero artifact">
          <div>
            <p className="eyebrow">Waypoint · frozen report snapshot</p>
            <h1>{reportSnapshot.title}</h1>
            <p className="subtitle">
              Version {reportSnapshot.version} · {reportSnapshot.engagement} · Cutoff {reportSnapshot.cutoff}
            </p>
          </div>
          <div className="report-toolbar">
            <button type="button" className="secondary-link" onClick={() => { setView('trail'); setActiveId('summit'); navigateToPhase('summit'); }}>
              Back to Summit
            </button>
            <button type="button" className="primary-button" onClick={() => window.open(reportPdfPath, '_blank', 'noopener')}>
              Open PDF artifact
            </button>
          </div>
        </section>

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
                  <p>{finding.summary}</p>
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
                  <p><strong>Source:</strong> {item.source}</p>
                  <p><strong>Actor:</strong> {item.actor}</p>
                  <p><strong>Host:</strong> {item.host}</p>
                  <p><strong>Attribution:</strong> {item.attribution}</p>
                  <p className="report-snippet">{item.rawSnippet}</p>
                  <p>{item.note}</p>
                </article>
              ))}
            </div>
          </section>

          <section className="report-section">
            <h2>Bundle manifest</h2>
            <div className="report-grid">
              {reportSnapshot.bundle.payloads.map((payload) => (
                <article key={payload.path} className="report-card">
                  <h3>{payload.path}</h3>
                  <p><strong>Size:</strong> {payload.size} bytes</p>
                  <p className="report-snippet">{payload.sha256}</p>
                </article>
              ))}
            </div>
            <div className="report-grid" style={{ marginTop: '12px' }}>
              <article className="report-card">
                <h3>Archive hash</h3>
                <p className="report-snippet">{reportSnapshot.bundle.outerArchiveSha256}</p>
              </article>
              <article className="report-card">
                <h3>Signature hook</h3>
                <p>{reportSnapshot.bundle.signatures.version}</p>
                <p>{reportSnapshot.bundle.signatures.items.length ? reportSnapshot.bundle.signatures.items.join(', ') : 'empty'}</p>
              </article>
            </div>
          </section>

          <section className="report-section">
            <h2>Verified export receipt</h2>
            <div className="report-grid">
              <article className="report-card">
                <h3>Receipt ID</h3>
                <p className="report-snippet">{reportSnapshot.receipt.id}</p>
                <p>{reportSnapshot.receipt.note}</p>
              </article>
              <article className="report-card">
                <h3>Receipt state</h3>
                <p>{reportSnapshot.receipt.captureState}</p>
                <p><strong>Verified at:</strong> {reportSnapshot.receipt.verifiedAt}</p>
              </article>
              <article className="report-card">
                <h3>Receipt manifest hash</h3>
                <p className="report-snippet">{reportSnapshot.receipt.manifestHash}</p>
              </article>
            </div>
          </section>

          <section className="report-section">
            <h2>Restore and regenerate</h2>
            <div className="report-grid">
              <article className="report-card">
                <h3>Offline tools</h3>
                <ul>{reportSnapshot.bundle.restore.tools.map((tool) => <li key={tool}>{tool}</li>)}</ul>
              </article>
              <article className="report-card">
                <h3>Clean-room checks</h3>
                <ul>{reportSnapshot.bundle.restore.cleanRoom.map((item) => <li key={item}>{item}</li>)}</ul>
              </article>
              <article className="report-card">
                <h3>Malicious paths</h3>
                <ul>{reportSnapshot.bundle.restore.maliciousPaths.map((item) => <li key={item}>{item}</li>)}</ul>
              </article>
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
      </main>
    );
  }

  const activeIndex = waypointOrder.indexOf(activeId);
  const activeWaypoint = waypointDetails[activeId];
  const currentWaypoint = {
    ...activeWaypoint,
    state: 'current' as const,
  };

  const waypoints = useMemo(() => {
    return waypointOrder.map((id, index) => ({
      ...waypointDetails[id],
      state: stateForIndex(index, activeIndex),
    }));
  }, [activeIndex]);

  const previousPhase = waypointOrder[Math.max(0, activeIndex - 1)];
  const nextPhase = waypointOrder[Math.min(waypointOrder.length - 1, activeIndex + 1)];
  const guidePhase = activeIndex < waypointOrder.length - 1 ? nextPhase : previousPhase;
  const guideButtonLabel = activeId === 'summit' ? 'Open report preview →' : activeIndex < waypointOrder.length - 1 ? `Continue into ${waypointDetails[guidePhase].name} →` : `Return to ${waypointDetails[guidePhase].name} →`;

  const phaseSummary = {
    recon: 'Collect signals and keep the pack tidy.',
    attacks: 'Every attempt is captured, attributed, and searchable.',
    findings: 'Promotions stay defensible and linked to evidence.',
    summit: 'Export, verify the manifest, then wipe the disposable box.',
  }[activeId];

  const visibleGuideExplainers = useMemo(() => {
    const query = guideQuery.trim().toLowerCase();
    return guideExplainers.filter((explanation) => {
      const haystack = [
        explanation.phase,
        explanation.title,
        explanation.summary,
        explanation.whenToUse,
        explanation.risks,
        explanation.keywords.join(' '),
      ]
        .join(' ')
        .toLowerCase();

      if (!query) {
        return explanation.phase === activeId;
      }

      return haystack.includes(query);
    });
  }, [activeId, guideQuery]);

  const summitReceipt = reportSnapshot.receipt;
  const summitExportMessage = {
    idle: 'Preflight the bundle before you freeze the snapshot.',
    preflight: 'Preflight running. Capture stays live while the bundle is checked.',
    exporting: 'Exporting now. The live audit trail keeps running while the snapshot is frozen.',
    verified: 'Verified export receipt recorded. Teardown is now guarded by the receipt.',
    failed: 'Checksum drift detected. Re-run the preflight after fixing the bundle.',
    canceled: 'Export canceled. Nothing was torn down, and capture remained intact.',
  }[summitExportStatus];
  const canDestroy = summitExportStatus === 'verified' && breakGlassArmed && destroyPhrase.trim().toUpperCase() === 'WIPE NOW';

  return (
    <main className="app-shell">
      <header className="masthead">
        <div className="masthead-copy">
          <p className="eyebrow">Waypoint · expedition shell</p>
          <h1>Recon / Attacks / Findings</h1>
          <p className="subtitle">A calm trail map for the audit spine, with route skeletons beneath each waypoint.</p>
        </div>

        <div className="masthead-actions">
          <div className="theme-switcher" role="group" aria-label="Theme selection">
            <button type="button" aria-pressed={theme === 'light'} className={theme === 'light' ? 'is-active' : ''} onClick={() => setTheme('light')}>
              Light
            </button>
            <button type="button" aria-pressed={theme === 'dark'} className={theme === 'dark' ? 'is-active' : ''} onClick={() => setTheme('dark')}>
              Dark
            </button>
          </div>

          <div className="progress-pill" aria-label="Trail progress">
            Trail {Math.min(activeIndex + 1, waypointOrder.length)} / {waypointOrder.length} · {activeWaypoint.name}
          </div>

          <div className="metrics" aria-label="Engagement progress">
            <div className="metric">
              <span className="metric-label">Traveled</span>
              <strong>{Math.min(activeIndex + 1, waypointOrder.length)} waypoints</strong>
            </div>
            <div className="metric">
              <span className="metric-label">To summit</span>
              <strong>{Math.max(0, waypointOrder.length - activeIndex - 1)} left</strong>
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
                {waypoints.map((waypoint, index) => {
                  const isCurrent = waypoint.id === activeId;
                  return (
                    <g
                      key={waypoint.id}
                      className={`waypoint ${waypoint.state} ${isCurrent ? 'is-current' : ''}`.trim()}
                    >
                      <circle
                        cx={waypoint.x}
                        cy={waypoint.y}
                        r={waypoint.state === 'current' ? 17 : 12}
                        className={`waypoint-node ${waypoint.state}`}
                      />
                      {waypoint.state === 'completed' ? (
                        <path d={`M${waypoint.x - 4} ${waypoint.y} l3 3 l6 -6`} className="checkmark" />
                      ) : null}
                      {isCurrent ? (
                        <path
                          d={`M${waypoint.x} ${waypoint.y - 8} c -4 5 -5 8 -2 11 c -3 0 -5 3 -2 5 c 2 2 7 2 9 0 c 3 -2 1 -5 -2 -5 c 3 -3 1 -7 -3 -11`}
                          className="campfire"
                        />
                      ) : null}
                      <text x={waypoint.x} y={waypoint.y + 28} textAnchor="middle" className="waypoint-label">
                        {waypoint.label}
                      </text>
                      <title>{`${waypoint.name} — ${stateLabel(waypoint.state)}`}</title>
                    </g>
                  );
                })}
              </svg>

              <div className="waypoint-overlay" aria-label="Trail waypoint shortcuts">
                {waypoints.map((waypoint) => (
                  <button
                    key={waypoint.id}
                    type="button"
                    className={`waypoint-hitbox ${waypoint.id === activeId ? 'is-active' : ''}`}
                    onClick={() => setActiveId(waypoint.id)}
                    aria-current={waypoint.id === activeId ? 'step' : undefined}
                    aria-label={`${waypoint.name}, ${stateLabel(waypoint.state)}${waypoint.id === activeId ? ', you are here' : ''}`}
                    style={{ left: `${(waypoint.x / 640) * 100}%`, top: `${(waypoint.y / 300) * 100}%` }}
                  />
                ))}
              </div>
            </div>
          </section>

          <section className="workspace-panel" aria-label={`${activeWaypoint.name} route skeleton`}>
            <div className="workspace-header">
              <div>
                <p className="workspace-kicker">{workspaceStepLabel(activeIndex)}</p>
                <h2>{activeWaypoint.workspaceTitle}</h2>
              </div>
              <p className="workspace-status">Saved 2 min ago</p>
            </div>

            <p className="workspace-lede">{activeWaypoint.workspaceLede}</p>

            <div className="workspace-grid">
              {activeWaypoint.cards.map((card) => (
                <section key={card.title} className="skeleton-card">
                  <h3>{card.title}</h3>
                  <ul>
                    {card.items.map((item) => (
                      <li key={item}>{item}</li>
                    ))}
                  </ul>
                </section>
              ))}
            </div>

            {activeId === 'summit' ? (
              <section className="summit-flow" aria-label="Summit export and teardown flow">
                <div className="summit-status">
                  <span className={`status-chip ${summitExportStatus}`}>{summitExportStatus}</span>
                  <p>{summitExportMessage}</p>
                </div>

                <div className="summit-controls">
                  <button
                    type="button"
                    className="primary-button"
                    onClick={() => setSummitExportStatus('preflight')}
                    disabled={summitExportStatus === 'preflight' || summitExportStatus === 'exporting'}
                  >
                    Run export preflight
                  </button>
                  <button
                    type="button"
                    className="secondary-link"
                    onClick={() => setSummitExportStatus('canceled')}
                    disabled={summitExportStatus !== 'preflight' && summitExportStatus !== 'exporting'}
                  >
                    Cancel export
                  </button>
                  <button
                    type="button"
                    className="secondary-link"
                    onClick={() => setSummitExportStatus('failed')}
                  >
                    Simulate checksum failure
                  </button>
                </div>

                {summitExportStatus === 'verified' ? (
                  <article className="receipt-card" aria-label="Verified export receipt">
                    <div className="panel-heading compact">
                      <h3>Receipt verified</h3>
                      <p>Capture stayed live while the bundle froze.</p>
                    </div>
                    <dl className="receipt-grid">
                      <div>
                        <dt>Receipt</dt>
                        <dd>{summitReceipt.id}</dd>
                      </div>
                      <div>
                        <dt>Verified</dt>
                        <dd>{summitReceipt.verifiedAt}</dd>
                      </div>
                      <div>
                        <dt>Manifest</dt>
                        <dd className="report-snippet">{summitReceipt.manifestHash}</dd>
                      </div>
                    </dl>
                  </article>
                ) : null}

                {summitExportStatus === 'failed' ? (
                  <article className="receipt-card is-failed" aria-label="Export failure recovery">
                    <div className="panel-heading compact">
                      <h3>Export needs recovery</h3>
                      <p>Rerun the preflight after fixing the bundle or evidence mismatch.</p>
                    </div>
                    <button type="button" className="primary-button" onClick={() => setSummitExportStatus('preflight')}>
                      Retry export preflight
                    </button>
                  </article>
                ) : null}

                <article className="break-glass-panel" aria-label="Break-glass teardown guard">
                  <div className="panel-heading compact">
                    <h3>Break-glass teardown</h3>
                    <p>Export receipt required before the live box can be destroyed.</p>
                  </div>
                  <label className="break-glass-toggle">
                    <input type="checkbox" checked={breakGlassArmed} onChange={(event) => setBreakGlassArmed(event.target.checked)} />
                    Arm the teardown guard
                  </label>
                  <label className="break-glass-input">
                    <span>Type WIPE NOW to confirm</span>
                    <input value={destroyPhrase} onChange={(event) => setDestroyPhrase(event.target.value)} placeholder="WIPE NOW" />
                  </label>
                  <button
                    type="button"
                    className="danger-button"
                    disabled={!canDestroy || teardownState === 'destroyed'}
                    onClick={() => setTeardownState('destroyed')}
                  >
                    {teardownState === 'destroyed' ? 'Teardown queued' : 'Destroy disposable instance'}
                  </button>
                  <p className="summit-warning">
                    {teardownState === 'destroyed'
                      ? 'Break-glass was used after receipt verification. Nothing else should run here.'
                      : canDestroy
                        ? 'Guard armed. The instance can be destroyed deliberately.'
                        : 'Guard remains fogged until the verified receipt and break-glass phrase are in place.'}
                  </p>
                </article>
              </section>
            ) : null}

            <div className="workspace-footer">
              <a className="secondary-link" href={pathForPhase(previousPhase)} onClick={(event) => {
                event.preventDefault();
                setActiveId(previousPhase);
                navigateToPhase(previousPhase);
              }}>
                Back to {waypointDetails[previousPhase].name}
              </a>
              <a
                className="primary-button"
                href={activeId === 'summit' ? pathForReport() : pathForPhase(nextPhase)}
                onClick={(event) => {
                  event.preventDefault();
                  if (activeId === 'summit') {
                    setView('report');
                    navigateToReport();
                    return;
                  }

                  setActiveId(nextPhase);
                  navigateToPhase(nextPhase);
                }}
              >
                {activeId === 'summit' ? 'Open report preview →' : `Continue to ${waypointDetails[nextPhase].name} →`}
              </a>
            </div>
          </section>
        </section>

        <aside className="sidebar" aria-label="Guide and trail details">
          <nav className="route-nav" aria-label="Engagement waypoints">
            <div className="panel-heading">
              <h2>Waypoints</h2>
              <p>All phases stay accessible; fog means no data discovered yet.</p>
            </div>
            <ol>
              {waypoints.map((waypoint, index) => (
                <li key={waypoint.id}>
                  <a
                    href={waypoint.path}
                    className={`route-link ${waypoint.id === activeId ? 'is-active' : ''}`}
                    aria-current={waypoint.id === activeId ? 'step' : undefined}
                    onClick={(event) => {
                      event.preventDefault();
                      setActiveId(waypoint.id);
                      navigateToPhase(waypoint.id);
                    }}
                  >
                    <span className="route-link-copy">
                      <strong>{waypoint.name}</strong>
                      <span>{workspaceStepLabel(index)}</span>
                    </span>
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
              <p>{activeWaypoint.note}</p>
              <div className="guide-tools">
                <label className="guide-search">
                  <span className="sr-only">Search reviewed guide notes</span>
                  <input
                    type="search"
                    value={guideQuery}
                    onChange={(event) => setGuideQuery(event.target.value)}
                    placeholder="Search reviewed notes and context"
                    aria-label="Search reviewed guide notes"
                  />
                </label>
                <button
                  type="button"
                  className="primary-button"
                  onClick={() => {
                    if (activeId === 'summit') {
                      setView('report');
                      navigateToReport();
                      return;
                    }

                    setActiveId(guidePhase);
                    navigateToPhase(guidePhase);
                  }}
                >
                  {guideButtonLabel}
                </button>
              </div>

              <div className="guide-note-list" aria-label="Reviewed guide notes">
                {visibleGuideExplainers.length ? (
                  visibleGuideExplainers.map((explanation) => (
                    <article key={explanation.id} id={explanation.id} className="guide-note-card">
                      <p className="guide-note-kicker">{waypointDetails[explanation.phase].name} · reviewed note</p>
                      <h3>{explanation.title}</h3>
                      <p>{explanation.summary}</p>
                      <dl>
                        <div>
                          <dt>When</dt>
                          <dd>{explanation.whenToUse}</dd>
                        </div>
                        <div>
                          <dt>Risks</dt>
                          <dd>{explanation.risks}</dd>
                        </div>
                      </dl>
                      <a href={explanation.contextHref}>{explanation.contextLabel}</a>
                    </article>
                  ))
                ) : (
                  <p className="guide-note-empty">No reviewed notes match this search.</p>
                )}
              </div>
            </div>
          </section>

          <section className="log-panel" aria-label="Journey log">
            <div className="panel-heading compact">
              <h2>📖 Journey log</h2>
              <p>The audit trail is the journey log — one entry per meaningful action.</p>
            </div>
            <ul>
              {journeyLog.map((entry, index) => (
                <li key={entry} className={index === activeIndex ? 'is-current' : ''}>
                  {entry}
                </li>
              ))}
            </ul>
          </section>

          <section className="route-summary" aria-label="Route summary">
            <div>
              <p className="metric-label">Current waypoint</p>
              <strong>{currentWaypoint.name}</strong>
            </div>
            <p>{phaseSummary}</p>
          </section>
        </aside>
      </div>
    </main>
  );
}
