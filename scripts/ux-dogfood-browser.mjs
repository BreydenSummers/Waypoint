import { mkdir, writeFile } from 'node:fs/promises';
import { resolve } from 'node:path';
import process from 'node:process';

function parseArgs(argv) {
  const args = { baseUrl: 'http://127.0.0.1:8080', outDir: 'docs/release-evidence/ux-dogfood' };
  for (let index = 2; index < argv.length; index += 1) {
    const token = argv[index];
    const next = argv[index + 1];
    if (token === '--base-url' && next) {
      args.baseUrl = next;
      index += 1;
    } else if (token === '--out-dir' && next) {
      args.outDir = next;
      index += 1;
    }
  }
  return args;
}

function toHex(byte) {
  return byte.toString(16).padStart(2, '0');
}

function srgbToLinear(component) {
  const normalized = component / 255;
  return normalized <= 0.03928 ? normalized / 12.92 : ((normalized + 0.055) / 1.055) ** 2.4;
}

function contrastRatio(foreground, background) {
  const fg = foreground.match(/\w\w/g)?.map((value) => Number.parseInt(value, 16));
  const bg = background.match(/\w\w/g)?.map((value) => Number.parseInt(value, 16));
  if (!fg || !bg || fg.length < 3 || bg.length < 3) return null;
  const [fr, fgGreen, fb] = fg;
  const [br, bgGreen, bb] = bg;
  const fgLuma = 0.2126 * srgbToLinear(fr) + 0.7152 * srgbToLinear(fgGreen) + 0.0722 * srgbToLinear(fb);
  const bgLuma = 0.2126 * srgbToLinear(br) + 0.7152 * srgbToLinear(bgGreen) + 0.0722 * srgbToLinear(bb);
  const lighter = Math.max(fgLuma, bgLuma);
  const darker = Math.min(fgLuma, bgLuma);
  return Number(((lighter + 0.05) / (darker + 0.05)).toFixed(2));
}

