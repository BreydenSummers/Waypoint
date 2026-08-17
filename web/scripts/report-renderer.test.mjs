import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import { dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

import { buildReportHtml } from './report-renderer.mjs';

const webRoot = dirname(fileURLToPath(import.meta.url));
const fixture = JSON.parse(await readFile(resolve(webRoot, '../../contracts/v1/fixtures/report-snapshot.json'), 'utf8'));
const snapshot = {
  ...fixture,
  findings: fixture.findings.map((finding, index) => index === 0 ? { ...finding, status: 'confirmed', promotedBy: 'alex.operator', promotedAt: '2025-01-10T08:45:00Z', affectedEntityIds: ['11111111-1111-4111-8111-111111111111'] } : finding),
  evidence: fixture.evidence.map((item, index) => index === 1 ? { ...item, egress: '203.0.113.26', initiatedBy: 'manual', parseStatus: 'raw' } : item),
  knownCaptureGaps: [
    { claimKind: 'result', status: 'pending', reason: 'missing_captured_source_action', observedBy: { handle: 'alex.operator' } },
    'Unknown tools remain raw-first; a missing parser does not drop evidence.',
  ],
};

const html = buildReportHtml(snapshot);
assert.match(html, /Runtime report snapshot|Frozen report snapshot/);
assert.ok(html.includes('&lt;script&gt;alert(&quot;x&quot;)&lt;/script&gt;'), 'raw evidence is escaped');
assert.ok(!html.includes('<script>alert("x")</script>'), 'raw evidence is not rendered as HTML');
assert.ok(html.includes('203.0.113.26'), 'egress attribution is rendered');
assert.ok(html.includes('pending'), 'capture gaps are rendered');
assert.ok(html.includes('bundle/report/frozen-report.pdf'), 'bundle payloads are rendered');
assert.ok(html.includes('SHA-256 verified') || html.includes('Hash verified, not signed'), 'report copy is explicit about hash verification');
assert.ok(html.includes('empty'), 'signature hook is visibly empty');
assert.ok(!html.toLowerCase().includes('signature verified'), 'report must not imply cryptographic signing');
