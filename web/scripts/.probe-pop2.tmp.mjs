import puppeteer from 'puppeteer-core';
const OUT = process.env.SHOT_DIR;
const EID = '90cd07ce-e2e0-49e7-a130-ef3554134ba4';
const browser = await puppeteer.launch({ executablePath: '/usr/bin/chromium', args: ['--no-sandbox', '--disable-dev-shm-usage'], headless: 'new' });
const page = await browser.newPage();
await page.setViewport({ width: 1560, height: 1000 });
await page.goto('http://127.0.0.1:8090/', { waitUntil: 'networkidle2' });
await page.evaluate((t) => localStorage.setItem('waypoint-token', t), 'waypoint-demo-owner-token');
await page.goto(`http://127.0.0.1:8090/engagements/${EID}/subnets`, { waitUntil: 'networkidle2' });
await new Promise((r) => setTimeout(r, 1200));
await page.evaluate(() => { document.querySelector('[data-cidr="10.4.10.0/24"][data-pop]').dispatchEvent(new MouseEvent('click', { bubbles: true })); });
await new Promise((r) => setTimeout(r, 400));
await page.evaluate(() => { document.querySelector('.snpop [data-action="subnet-popout"]').dispatchEvent(new MouseEvent('click', { bubbles: true })); });
await new Promise((r) => setTimeout(r, 1500));
const pages = await browser.pages();
console.log('pages:', pages.length, pages.map((p) => p.url()));
const pop = pages.find((p) => p !== page && p.url() !== 'about:blank#no') || pages[pages.length - 1];
if (pop === page) { console.log('POPOUT: not found'); } else {
  const info = await pop.evaluate(() => ({ title: document.title, dots: document.querySelectorAll('.sngip.on').length, items: document.querySelectorAll('li').length, body: document.body.innerText.slice(0, 120) }));
  console.log('POPOUT:', JSON.stringify(info));
  await pop.setViewport({ width: 640, height: 780 });
  await pop.screenshot({ path: `${OUT}/popout-window.png` });
}
await browser.close();
