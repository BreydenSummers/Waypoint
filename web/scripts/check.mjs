import { readFile } from 'node:fs/promises';
import { dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const webRoot = resolve(dirname(fileURLToPath(import.meta.url)), '..');
const app = await readFile(resolve(webRoot, 'src/App.tsx'), 'utf8');
const index = await readFile(resolve(webRoot, 'index.html'), 'utf8');
const distIndex = await readFile(resolve(webRoot, '../internal/webassets/dist/index.html'), 'utf8');

if (app.includes("window.history.replaceState({}, '', route)") || app.includes("window.history.replaceState({}, \"\", route)")) {
  throw new Error('client route normalization still rewrites the SPA fallback URL');
}

for (const html of [index, distIndex]) {
  if (!html.includes('id="root"') || !html.includes('/assets/waypoint.js') || !html.includes('/assets/waypoint.css')) {
    throw new Error('web shell is missing the app root or asset references');
  }
}

console.log('web skeleton check passed');
