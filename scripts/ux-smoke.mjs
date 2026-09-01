#!/usr/bin/env node
/**
 * Kryton UX smoke (Playwright).
 *
 * Usage:
 *   npm i playwright
 *   npx playwright install chromium
 *   KRYTON_URL=http://127.0.0.1:9088 KRYTON_DOCKUR_URL=http://127.0.0.1:7088 node scripts/ux-smoke.mjs
 *
 * Optional screenshots dir: KRYTON_SMOKE_SHOTS=/tmp/kryton-smoke
 */
import { chromium } from 'playwright';
import { mkdirSync } from 'node:fs';
import { join } from 'node:path';

const kubevirtURL = process.env.KRYTON_URL || 'http://127.0.0.1:9088';
const dockurURL = process.env.KRYTON_DOCKUR_URL || 'http://127.0.0.1:7088';
const shots = process.env.KRYTON_SMOKE_SHOTS || '/tmp/kryton-smoke';
mkdirSync(shots, { recursive: true });

const browser = await chromium.launch({ headless: true });
const page = await browser.newPage();
const failures = [];
page.on('pageerror', (e) => failures.push(String(e)));

await page.goto(kubevirtURL + '/', { waitUntil: 'networkidle', timeout: 60000 });
await page.waitForTimeout(1200);

const provider = await page.locator('#providerSide').innerText().catch(() => '');
await page.locator('[data-create]').first().click();
await page.waitForTimeout(700);
const hint = await page.locator('#createImageHint').innerText().catch(() => '');
const submitDisabled = await page.locator('#createSubmit').isDisabled();
await page.screenshot({ path: join(shots, 'kv-create.png'), fullPage: true });
await page.locator('[data-close-modal]').first().click().catch(() => {});

const dockur = await chromium.launch({ headless: true });
const dpage = await dockur.newPage();
await dpage.goto(dockurURL + '/', { waitUntil: 'networkidle', timeout: 60000 });
await dpage.waitForTimeout(1500);
const machineCount = await dpage.locator('.machine-card, .mini-machine, [data-machine]').count().catch(() => 0);
const overview = await dpage.locator('#heroTitle').innerText().catch(() => '');
await dpage.screenshot({ path: join(shots, 'dockur-overview.png'), fullPage: true });

const open = dpage.locator('button:has-text("Open"), .machine-row, .mini-machine').first();
if (await open.count()) {
  await open.click().catch(() => {});
  await dpage.waitForTimeout(1000);
  await dpage.screenshot({ path: join(shots, 'dockur-detail.png'), fullPage: true });
}

const result = {
  kubevirt: { url: kubevirtURL, provider, hint, submitDisabled, failures },
  dockur: { url: dockurURL, overview, machineCount },
  shots,
};
console.log(JSON.stringify(result, null, 2));

await browser.close();
await dockur.close();

if (failures.length) process.exit(1);