async function main() {
  const { baseUrl, outDir } = parseArgs(process.argv);
  await mkdir(outDir, { recursive: true });

  let playwright;
  try {
    playwright = await import('playwright');
  } catch (error) {
    const blocker = 'BLOCKER: Playwright is unavailable on this host; install it to capture browser screenshots, accessibility trees, and keyboard-flow evidence.';
    await writeFile(resolve(outDir, 'blocker.txt'), `${blocker}\n${error instanceof Error ? error.message : String(error)}\n`);
    console.error(blocker);
    process.exitCode = 1;
    return;
  }

  const { chromium } = playwright;
  const browser = await chromium.launch({ headless: true });
  try {
    const context = await browser.newContext({
      baseURL: baseUrl,
      viewport: { width: 1440, height: 1200 },
      colorScheme: 'light',
    });
    const page = await context.newPage();

    const artifacts = [];
    const shot = async (name) => {
      const file = resolve(outDir, name);
      await page.screenshot({ path: file, fullPage: true });
      artifacts.push(name);
    };

    async function openRoute(path) {
      await page.goto(path, { waitUntil: 'domcontentloaded' });
      await page.waitForTimeout(800);
    }

    async function clickIfPresent(locator) {
      if (await locator.count()) {
        const target = locator.first();
        if (!(await target.isDisabled())) {
          await target.click();
          await page.waitForTimeout(250);
          return true;
        }
      }
      return false;
    }

    async function fillIfPresent(locator, value) {
      if (await locator.count()) {
        await locator.first().fill(value);
        await page.waitForTimeout(120);
        return true;
      }
      return false;
    }

    await openRoute('/engagements/demo/attacks');
    await shot('desktop-light-trail.png');

    await page.evaluate(() => {
      document.documentElement.dataset.theme = 'dark';
      try {
        window.localStorage.setItem('waypoint-theme', 'dark');
      } catch {
        // ignore
      }
    });
    await openRoute('/engagements/demo/attacks');
    await shot('desktop-dark-trail.png');
    await page.evaluate(() => {
      document.documentElement.dataset.theme = 'light';
      try {
        window.localStorage.setItem('waypoint-theme', 'light');
      } catch {
        // ignore
      }
    });

    await page.keyboard.press('Tab');
    await page.keyboard.press('Tab');
    await page.keyboard.press('Tab');
    await page.keyboard.press('Tab');
    const keyboardTranscript = [];
    for (let index = 0; index < 8; index += 1) {
      const active = await page.evaluate(() => {
        const node = document.activeElement;
        if (!node) return null;
        return {
          tag: node.tagName,
          text: (node.textContent || '').trim().slice(0, 120),
          ariaCurrent: node.getAttribute?.('aria-current'),
          ariaLabel: node.getAttribute?.('aria-label'),
          role: node.getAttribute?.('role'),
        };
      });
      keyboardTranscript.push(active);
      await page.keyboard.press('Tab');
    }
    await writeFile(resolve(outDir, 'keyboard-flow.json'), JSON.stringify(keyboardTranscript, null, 2));

    await openRoute('/engagements/demo/recon');
    await clickIfPresent(page.locator('button.finding-card'));
    await clickIfPresent(page.getByRole('button', { name: /Use as source/i }));
    await clickIfPresent(page.getByRole('button', { name: /Use as target/i }));
    await clickIfPresent(page.getByRole('button', { name: /Preview merge/i }));
    await clickIfPresent(page.getByRole('button', { name: /Apply merge/i }));
    await clickIfPresent(page.getByRole('button', { name: /Preview split/i }));
    await clickIfPresent(page.getByRole('button', { name: /Apply split/i }));
    await shot('desktop-light-recon.png');
    await shot('desktop-light-recon-interactions.png');

    await openRoute('/engagements/demo/attacks');
    await clickIfPresent(page.locator('button.attack-row-button'));
    await clickIfPresent(page.locator('button.evidence-box'));
    await clickIfPresent(page.getByRole('button', { name: /Send to Findings/i }));
    await shot('desktop-light-attacks.png');
    await shot('desktop-light-attacks-interactions.png');

    await openRoute('/engagements/demo/findings');
    await clickIfPresent(page.locator('button.finding-card'));
    await fillIfPresent(page.getByLabel('Title').first(), 'Dogfood review');
    await fillIfPresent(page.getByLabel('Remediation').first(), 'Keep provenance attached to the confirmed result.');
    await clickIfPresent(page.getByRole('button', { name: /Save revision/i }));
    await shot('desktop-light-findings.png');
    await shot('desktop-light-findings-interactions.png');

    await openRoute('/engagements/demo/summit');
    await clickIfPresent(page.getByRole('button', { name: /Start export job/i }));
    await page.waitForTimeout(1200);
    await clickIfPresent(page.getByRole('button', { name: /Refresh jobs/i }));
    await fillIfPresent(page.locator('input[placeholder="destroy verified engagement data"]'), 'destroy verified engagement data');
    await clickIfPresent(page.getByRole('button', { name: /Authorize teardown/i }));
    await clickIfPresent(page.getByRole('button', { name: /Consume authorization/i }));
    await shot('desktop-light-summit.png');
    await shot('desktop-light-summit-interactions.png');

    await openRoute('/engagements/demo/summit/report');
    await clickIfPresent(page.getByRole('button', { name: /Open PDF artifact/i }));
    await shot('desktop-light-report.png');

    await context.setOffline(true);
    await page.waitForTimeout(1800);
    await shot('desktop-light-sse-offline.png');
    await context.setOffline(false);
    await page.waitForTimeout(2500);
    await shot('desktop-light-sse-recovered.png');

    const a11yTree = await page.accessibility.snapshot({ interestingOnly: false });
    await writeFile(resolve(outDir, 'accessibility-tree.json'), JSON.stringify(a11yTree, null, 2));

    const contrast = await page.evaluate(() => {
      const styles = getComputedStyle(document.documentElement);
      return {
        parchment: styles.getPropertyValue('--artifact-parchment').trim(),
        cocoa: styles.getPropertyValue('--cocoa').trim(),
        deepBark: styles.getPropertyValue('--deep-bark').trim(),
        bark: styles.getPropertyValue('--bark').trim(),
        wheat: styles.getPropertyValue('--wheat').trim(),
      };
    });
    const contrastReport = {
      'cocoa on parchment': contrastRatio(contrast.cocoa.replace('#', ''), contrast.parchment.replace('#', '')),
      'deep bark on parchment': contrastRatio(contrast.deepBark.replace('#', ''), contrast.parchment.replace('#', '')),
      'wheat on bark': contrastRatio(contrast.wheat.replace('#', ''), contrast.bark.replace('#', '')),
    };
    await writeFile(resolve(outDir, 'contrast.json'), JSON.stringify(contrastReport, null, 2));

    await page.emulateMedia({ reducedMotion: 'reduce' });
    await openRoute('/engagements/demo/attacks');
    await shot('desktop-light-reduced-motion.png');
    const reducedMotionState = await page.evaluate(() => ({
      reducedMotion: window.matchMedia('(prefers-reduced-motion: reduce)').matches,
    }));
    await writeFile(resolve(outDir, 'reduced-motion.json'), JSON.stringify(reducedMotionState, null, 2));

    async function mobileCapture(theme, path, suffix) {
      const mobileContext = await browser.newContext({
        baseURL,
        viewport: { width: 390, height: 844 },
        colorScheme: theme,
      });
      const mobilePage = await mobileContext.newPage();
      await mobilePage.emulateMedia({ reducedMotion: 'no-preference' });
      await mobilePage.goto(path, { waitUntil: 'domcontentloaded' });
      await mobilePage.waitForTimeout(800);
      await mobilePage.screenshot({ path: resolve(outDir, `mobile-${theme}-${suffix}.png`), fullPage: true });
      await mobileContext.close();
      artifacts.push(`mobile-${theme}-${suffix}.png`);
    }

    await mobileCapture('light', '/engagements/demo/attacks', 'trail');
    await mobileCapture('dark', '/engagements/demo/attacks', 'trail');

    try {
      const axe = await import('axe-core');
      await page.addScriptTag({ content: axe.source });
      const axeResult = await page.evaluate(async () => {
        // @ts-expect-error axe is injected by the script tag above.
        return await axe.run(document, {
          resultTypes: ['violations', 'passes', 'incomplete'],
        });
      });
      await writeFile(resolve(outDir, 'axe.json'), JSON.stringify(axeResult, null, 2));
    } catch (error) {
      await writeFile(resolve(outDir, 'axe.json'), JSON.stringify({ blocker: 'axe-core unavailable on this host', detail: error instanceof Error ? error.message : String(error) }, null, 2));
    }

    await writeFile(resolve(outDir, 'index.json'), JSON.stringify({ baseUrl, artifacts }, null, 2));
    console.log(`captured Waypoint UX evidence in ${outDir}`);
  } finally {
    await browser.close();
  }
}

await main();
