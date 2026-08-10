import { copyFile, mkdir, access } from 'node:fs/promises';
import { dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const webRoot = dirname(fileURLToPath(import.meta.url));
const distRoot = resolve(webRoot, '../internal/webassets/dist');

await mkdir(resolve(distRoot, 'assets'), { recursive: true });
await copyFile(resolve(webRoot, 'index.html'), resolve(distRoot, 'index.html'));

for (const asset of ['assets/waypoint.js', 'assets/waypoint.css']) {
  await access(resolve(distRoot, asset));
}
