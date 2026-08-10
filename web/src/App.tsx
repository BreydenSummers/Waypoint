import { useEffect, useMemo, useState } from 'react';

type ThemeMode = 'light' | 'dark';
type WaypointState = 'completed' | 'current' | 'fog';
type PhaseId = 'recon' | 'attacks' | 'findings' | 'summit';

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

const engagementPath = '/engagements/demo';
const waypointOrder: PhaseId[] = ['recon', 'attacks', 'findings', 'summit'];

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

function phaseFromPath(pathname: string): PhaseId {
  const match = pathname.match(/^\/engagements\/[^/]+\/(recon|attacks|findings|summit)\/?$/);
  if (match) {
    return match[1] as PhaseId;
  }

  return 'attacks';
}

function getInitialPhase(): PhaseId {
  if (typeof window === 'undefined') {
    return 'attacks';
  }

  return phaseFromPath(window.location.pathname);
}

function pathForPhase(phase: PhaseId): string {
  return waypointDetails[phase].path;
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

export function App() {
  const [theme, setTheme] = useState<ThemeMode>(getInitialTheme);
  const [activeId, setActiveId] = useState<PhaseId>(getInitialPhase);

  useEffect(() => {
    document.documentElement.dataset.theme = theme;

    try {
      window.localStorage.setItem('waypoint-theme', theme);
    } catch {
      // Ignore storage failures; theme still applies for this session.
    }
  }, [theme]);

  useEffect(() => {
    const onPopState = () => setActiveId(phaseFromPath(window.location.pathname));
    window.addEventListener('popstate', onPopState);
    return () => window.removeEventListener('popstate', onPopState);
  }, []);

  useEffect(() => {
    const route = pathForPhase(activeId);

    if (window.location.pathname !== route) {
      window.history.replaceState({}, '', route);
    }

    document.title = `Waypoint — ${waypointDetails[activeId].name}`;
  }, [activeId]);

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
  const guideButtonLabel = activeIndex < waypointOrder.length - 1 ? `Continue into ${waypointDetails[guidePhase].name} →` : `Return to ${waypointDetails[guidePhase].name} →`;

  const phaseSummary = {
    recon: 'Collect signals and keep the pack tidy.',
    attacks: 'Every attempt is captured, attributed, and searchable.',
    findings: 'Promotions stay defensible and linked to evidence.',
    summit: 'Export, verify the manifest, then wipe the disposable box.',
  }[activeId];

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
              <a className="primary-button" href={pathForPhase(nextPhase)} onClick={(event) => {
                event.preventDefault();
                setActiveId(nextPhase);
                navigateToPhase(nextPhase);
              }}>
                Continue to {waypointDetails[nextPhase].name} →
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
            <div>
              <h2>Guide's note</h2>
              <p>{activeWaypoint.note}</p>
              <button
                type="button"
                className="primary-button"
                onClick={() => {
                  setActiveId(guidePhase);
                  navigateToPhase(guidePhase);
                }}
              >
                {guideButtonLabel}
              </button>
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
