export function escapeHtml(value) {
  return String(value ?? '')
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#39;');
}

function renderList(items) {
  if (!Array.isArray(items) || items.length === 0) {
    return '<li>None recorded.</li>';
  }
  return items.map((item) => `<li>${escapeHtml(item)}</li>`).join('');
}

function renderCards(items, renderItem) {
  return items.map((item) => `<article class="report-card">${renderItem(item)}</article>`).join('');
}

function renderInlineList(items) {
  if (!Array.isArray(items) || items.length === 0) {
    return 'None recorded.';
  }
  return items.map((item) => escapeHtml(item)).join(', ');
}

function renderCaptureGap(item) {
  if (item && typeof item === 'object') {
    const title = item.claimKind ? `${item.claimKind} claim` : 'Capture gap';
    const status = item.status ? ` · ${item.status}` : '';
    const reason = item.reason || item.notes || '';
    const observedBy = item.observedBy?.handle || item.observedBy?.title || '';
    const sourceActionId = item.sourceActionId ? ` · source ${item.sourceActionId}` : '';
    return `<li><strong>${escapeHtml(title)}</strong>${escapeHtml(status)}${sourceActionId ? escapeHtml(sourceActionId) : ''}${observedBy ? ` · ${escapeHtml(observedBy)}` : ''}${reason ? ` — ${escapeHtml(reason)}` : ''}</li>`;
  }
  return `<li>${escapeHtml(item)}</li>`;
}

