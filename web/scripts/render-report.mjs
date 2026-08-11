import { spawnSync } from 'node:child_process';
import { resolve } from 'node:path';

const chromium = process.env.WAYPOINT_CHROMIUM || process.env.CHROMIUM || 'chromium';
const url = process.argv[2] || 'http://127.0.0.1:18080/engagements/demo/summit/report';
const output = process.argv[3] || resolve(process.cwd(), 'waypoint-report.pdf');

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
    url,
  ],
  { stdio: 'inherit' },
);

if (result.error) {
  throw result.error;
}

if (result.status !== 0) {
  process.exit(result.status ?? 1);
}

console.log(`rendered ${output} with ${chromium}`);
