// Functional browser checks against the read-only preview; no fabricated API data.
import assert from 'node:assert/strict';
import { createRequire } from 'node:module';
import { existsSync } from 'node:fs';
const require = createRequire(import.meta.url);
const puppeteer = require(process.env.HALLMARK_PUPPETEER || 'puppeteer-core');
const executablePath = process.env.WAZI_BROWSER_EXECUTABLE || 'C:/Program Files/Google/Chrome/Application/chrome.exe';
assert(existsSync(executablePath), 'Set WAZI_BROWSER_EXECUTABLE to a local Chromium browser');
const base = process.env.WAZI_COMMUNITY_PREVIEW || 'http://127.0.0.1:3690';
const browser = await puppeteer.launch({ executablePath, headless: true, args: ['--no-first-run'] });
try {
  const page = await browser.newPage();
  const errors = [];
  page.on('pageerror', (error) => errors.push(error.message));
  await page.goto(base + '/apps', { waitUntil: 'networkidle0' });
  await page.waitForSelector('.app button');
  assert.equal(await page.$$eval('.app', (rows) => rows.length), 10);
  assert.equal(await page.$eval('#library-nav', (el) => el.getAttribute('aria-current')), 'page');
  await page.type('#search', 'Nginx');
  assert.equal(await page.$$eval('.app', (rows) => rows.length), 1);
  await page.click('.app button');
  assert.equal(await page.evaluate(() => document.activeElement.id), 'detail-title');
  assert(await page.$eval('#detail-body', (el) => el.textContent.includes('Previously tested in Wazi')));
  await page.click('#close-detail');
  assert.equal(await page.evaluate(() => document.activeElement.dataset.application), 'org.nginx.nginx');
  await page.select('#environment', 'macos-desktop');
  assert(await page.$eval('.package-state', (el) => el.textContent.includes('No matching signed setup')));
  await page.select('#availability', 'available');
  assert.equal(await page.$$eval('.app', (rows) => rows.length), 0);
  await page.select('#availability', 'needed');
  assert.equal(await page.$$eval('.app', (rows) => rows.length), 1);
  const starter = await page.evaluate(async () => (await fetch('/api/starter?application=org.nginx.nginx')).json());
  assert.equal(starter.executionAllowed, false);
  assert.equal(starter.state, 'discovery-starter');
  await page.goto(base + '/dashboard?tab=contributions', { waitUntil: 'networkidle0' });
  assert.equal(await page.$eval('#dashboard-nav', (el) => el.getAttribute('aria-current')), 'page');
  assert(await page.$eval('#workspace-status', (el) => el.textContent.includes('Sign in')));
  for (const width of [320, 375, 768, 1440, 1920]) {
    await page.setViewport({ width, height: 900 });
    await page.goto(base + '/apps', { waitUntil: 'networkidle0' });
    assert(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth), 'horizontal overflow at ' + width);
  }
  assert.deepEqual(errors, []);
  console.log('Community browser checks passed: search, evidence, focus return, compatibility filters, inert starter, workspace and five widths.');
} finally {
  await browser.close();
}
