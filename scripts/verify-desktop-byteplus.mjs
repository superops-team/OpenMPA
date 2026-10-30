import assert from 'node:assert/strict';
import { mkdir, writeFile } from 'node:fs/promises';
import { resolve } from 'node:path';
import { chromium } from 'playwright';
import { installDesktopFixture } from './desktop-design-fixture.mjs';

const origin = process.argv[2] || 'http://127.0.0.1:4186';
assert.equal(new URL(origin).hostname, '127.0.0.1');
const output = resolve(
  '.openmpa/byteplus-verification',
  new Date().toISOString().replaceAll(':', '-'),
);
await mkdir(output, { recursive: true });
const browser = await chromium.launch({ channel: 'chrome', headless: true });
const page = await browser.newPage({ viewport: { width: 1280, height: 1000 } });
await page.addInitScript(installDesktopFixture);
await page.addInitScript(() => {
  const ok = (value) => ({ ok: true, value });
  let platform = { signedIn: false, epoch: 1 };
  Object.assign(globalThis.__designOverrides, {
    'Platform.Status': () => ok(platform),
    'Platform.VaultStatus': () => ok({ exists: false }),
    'Connection.Status': () => ok({ connected: false, connectionEpoch: 1 }),
    'Platform.Regions': () =>
      ok({
        regions: [
          {
            provider: 'volcengine',
            region: 'cn-beijing',
            displayName: 'Beijing',
            endpointHost: 'agentkit.cn-beijing.volcengineapi.com',
          },
          {
            provider: 'volcengine',
            region: 'cn-shanghai',
            displayName: 'Shanghai',
            endpointHost: 'agentkit.cn-shanghai.volcengineapi.com',
          },
          {
            provider: 'byteplus',
            region: 'ap-southeast-1',
            displayName: 'Singapore',
            endpointHost: 'agentkit.ap-southeast-1.byteplusapi.com',
          },
        ],
      }),
    'Platform.SignIn': (input) => {
      platform = {
        signedIn: true,
        provider: input.provider,
        region: input.region,
        epoch: platform.epoch + 1,
        principal: {
          accountId: 'fixture-account',
          scope: `${input.provider}:fixture/User/fixture`,
        },
      };
      return ok(platform);
    },
    'Platform.ListRuntimes': () => ok({ complete: true, runtimes: [] }),
    'MPALifecycle.ListCreations': () => ok({ tasks: [] }),
    'MPALifecycle.ListDeletions': () => ok({ tasks: [] }),
    'Platform.SignOut': () => {
      platform = { signedIn: false, epoch: platform.epoch + 1 };
      return ok({});
    },
  });
});
try {
  await page.goto(origin);
  await page.waitForFunction(() => globalThis.__designShell);
  await page.evaluate(() =>
    globalThis.__designNavigate('selectSettingsSection', 'Account'),
  );
  const platform = page.getByLabel('Cloud platform', { exact: true });
  await platform.waitFor();
  const region = page.getByLabel('Region', { exact: true });
  assert.deepEqual(
    await region
      .locator('option')
      .evaluateAll((items) => items.map((i) => i.value)),
    ['cn-beijing', 'cn-shanghai'],
  );
  await page.getByLabel('Access key', { exact: true }).fill('domestic-fixture');
  await platform.selectOption('byteplus');
  assert.equal(
    await page.getByLabel('Access key', { exact: true }).inputValue(),
    '',
  );
  assert.deepEqual(
    await region
      .locator('option')
      .evaluateAll((items) => items.map((i) => i.value)),
    ['ap-southeast-1'],
  );
  await page.screenshot({
    path: resolve(output, 'byteplus-login-en.png'),
    fullPage: true,
  });
  await page.getByLabel('Access key', { exact: true }).fill('bp-fixture-ak');
  await page.getByLabel('Secret key', { exact: true }).fill('bp-fixture-sk');
  await page.getByRole('button', { name: 'Sign in', exact: true }).click();
  await page
    .getByRole('heading', { name: 'BytePlus account', exact: true })
    .waitFor();
  const request = await page.evaluate(
    () =>
      globalThis.__designRequests.find((r) => r.method === 'Platform.SignIn')
        .arg,
  );
  assert.equal(request.provider, 'byteplus');
  assert.equal(request.region, 'ap-southeast-1');
  await page.evaluate(() =>
    globalThis.__designNavigate('selectSettingsSection', 'Runtime'),
  );
  await region.waitFor();
  assert.deepEqual(
    await region
      .locator('option')
      .evaluateAll((items) => items.map((i) => i.value)),
    ['ap-southeast-1'],
  );
  await page.screenshot({
    path: resolve(output, 'byteplus-runtime-en.png'),
    fullPage: true,
  });
  await page.evaluate(() =>
    globalThis.__designNavigate('selectSettingsSection', 'Account'),
  );
  await page.getByRole('button', { name: 'Sign out', exact: true }).click();
  await platform.waitFor();
  await platform.selectOption('volcengine');
  assert.deepEqual(
    await region
      .locator('option')
      .evaluateAll((items) => items.map((i) => i.value)),
    ['cn-beijing', 'cn-shanghai'],
  );
  await page.evaluate(() => globalThis.__designLocale('zh-Hans'));
  await page.setViewportSize({ width: 760, height: 1000 });
  await page.getByLabel('云平台', { exact: true }).selectOption('byteplus');
  await page.screenshot({
    path: resolve(output, 'byteplus-login-zh.png'),
    fullPage: true,
  });
  assert.equal(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth,
    ),
    true,
  );
  const report = {
    success: true,
    checks: [
      'domestic default',
      'provider-specific regions',
      'credential clearing',
      'BP sign-in payload',
      'BP runtime region',
      'return to domestic',
      'Chinese narrow layout',
    ],
  };
  await writeFile(
    resolve(output, 'report.json'),
    JSON.stringify(report, null, 2),
  );
  console.log(JSON.stringify({ ...report, output }, null, 2));
} catch (error) {
  await page.screenshot({
    path: resolve(output, 'failure.png'),
    fullPage: true,
  });
  console.error(await page.locator('body').innerText());
  throw error;
} finally {
  await browser.close();
}
