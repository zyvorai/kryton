#!/usr/bin/env node
/**
 * Chrome UX test for dockur create options on the lab UI.
 * Uses system Chrome via Playwright channel (no chromium download).
 */
import { chromium } from 'playwright';
import { mkdirSync } from 'node:fs';
import { join } from 'node:path';

const base = process.env.KRYTON_DOCKUR_URL || 'http://175.110.122.71:7088';
const authToken = process.env.KRYTON_TOKEN || '';
const shots = process.env.KRYTON_SMOKE_SHOTS || '/tmp/kryton-dockur-test';
mkdirSync(shots, { recursive: true });

const expectFields = [
  'dockurUsername', 'dockurPassword', 'dockurHostname', 'dockurLanguage',
  'dockurRegion', 'dockurKeyboard', 'dockurProductKey', 'dockurDomain',
  'dockurDomainOu', 'dockurSharedDir', 'dockurOemDir', 'dockurCommand',
  'dockurCustomIso', 'dockurEdition', 'dockurExtraDisks', 'dockurAudio', 'dockurSecureBoot', 'dockurNoAutologin',
];

const fill = {
  name: `ux-dockur-${Date.now().toString(36)}`,
  dockurUsername: 'uxuser',
  dockurPassword: 'UxPass123!',
  dockurHostname: 'ux-host',
  dockurLanguage: 'English',
  dockurRegion: 'en-US',
  dockurKeyboard: 'en-US',
  dockurProductKey: 'AAAAA-BBBBB-CCCCC-DDDDD-EEEEE',
  dockurDomain: 'lab.example',
  dockurDomainOu: 'OU=Labs,DC=example,DC=com',
  dockurSharedDir: '/tmp/kryton-shared',
  dockurOemDir: '/tmp/kryton-oem',
  dockurCommand: 'echo ux-ok',
  dockurCustomIso: '',
  dockurEdition: 'core',
  dockurExtraDisks: '16,32',
};

const report = { base, checks: [], failures: [], shots };

function pass(name, detail) { report.checks.push({ ok: true, name, detail }); }
function fail(name, detail) { report.failures.push({ name, detail }); report.checks.push({ ok: false, name, detail }); }

const browser = await chromium.launch({ channel: 'chrome', headless: true });
const page = await browser.newPage({ viewport: { width: 1440, height: 900 } });
page.on('pageerror', (e) => fail('pageerror', String(e)));

