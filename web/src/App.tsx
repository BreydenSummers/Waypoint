import { useEffect, useMemo, useState } from 'react';

type ThemeMode = 'light' | 'dark';
type WaypointState = 'completed' | 'current' | 'fog';
type PhaseId = 'recon' | 'attacks' | 'findings' | 'summit';
type RouteView = 'trail' | 'report';

type ReportSection = {
  title: string;
  items: string[];
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
    'Export: freeze the snapshot before PDF rendering and bundle manifest generation.',
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
      { title: 'Report copy', items: ['Client-readable summary', 'Machine-readable bundle', 'Hash manifest hook'] },
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
      { title: 'Export bundle', items: ['Database dump', 'Evidence artifacts', 'SHA-256 manifest'] },
      { title: 'Teardown', items: ['Confirm the report reconstructs', 'Leave the instance disposable', 'No lingering state'] },
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
            <button type="button" className="primary-button" onClick={() => window.print()}>
              Print PDF
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
