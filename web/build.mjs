import { access, copyFile, mkdir } from 'node:fs/promises';
import { execFileSync } from 'node:child_process';
import { dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const webRoot = dirname(fileURLToPath(import.meta.url));
const distRoot = resolve(webRoot, '../internal/webassets/dist');

await mkdir(resolve(distRoot, 'assets'), { recursive: true });
await copyFile(resolve(webRoot, 'index.html'), resolve(distRoot, 'index.html'));
await copyFile(resolve(webRoot, 'src/styles.css'), resolve(distRoot, 'assets/waypoint.css'));

for (const asset of ['assets/waypoint.js', 'assets/waypoint.css']) {
  await access(resolve(distRoot, asset));
}

execFileSync(process.execPath, ['scripts/check.mjs'], {
  cwd: webRoot,
  stdio: 'inherit',
});
