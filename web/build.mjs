import { build } from 'esbuild';
import { copyFile, mkdir, rm } from 'node:fs/promises';
import { dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const webRoot = dirname(fileURLToPath(import.meta.url));
const distRoot = resolve(webRoot, '../internal/webassets/dist');

await rm(distRoot, { recursive: true, force: true });
await mkdir(resolve(distRoot, 'assets'), { recursive: true });

await build({
  entryPoints: [resolve(webRoot, 'src/main.tsx')],
  bundle: true,
  format: 'esm',
  outfile: resolve(distRoot, 'assets/waypoint.js'),
  jsx: 'automatic',
  target: ['es2020'],
  sourcemap: false,
  minify: true,
  define: {
    'process.env.NODE_ENV': '"production"'
  },
  loader: {
    '.css': 'css'
  }
});

await copyFile(resolve(webRoot, 'index.html'), resolve(distRoot, 'index.html'));
