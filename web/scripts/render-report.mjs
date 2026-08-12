import { mkdtemp, mkdir, readFile, rm, writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath, pathToFileURL } from 'node:url';
import { spawnSync } from 'node:child_process';

import { buildReportHtml } from './report-renderer.mjs';

const webRoot = resolve(dirname(fileURLToPath(import.meta.url)), '..');
const repoRoot = resolve(webRoot, '..');
const snapshotPath = resolve(repoRoot, process.argv[2] || 'contracts/v1/fixtures/report-snapshot.json');
const output = resolve(repoRoot, process.argv[3] || 'waypoint-report.pdf');
const chromium = process.env.WAYPOINT_CHROMIUM || '/usr/bin/chromium';

const snapshot = JSON.parse(await readFile(snapshotPath, 'utf8'));
const tempDir = await mkdtemp(join(tmpdir(), 'waypoint-report-'));

try {
  const htmlPath = join(tempDir, 'report.html');
  await writeFile(htmlPath, buildReportHtml(snapshot), 'utf8');
  await mkdir(dirname(output), { recursive: true });

  const result = spawnSync(
    chromium,
    [
      '--headless=new',
      '--disable-gpu',
      '--no-first-run',
      '--no-default-browser-check',
      '--disable-dev-shm-usage',
      '--print-to-pdf-no-header',
      `--print-to-pdf=${output}`,
      pathToFileURL(htmlPath).href,
    ],
    { stdio: 'inherit' },
  );

  if (result.error) {
    throw result.error;
  }

  if (result.status !== 0) {
    process.exit(result.status ?? 1);
  }

  console.log(`rendered ${output} with ${chromium} from ${snapshotPath}`);
} finally {
  await rm(tempDir, { recursive: true, force: true });
}
