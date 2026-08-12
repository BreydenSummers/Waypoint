#!/usr/bin/env node
import { verifyBundle } from '../../web/scripts/bundle-tools.mjs';

const bundleRoot = process.argv[2] ? process.argv[2] : '.';

try {
  const result = await verifyBundle(bundleRoot);
  process.stdout.write(`${JSON.stringify({ status: 'verified', ...result }, null, 2)}\n`);
} catch (error) {
  console.error(error instanceof Error ? error.message : String(error));
  process.exit(1);
}
