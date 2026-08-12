#!/usr/bin/env node
import { regenerateReport } from '../../web/scripts/bundle-tools.mjs';

const bundleRoot = process.argv[2] ? process.argv[2] : '.';
const outputPath = process.argv[3];

try {
  const result = await regenerateReport(bundleRoot, outputPath);
  if (result.html) {
    process.stdout.write(result.html);
  } else {
    process.stdout.write(`${JSON.stringify({ status: 'rendered', ...result }, null, 2)}\n`);
  }
} catch (error) {
  console.error(error instanceof Error ? error.message : String(error));
  process.exit(1);
}
