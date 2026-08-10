import { useMemo, useState } from 'react';

type WaypointState = 'completed' | 'current' | 'fog';

type Waypoint = {
  id: string;
  name: string;
  label: string;
  state: WaypointState;
  x: number;
  y: number;
  note: string;
};

const waypoints: Waypoint[] = [
  {
    id: 'recon',
    name: 'Recon',
    label: 'Recon',
    state: 'completed',
    x: 72,
    y: 248,
    note: 'Imported context is in your pack and ready to revisit.',
  },
  {
    id: 'attacks',
    name: 'Attacks',
    label: 'Attacks',
    state: 'current',
    x: 228,
    y: 182,
    note: 'Captured actions land here with full attribution.',
  },
  {
    id: 'findings',
    name: 'Findings',
    label: 'Findings',
    state: 'fog',
    x: 430,
    y: 128,
    note: 'Nothing promoted yet — the ridge is still clearing.',
  },
  {
    id: 'summit',
    name: 'Summit',
    label: 'Summit',
    state: 'fog',
    x: 586,
    y: 64,
    note: 'Export waits here once the trail is complete.',
  },
];

const journeyLog = [
  'Day 1 — Basecamp set. Team invited, scope loaded.',
  'Day 2 — Creek crossed. 240 records packed.',
  'Now — Made camp in Attacks. The trail log is live.',
];

function stateLabel(state: WaypointState): string {
  switch (state) {
    case 'completed':
      return 'completed';
    case 'current':
      return 'current';
    case 'fog':
      return 'fog';
    default:
      return state;
  }
}

export function App() {
  const [activeId, setActiveId] = useState('attacks');

  const active = useMemo(
    () => waypoints.find((waypoint) => waypoint.id === activeId) ?? waypoints[1],
    [activeId],
  );

  return (
    <main className="app-shell">
      <header className="masthead">
        <div>
          <p className="eyebrow">Waypoint · expedition shell</p>
          <h1>Recon / Attacks / Findings</h1>
          <p className="subtitle">Journey log, guide note, and export trail in one calm map.</p>
        </div>
        <div className="metrics" aria-label="Engagement progress">
          <div className="metric">
            <span className="metric-label">Traveled</span>
            <strong>3.1 mi</strong>
          </div>
          <div className="metric">
            <span className="metric-label">To summit</span>
            <strong>2.4 mi</strong>
          </div>
        </div>
      </header>

      <section className="map-card" aria-label="Engagement trail map">
        <ol className="sr-only">
          {waypoints.map((waypoint) => (
            <li key={waypoint.id}>{`${waypoint.name} — ${stateLabel(waypoint.state)}`}</li>
          ))}
        </ol>
        <div className="map-stage">
          <svg viewBox="0 0 640 300" role="img" aria-label="Trail map with waypoint buttons">
            <rect width="640" height="300" className="map-terrain" />
            <path d="M60 252 C 132 234, 148 194, 206 182 C 270 168, 286 220, 350 204 C 402 190, 420 148, 472 126 C 516 108, 548 84, 590 60"
              className="trail-path" />
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
            {waypoints.map((waypoint) => {
              const current = waypoint.id === active.id;
              return (
                <g key={waypoint.id} className={`waypoint ${waypoint.state} ${current ? 'is-current' : ''}`.trim()}>
                  <circle
                    cx={waypoint.x}
                    cy={waypoint.y}
                    r={waypoint.state === 'current' ? 17 : 12}
                    className={`waypoint-node ${waypoint.state}`}
                  />
                  {waypoint.state === 'completed' ? (
                    <path d={`M${waypoint.x - 4} ${waypoint.y} l3 3 l6 -6`} className="checkmark" />
                  ) : null}
                  {current ? (
                    <path d={`M${waypoint.x} ${waypoint.y - 8} c -4 5 -5 8 -2 11 c -3 0 -5 3 -2 5 c 2 2 7 2 9 0 c 3 -2 1 -5 -2 -5 c 3 -3 1 -7 -3 -11`} className="campfire" />
                  ) : null}
                  <text x={waypoint.x} y={waypoint.y + 27} textAnchor="middle" className="waypoint-label">
                    {waypoint.label}
                  </text>
                </g>
              );
            })}
          </svg>
          <div className="waypoint-overlay">
            {waypoints.map((waypoint) => (
              <button
                key={waypoint.id}
                type="button"
                className={`waypoint-hitbox ${waypoint.id === active.id ? 'is-active' : ''}`}
                onClick={() => setActiveId(waypoint.id)}
                aria-current={waypoint.id === active.id ? 'step' : undefined}
                aria-label={`${waypoint.name}, ${stateLabel(waypoint.state)}${waypoint.id === active.id ? ', you are here' : ''}`}
                style={{ left: `${(waypoint.x / 640) * 100}%`, top: `${(waypoint.y / 300) * 100}%` }}
              />
            ))}
          </div>
        </div>
      </section>

      <section className="panels">
        <article className="guide-panel artifact">
          <div className="panel-icon" aria-hidden="true">🧭</div>
          <div>
            <h2>Guide's note</h2>
            <p>{active.note}</p>
            <button type="button" className="primary-button">
              Continue into {active.name} →
            </button>
          </div>
        </article>

        <article className="log-panel">
          <h2>📖 Journey log</h2>
          <ul>
            {journeyLog.map((entry) => (
              <li key={entry}>{entry}</li>
            ))}
          </ul>
        </article>
      </section>

      <section className="workspace-panel">
        <div>
          <p className="workspace-kicker">Stage workspace</p>
          <h2>{active.name}</h2>
        </div>
        <p>
          Placeholder work surface for captured actions, findings promotion, and export-ready notes.
        </p>
      </section>
    </main>
  );
}