export function buildReportHtml(snapshot) {
  const bundle = snapshot?.bundle ?? {};
  const contractVersion = snapshot?.contractVersion ?? snapshot?.version ?? 'v1';
  const signatures = bundle.signatures ?? { version: 'v1', items: [] };
  const restore = bundle.restore ?? { tools: [], cleanRoom: [], maliciousPaths: [] };
  const payloads = Array.isArray(bundle.payloads) ? bundle.payloads : [];
  const findings = Array.isArray(snapshot?.findings) ? snapshot.findings : [];
  const evidence = Array.isArray(snapshot?.evidence) ? snapshot.evidence : [];
  const attribution = Array.isArray(snapshot?.attribution) ? snapshot.attribution : [];
  const scope = Array.isArray(snapshot?.scope) ? snapshot.scope : [];
  const methodology = Array.isArray(snapshot?.methodology) ? snapshot.methodology : [];
  const knownCaptureGaps = Array.isArray(snapshot?.knownCaptureGaps) ? snapshot.knownCaptureGaps : [];

  return `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>${escapeHtml(snapshot?.title || 'Waypoint report')}</title>
  <style>
    :root {
      color-scheme: light;
      --deep-bark: #3B2617;
      --bark: #4A2F1B;
      --saddle: #6B4423;
      --trail: #8B5E34;
      --harvest: #BA7517;
      --lantern: #EF9F27;
      --wheat: #FAC775;
      --parchment: #FAEEDA;
      --map-cream: #E8DCC3;
      --contour: #D4C4A0;
      --dark-cocoa: #633806;
      --cocoa: #854F0B;
      --stone: #B4A78C;
    }
    * { box-sizing: border-box; }
    body {
      margin: 0;
      padding: 32px;
      background: #f4eee0;
      color: var(--deep-bark);
      font: 14px/1.5 system-ui, sans-serif;
    }
    main { max-width: 980px; margin: 0 auto; }
    .hero, .section, .card {
      border: 1px solid var(--contour);
      border-radius: 16px;
      background: var(--parchment);
      box-shadow: 0 10px 28px rgba(59, 38, 23, 0.08);
    }
    .hero { padding: 20px; margin-bottom: 16px; }
    .eyebrow {
      margin: 0 0 6px;
      text-transform: uppercase;
      letter-spacing: 0.12em;
      font-size: 12px;
      color: var(--cocoa);
    }
    h1, h2, h3, p, ul { margin: 0; }
    h1 { font-size: 30px; line-height: 1.1; color: var(--dark-cocoa); }
    .subtitle { margin-top: 8px; color: var(--cocoa); }
    .meta { margin-top: 14px; display: flex; flex-wrap: wrap; gap: 8px; }
    .pill { padding: 6px 10px; border-radius: 999px; background: rgba(186, 117, 23, 0.12); color: var(--dark-cocoa); }
    .section { padding: 18px; margin-top: 14px; }
    .section h2 { font-size: 18px; margin-bottom: 10px; color: var(--dark-cocoa); }
    .grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(240px, 1fr)); gap: 12px; }
    .card { padding: 14px; background: rgba(255,255,255,0.35); }
    .card h3 { font-size: 15px; margin-bottom: 8px; color: var(--dark-cocoa); }
    .badge { display: inline-block; margin-bottom: 8px; font-size: 12px; text-transform: uppercase; letter-spacing: 0.08em; color: var(--saddle); }
    strong { color: var(--dark-cocoa); }
    pre { margin: 10px 0 0; white-space: pre-wrap; word-break: break-word; font: inherit; color: var(--cocoa); }
    ul { padding-left: 18px; }
    li + li { margin-top: 4px; }
    .monospace { font-family: ui-monospace, SFMono-Regular, Menlo, monospace; }
  </style>
</head>
<body>
  <main>
    <section class="hero">
      <p class="eyebrow">Waypoint · frozen report snapshot</p>
      <h1>${escapeHtml(snapshot?.title || 'Runtime report snapshot')}</h1>
      <p class="subtitle">Version ${escapeHtml(contractVersion)} · ${escapeHtml(snapshot?.engagement || 'Unknown engagement')} · Cutoff ${escapeHtml(snapshot?.cutoff || 'unknown')}</p>
      <div class="meta">
        <span class="pill">Hash verified, not signed</span>
        <span class="pill">Offline renderer</span>
        <span class="pill">Snapshot frozen before print</span>
      </div>
    </section>

    <section class="section">
      <h2>Scope</h2>
      <ul>${renderList(scope)}</ul>
    </section>

    <section class="section">
      <h2>Methodology</h2>
      <ul>${renderList(methodology)}</ul>
    </section>

    <section class="section">
      <h2>Findings</h2>
      <div class="grid">
        ${renderCards(findings, (finding) => `
          <p class="badge">${escapeHtml(finding?.severity || 'Unspecified')}</p>
          <h3>${escapeHtml(finding?.title || 'Untitled finding')}</h3>
          <p>${escapeHtml(finding?.summary || '')}</p>
          <p><strong>Status:</strong> ${escapeHtml(finding?.status || 'open')}</p>
          <p><strong>Evidence:</strong> ${renderInlineList(finding?.evidence)}</p>
          <p><strong>Promoted by:</strong> ${escapeHtml(finding?.promotedBy || '')}</p>
          <p><strong>Promoted at:</strong> ${escapeHtml(finding?.promotedAt || '')}</p>
          <p><strong>Remediation:</strong> ${escapeHtml(finding?.remediation || '')}</p>
          <p><strong>Affected entities:</strong> ${renderInlineList(finding?.affectedEntityIds)}</p>
        `)}
      </div>
    </section>

    <section class="section">
      <h2>Evidence</h2>
      <div class="grid">
        ${renderCards(evidence, (item) => `
          <p class="badge">${escapeHtml(item?.label || 'Evidence')}</p>
          <p><strong>Source:</strong> ${escapeHtml(item?.source || item?.command || '')}</p>
          <p><strong>Target:</strong> ${escapeHtml(item?.target || '')}</p>
          <p><strong>Actor:</strong> ${escapeHtml(item?.actor || '')}</p>
          <p><strong>Host:</strong> ${escapeHtml(item?.host || '')}</p>
          <p><strong>Egress:</strong> ${escapeHtml(item?.egress || '')}</p>
          <p><strong>Initiated by:</strong> ${escapeHtml(item?.initiatedBy || '')}</p>
          <p><strong>Parse status:</strong> ${escapeHtml(item?.parseStatus || '')}</p>
          <p><strong>Attribution:</strong> ${escapeHtml(item?.attribution || '')}</p>
          <pre>${escapeHtml(item?.rawSnippet || '')}</pre>
          <p>${escapeHtml(item?.note || '')}</p>
        `)}
      </div>
    </section>

    <section class="section">
      <h2>Bundle manifest</h2>
      <div class="grid">
        ${payloads.map((payload) => `
          <article class="card">
            <h3 class="monospace">${escapeHtml(payload?.path || '')}</h3>
            <p><strong>Size:</strong> ${escapeHtml(payload?.size || 0)} bytes</p>
            <pre>${escapeHtml(payload?.sha256 || '')}</pre>
          </article>
        `).join('')}
      </div>
      <div class="grid" style="margin-top: 12px;">
        <article class="card">
          <h3>Archive hash</h3>
          <pre>${escapeHtml(bundle?.outerArchiveSha256 || '')}</pre>
        </article>
        <article class="card">
          <h3>Signature hook</h3>
          <p>${escapeHtml(signatures.version || 'v1')}</p>
          <p>${signatures.items?.length ? escapeHtml(signatures.items.join(', ')) : 'empty'}</p>
        </article>
      </div>
    </section>

    <section class="section">
      <h2>Verified export receipt</h2>
      <div class="grid">
        <article class="card">
          <h3>Receipt ID</h3>
          <pre>${escapeHtml(snapshot?.receipt?.id || '')}</pre>
          <p>${escapeHtml(snapshot?.receipt?.note || '')}</p>
        </article>
        <article class="card">
          <h3>Receipt state</h3>
          <p>${escapeHtml(snapshot?.receipt?.captureState || '')}</p>
          <p><strong>Verified at:</strong> ${escapeHtml(snapshot?.receipt?.verifiedAt || '')}</p>
        </article>
        <article class="card">
          <h3>Receipt manifest hash</h3>
          <pre>${escapeHtml(snapshot?.receipt?.manifestHash || '')}</pre>
        </article>
      </div>
    </section>

    <section class="section">
      <h2>Restore and regenerate</h2>
      <div class="grid">
        <article class="card">
          <h3>Offline tools</h3>
          <ul>${renderList(restore.tools)}</ul>
        </article>
        <article class="card">
          <h3>Clean-room checks</h3>
          <ul>${renderList(restore.cleanRoom)}</ul>
        </article>
        <article class="card">
          <h3>Malicious paths</h3>
          <ul>${renderList(restore.maliciousPaths)}</ul>
        </article>
      </div>
    </section>

    <section class="section">
      <h2>Attribution</h2>
      <div class="grid">
        ${renderCards(attribution, (section) => `
          <h3>${escapeHtml(section?.title || '')}</h3>
          <ul>${renderList(section?.items)}</ul>
        `)}
      </div>
    </section>

    <section class="section">
      <h2>Known capture gaps</h2>
      <ul>${Array.isArray(knownCaptureGaps) && knownCaptureGaps.length ? knownCaptureGaps.map((item) => renderCaptureGap(item)).join('') : '<li>None recorded.</li>'}</ul>
    </section>
  </main>
</body>
</html>`;
}
