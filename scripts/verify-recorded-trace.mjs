import { chromium } from 'playwright';
import assert from 'node:assert/strict';
import { readFile, mkdir, writeFile } from 'node:fs/promises';
import { resolve } from 'node:path';
import { installDesktopFixture } from './desktop-design-fixture.mjs';

const origin = 'http://127.0.0.1:4174/desktop.html';
const trace = JSON.parse(
  await readFile('.openmpa/live-worker-archive/merged-trace.json', 'utf8'),
);
const output = resolve('.openmpa/live-worker-archive/visual');
await mkdir(output, { recursive: true });
const browser = await chromium.launch({ headless: true, channel: 'chrome' });
const context = await browser.newContext({
  viewport: { width: 1280, height: 900 },
  reducedMotion: 'reduce',
});
await context.addInitScript(installDesktopFixture);
await context.addInitScript((value) => {
  // Keep the fixture's routing ids; all execution steps are real captured data.
  globalThis.__recordedTrace = {
    ...value,
    sessionId: 'session-design',
    invocationId: 'turn-design',
  };
}, trace);
const page = await context.newPage();
const errors = [];
page.on('pageerror', (error) => errors.push(error.message));
try {
  await page.goto(origin);
  await page.getByText('Review complete', { exact: true }).waitFor();
  await page.evaluate(() => {
    globalThis.__designOverrides['Trace.Read'] = () => ({
      ok: true,
      value: globalThis.__recordedTrace,
    });
    globalThis.__designNavigate('trace');
  });
  await page.getByRole('dialog', { name: 'Turn trace' }).waitFor();
  await page
    .getByRole('button', { name: 'View worker steps', exact: true })
    .waitFor();
  const command = page
    .locator('.trace-row')
    .filter({ hasText: 'openmpa-archive-check' });
  assert.equal(
    await command.count(),
    1,
    'Recorded command must appear once with its actual command text',
  );
  await command.click();
  await page
    .locator('.trace-detail')
    .getByText('Recorded locally', { exact: true })
    .waitFor();
  assert.ok(
    (await page.locator('.trace-detail').textContent()).includes('809ms'),
  );
  const results = [];
  for (const theme of ['light', 'dark']) {
    await page.evaluate(
      (theme) => globalThis.__designNavigate('themeChanged', theme),
      theme,
    );
    for (const width of [1280, 768, 320]) {
      await page.setViewportSize({ width, height: 900 });
      await page.evaluate(() => new Promise((resolve) => requestAnimationFrame(() => requestAnimationFrame(resolve))));
      const layout = await page.evaluate(() => {
        const toolbar = document
          .querySelector('.trace-toolbar')
          .getBoundingClientRect();
        const ledger = document
          .querySelector('.trace-ledger')
          .getBoundingClientRect();
        return {
          width: document.documentElement.scrollWidth,
          toolbarBottom: toolbar.bottom,
          ledgerHeight: ledger.height,
          workerCount: document.querySelector('.trace-worker strong')
            .textContent,
          dialogs: document.querySelectorAll('dialog.trace-panel').length,
          headerCount: document.querySelectorAll('.trace-panel__header').length,
        };
      });
      await page.screenshot({ path: resolve(output, `${theme}-${width}.png`) });
      assert.ok(layout.width <= width, 'Recorded trace must fit the viewport');
      assert.equal(layout.dialogs, 1);
      assert.equal(layout.headerCount, 1);
      assert.ok(
        layout.toolbarBottom < 900 && layout.ledgerHeight >= 120,
        `Worker overview must leave the trace controls and ledger usable: ${JSON.stringify(layout)}`,
      );
      results.push({ theme, viewport: width, ...layout });
    }
  }
  await page.setViewportSize({ width: 1280, height: 900 });
  await page
    .locator('.trace-row')
    .filter({ hasText: /^Reasoning/ })
    .first()
    .click();
  assert.ok(
    (await page.locator('.trace-recorded-text').textContent()).length > 10,
  );
  assert.deepEqual(errors, []);
  const cdp = await page.context().newCDPSession(page);
  const screenshot = await cdp.send('Page.captureScreenshot', { format: 'png', fromSurface: true, captureBeyondViewport: false });
  await writeFile(resolve(output, 'dark-current-frame.png'), Buffer.from(screenshot.data, 'base64'));
  await writeFile(
    resolve(output, 'result.json'),
    JSON.stringify(
      {
        source:
          'real temporary Runtime execution, ids replaced only for fixture routing',
        results,
        errors,
      },
      null,
      2,
    ),
  );
  console.log(JSON.stringify({ pass: true, results, output }, null, 2));
} finally {
  await browser.close();
}
