// cdp.mjs - drive one persistent headless Chromium over CDP, one command per call.
//
// Why this exists: the agent (often a weaker model) should not have to orchestrate a
// 25-tool MCP session to look at a page. It runs ONE shell command per step and reads
// plain text back, exactly like the white-hat agent's browser skill does.
//
// The browser is launched once and stays up (CDP on 127.0.0.1:9222), so every later
// command reattaches to the same page instead of paying a cold start.
//
//   node /app/skills/browser/cdp.mjs start
//   node /app/skills/browser/cdp.mjs goto https://example.com
//   node /app/skills/browser/cdp.mjs snapshot
//   node /app/skills/browser/cdp.mjs click "text=Sign in"
//   node /app/skills/browser/cdp.mjs fill "#email" "a@b.c"
//   node /app/skills/browser/cdp.mjs eval "document.title"
//   node /app/skills/browser/cdp.mjs screenshot /tmp/page.png
//   node /app/skills/browser/cdp.mjs stop
//
// Selectors: anything Playwright's page.locator() accepts - CSS (#id, .class, input),
// "text=Sign in", "role=button[name='Send']". No ref bookkeeping needed.

import { spawn } from 'node:child_process';
import { existsSync } from 'node:fs';
import { createRequire } from 'node:module';

// playwright is installed globally in the image, and ESM import does not honour
// NODE_PATH, so resolve it through a require rooted at the global module dir.
function loadPlaywright() {
  for (const base of ['/app/skills/browser/', '/usr/lib/node_modules/', '/usr/local/lib/node_modules/']) {
    try {
      return createRequire(base)('playwright');
    } catch {
      /* try the next root */
    }
  }
  throw new Error('playwright module not found (looked in the skill dir and the global node_modules)');
}
const { chromium } = loadPlaywright();

const CDP = process.env.CDP_URL || 'http://127.0.0.1:9222';
const PROFILE = process.env.CDP_PROFILE || '/tmp/cdp-profile';

async function cdpUp() {
  try {
    const r = await fetch(CDP + '/json/version', { signal: AbortSignal.timeout(1500) });
    return r.ok;
  } catch {
    return false;
  }
}

async function start() {
  if (await cdpUp()) {
    console.log('already running: ' + CDP);
    return;
  }
  const exe = chromium.executablePath();
  if (!existsSync(exe)) {
    throw new Error('chromium not found at ' + exe + ' (set PLAYWRIGHT_BROWSERS_PATH)');
  }
  const args = [
    '--headless=new',
    '--remote-debugging-address=127.0.0.1',
    '--remote-debugging-port=9222',
    '--user-data-dir=' + PROFILE,
    '--no-first-run',
    '--no-default-browser-check',
    '--disable-gpu',
    '--disable-dev-shm-usage',
    // the container runs as root; without these chromium refuses to start
    '--no-sandbox',
    '--disable-setuid-sandbox',
    'about:blank',
  ];
  const child = spawn(exe, args, { detached: true, stdio: 'ignore' });
  child.unref();
  for (let i = 0; i < 40; i++) {
    if (await cdpUp()) {
      console.log('started: ' + CDP + ' (pid ' + child.pid + ')');
      return;
    }
    await new Promise((r) => setTimeout(r, 250));
  }
  throw new Error('chromium did not open the CDP endpoint in 10s');
}

async function withPage(fn) {
  if (!(await cdpUp())) {
    throw new Error('browser is not running - run: node /app/skills/browser/cdp.mjs start');
  }
  const browser = await chromium.connectOverCDP(CDP);
  try {
    const ctx = browser.contexts()[0] ?? (await browser.newContext());
    const page = ctx.pages()[0] ?? (await ctx.newPage());
    return await fn(page);
  } finally {
    await browser.close();
  }
}

// snapshot prints an aria tree with roles and names - enough for a model to pick a
// target without reading raw HTML, and far cheaper than dumping the DOM.
async function snapshot(page) {
  const url = page.url();
  const title = await page.title().catch(() => '');
  let tree = '';
  try {
    tree = await page.locator('body').ariaSnapshot({ timeout: 5000 });
  } catch {
    tree = (await page.locator('body').innerText({ timeout: 5000 })).slice(0, 6000);
  }
  const lines = tree.split('\n');
  const clipped = lines.slice(0, 220).join('\n');
  console.log('# url: ' + url + (title ? '\n# title: ' + title : ''));
  console.log(clipped);
  if (lines.length > 220) console.log('# ... ' + (lines.length - 220) + ' more lines (use snapshot with a selector to narrow)');
}

const [cmd, ...rest] = process.argv.slice(2);

try {
  switch (cmd) {
    case 'start':
      await start();
      break;
    case 'stop':
      await withPage(async (page) => page.context().browser()?.close()).catch(() => {});
      console.log('stopped');
      break;
    case 'goto':
      await withPage(async (page) => {
        const res = await page.goto(rest[0], { waitUntil: 'domcontentloaded', timeout: 45000 });
        console.log('# status: ' + (res ? res.status() : '?'));
        // SPAs (Element, Nuxt, React) paint after domcontentloaded; wait until the body
        // actually has text so the first snapshot is not an empty tree.
        for (let i = 0; i < 25; i++) {
          const text = await page.evaluate(() => (document.body ? document.body.innerText.trim() : '')).catch(() => '');
          if (text.length > 0) break;
          await page.waitForTimeout(200);
        }
        await page.waitForTimeout(300);
        await snapshot(page);
      });
      break;
    case 'snapshot':
      await withPage(async (page) => {
        if (rest[0]) {
          console.log(await page.locator(rest[0]).ariaSnapshot({ timeout: 5000 }));
        } else {
          await snapshot(page);
        }
      });
      break;
    case 'click':
      await withPage(async (page) => {
        await page.locator(rest[0]).first().click({ timeout: 15000 });
        await page.waitForTimeout(600);
        await snapshot(page);
      });
      break;
    case 'fill':
      await withPage(async (page) => {
        await page.locator(rest[0]).first().fill(rest[1] ?? '', { timeout: 15000 });
        console.log('filled ' + rest[0]);
      });
      break;
    case 'type':
      await withPage(async (page) => {
        await page.keyboard.type(rest.join(' '));
        console.log('typed');
      });
      break;
    case 'press':
      await withPage(async (page) => {
        await page.keyboard.press(rest[0]);
        await page.waitForTimeout(600);
        await snapshot(page);
      });
      break;
    case 'eval':
      await withPage(async (page) => {
        const out = await page.evaluate(rest.join(' '));
        console.log(typeof out === 'string' ? out : JSON.stringify(out, null, 1));
      });
      break;
    case 'screenshot':
      await withPage(async (page) => {
        await page.screenshot({ path: rest[0] || '/tmp/page.png', fullPage: false });
        console.log('saved ' + (rest[0] || '/tmp/page.png'));
      });
      break;
    case 'url':
      await withPage(async (page) => console.log(page.url()));
      break;
    default:
      console.log('usage: cdp.mjs start|stop|goto <url>|snapshot [sel]|click <sel>|fill <sel> <text>|type <text>|press <key>|eval <js>|screenshot [path]|url');
      process.exit(cmd ? 2 : 0);
  }
} catch (e) {
  console.error('ERROR: ' + (e && e.message ? e.message : String(e)));
  process.exit(1);
}