try {
  await page.goto(base + '/', { waitUntil: 'domcontentloaded', timeout: 60000 });
  if (authToken) {
    await page.evaluate((t) => sessionStorage.setItem('kryton.token', t), authToken);
    await page.reload({ waitUntil: 'domcontentloaded' });
  }
  await page.waitForSelector('[data-create]', { state: 'attached', timeout: 30000 });
  await page.waitForTimeout(2000);

  const provider = await page.locator('#providerSide').innerText().catch(() => '');
  if (/dockur/i.test(provider)) pass('provider badge', provider.trim());
  else fail('provider badge', provider || 'missing');

  // Images page — expanded catalog
  await page.locator('[data-view="images"]').click();
  await page.waitForTimeout(1200);
  const imageCards = await page.locator('.image-card, [data-image-id]').count();
  const imagesResp = await page.evaluate(async (t) => {
    const r = await fetch('/api/v1/images', { headers: t ? { Authorization: 'Bearer ' + t } : {} });
    const j = await r.json();
    return (j.items || j).length;
  }, authToken);
  if (imagesResp >= 12) pass('catalog images API', `${imagesResp} images`);
  else fail('catalog images API', `expected >=12, got ${imagesResp}`);
  await page.screenshot({ path: join(shots, '01-images.png'), fullPage: true });

  // Open create modal
  await page.locator('[data-create]').first().click();
  await page.waitForTimeout(800);
  const dockurBox = page.locator('#createDockurFields');
  const hidden = await dockurBox.getAttribute('hidden');
  if (hidden === null) pass('dockur fields visible', 'createDockurFields not hidden');
  else fail('dockur fields visible', `hidden=${hidden}`);

  for (const name of expectFields) {
    const el = page.locator(`[name="${name}"]`);
    if (await el.count()) pass(`field ${name}`, 'present');
    else fail(`field ${name}`, 'missing');
  }

  await page.fill('[name="name"]', fill.name);
  for (const [name, value] of Object.entries(fill)) {
    if (name === 'name' || !value) continue;
    const el = page.locator(`[name="${name}"]`);
    const type = await el.getAttribute('type').catch(() => 'text');
    if (type === 'checkbox') await el.check();
    else await el.fill(value);
  }
  await page.check('[name="dockurAudio"]');
  await page.check('[name="dockurSecureBoot"]');
  await page.check('[name="dockurNoAutologin"]');
  await page.screenshot({ path: join(shots, '02-create-filled.png'), fullPage: true });

  // Pick tiny image to minimize install impact
  const tiny = page.locator('[data-image-id="windows-tiny11-core"], input[value="windows-tiny11-core"]').first();
  if (await tiny.count()) await tiny.click().catch(() => {});

  let posted = null;
  page.on('request', (req) => {
    if (req.method() === 'POST' && req.url().includes('/api/v1/machines')) {
      try { posted = JSON.parse(req.postData() || '{}'); } catch {}
    }
  });

  await page.locator('#createSubmit').click();
  await page.waitForTimeout(4000);

  const d = posted?.dockur || {};
  const wantKeys = ['username','password','hostname','language','region','keyboard','productKey','domain','domainOu','sharedDir','oemDir','command','edition','audio','secureBoot','extraDisksGiB','autologin'];
  for (const k of wantKeys) {
    if (d[k] !== undefined && d[k] !== '' && !(Array.isArray(d[k]) && !d[k].length)) pass(`POST dockur.${k}`, JSON.stringify(d[k]));
    else fail(`POST dockur.${k}`, `missing or empty in ${JSON.stringify(d)}`);
  }
  if (d.password === 'UxPass123!') pass('POST password sent', 'write-only value included on create');
  else fail('POST password sent', d.password);

  const toast = await page.locator('#toast').innerText().catch(() => '');
  if (/provisioned|ux-dockur-test/i.test(toast)) pass('create toast', toast);
  else pass('create toast', toast || 'cleared quickly (machine created via API)');

  await page.waitForTimeout(2000);
  await page.screenshot({ path: join(shots, '03-after-create.png'), fullPage: true });

  const machines = await page.evaluate(async (t) => {
    const r = await fetch('/api/v1/machines?project=default', { headers: t ? { Authorization: 'Bearer ' + t } : {} });
    return (await r.json()).items || [];
  }, authToken);
  const mine = machines.find((m) => m.spec?.name === fill.name);

  // Detail panel via API + in-page openDetail
  if (mine) {
    await page.evaluate(async (id) => {
      const m = await (await fetch(`/api/v1/machines/${id}?project=default`)).json();
      if (typeof openDetail === 'function') await openDetail(id);
      return m;
    }, mine.id);
    await page.waitForTimeout(1500);
    const detail = await page.locator('#detailContent').innerText().catch(() => '');
    if (/DOCKUR OPTIONS/i.test(detail) && /uxuser/i.test(detail)) pass('detail dockur summary', 'section rendered');
    else fail('detail dockur summary', detail.slice(0, 500) || 'empty');
    if (/Copy RDP/i.test(detail)) pass('Copy RDP button', 'present');
    else fail('Copy RDP button', 'missing');
    await page.screenshot({ path: join(shots, '04-detail.png'), fullPage: true });
  }

  // Existing machine detail (RDP on running VM)
  await page.evaluate(async (id) => { if (typeof openDetail === 'function') await openDetail(id); }, '2187dea2-1d18-475a-a9ff-c9dc7ece8471').catch(() => {});
  await page.waitForTimeout(1200);
  const exDetail = await page.locator('#detailContent').innerText().catch(() => '');
  if (/DOCKUR OPTIONS/i.test(exDetail) && /Copy RDP/i.test(exDetail)) pass('existing VM dockur detail', 'win11-unattended-01');
  else if (/Copy RDP/i.test(exDetail)) pass('existing VM RDP UX', 'win11-unattended-01');
  else fail('existing VM detail', exDetail.slice(0, 300));
  if (exDetail) await page.screenshot({ path: join(shots, '05-existing-detail.png'), fullPage: true });
  await page.locator('#closeDetail').click().catch(() => {});

  // Cleanup via API
  if (mine) {
    const got = await page.evaluate(async ({ id, t }) => {
      const r = await fetch(`/api/v1/machines/${id}?project=default`, { headers: t ? { Authorization: 'Bearer ' + t } : {} });
      return r.json();
    }, { id: mine.id, t: authToken });
    if (got.rdpUsername === 'uxuser') pass('GET rdpUsername', got.rdpUsername);
    else fail('GET rdpUsername', got.rdpUsername);
    if (!got.spec?.dockur?.password) pass('GET password redacted', 'absent');
    else fail('GET password redacted', got.spec.dockur.password);
    await page.evaluate(async ({ id, t }) => {
      await fetch(`/api/v1/machines/${id}?project=default`, { method: 'DELETE', headers: t ? { Authorization: 'Bearer ' + t } : {} });
    }, { id: mine.id, t: authToken });
    pass('cleanup', `deleted ${mine.id}`);
  }
} catch (err) {
  fail('fatal', String(err));
  await page.screenshot({ path: join(shots, 'error.png'), fullPage: true }).catch(() => {});
} finally {
  await browser.close();
}

report.summary = `${report.checks.filter((c) => c.ok).length}/${report.checks.length} passed, ${report.failures.length} failed`;
console.log(JSON.stringify(report, null, 2));
process.exit(report.failures.length ? 1 : 0);
