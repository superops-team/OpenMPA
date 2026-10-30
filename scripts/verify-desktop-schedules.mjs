import assert from 'node:assert/strict';
import { mkdir, writeFile } from 'node:fs/promises';
import { resolve } from 'node:path';
import { chromium, webkit } from 'playwright';
import { installDesktopFixture } from './desktop-design-fixture.mjs';
import { installScheduleFixture } from './desktop-schedule-fixture.mjs';

const origin = process.argv[2] || 'http://127.0.0.1:4185';
const engine = process.argv[3] || 'chromium';
assert.ok(['chromium', 'webkit'].includes(engine));
assert.equal(new URL(origin).hostname, '127.0.0.1');
const output = resolve(
  '.openmpa/schedule-verification',
  new Date().toISOString().replaceAll(':', '-'),
);
await mkdir(output, { recursive: true });
const browser = await (engine === 'webkit'
  ? webkit.launch({ headless: true })
  : chromium.launch({ channel: 'chrome', headless: true }));
const context = await browser.newContext({
  viewport: { width: 1400, height: 950 },
  reducedMotion: 'reduce',
  timezoneId: 'Asia/Shanghai',
});
await context.addInitScript(installDesktopFixture);
await context.addInitScript(installScheduleFixture);
const page = await context.newPage();
page.setDefaultTimeout(10000);
const errors = [],
  checks = [];
page.on('pageerror', (error) => errors.push(error.message));
async function menu(name, item) {
  await page
    .getByRole('button', { name: `Actions · ${name}`, exact: true })
    .click();
  await page.getByRole('menuitem', { name: item, exact: true }).click();
}
async function checkMenu(name, screenshot, actionLabel = 'Actions') {
  const trigger = page.getByRole('button', {
    name: `${actionLabel} · ${name}`,
    exact: true,
  });
  await trigger.click();
  const menu = page.getByRole('menu');
  await menu.waitFor();
  await page.waitForFunction(() => {
    const menu = document.querySelector('.schedule-menu:popover-open');
    return menu && Number.parseFloat(menu.style.top) >= 8;
  });
  assert.equal(await trigger.getAttribute('aria-expanded'), 'true');
  const bounds = await menu.boundingBox();
  const viewport = page.viewportSize();
  assert.ok(bounds.height < 240, `Menu stretched to ${bounds.height}px`);
  assert.ok(bounds.y >= 8 && bounds.y + bounds.height <= viewport.height - 7);
  assert.ok(bounds.x >= 8 && bounds.x + bounds.width <= viewport.width - 7);
  const items = menu.getByRole('menuitem');
  assert.equal(await items.count(), 4);
  assert.notEqual(
    await items.last().evaluate((el) => getComputedStyle(el).color),
    await items.nth(1).evaluate((el) => getComputedStyle(el).color),
    'Delete action should be visually distinct',
  );
  let previousBottom = bounds.y;
  for (const item of await items.all()) {
    const rect = await item.boundingBox();
    assert.ok(rect.y >= previousBottom);
    assert.ok(rect.y - previousBottom < 16, 'Excessive menu item spacing');
    assert.ok(rect.y + rect.height <= bounds.y + bounds.height);
    previousBottom = rect.y + rect.height;
  }
  await page.keyboard.press('End');
  assert.equal(
    await items.last().evaluate((el) => el === document.activeElement),
    true,
  );
  await page.screenshot({ path: resolve(output, screenshot), fullPage: true });
  await page.keyboard.press('Escape');
  await menu.waitFor({ state: 'hidden' });
  await page.waitForFunction(
    (el) => el.getAttribute('aria-expanded') === 'false',
    await trigger.elementHandle(),
  );
  assert.equal(await trigger.getAttribute('aria-expanded'), 'false');
}
try {
  await page.goto(origin);
  await page.waitForFunction(
    () => globalThis.__designShell?.selectedAssistant === 'runtime-design',
  );
  await page.evaluate(() => globalThis.__designNavigate('tasks'));
  await page
    .getByRole('heading', { name: 'Scheduled Tasks', exact: true })
    .waitFor()
    .catch(() =>
      page
        .getByRole('heading', { name: 'Scheduled tasks', exact: true })
        .waitFor(),
    );
  await page
    .getByRole('button', { name: 'Daily project summary', exact: true })
    .waitFor();
  assert.equal(await page.locator('.task-row').count(), 10);
  await page.getByText('Executions: 45', { exact: true }).waitFor();
  await page.getByText('Success rate: 80%', { exact: true }).waitFor();
  await page.screenshot({
    path: resolve(output, 'list-en.png'),
    fullPage: true,
  });
  await checkMenu('Daily project summary', 'menu-en.png');
  await checkMenu('Task 09', 'menu-bottom-edge-en.png');
  checks.push(
    'Compact menu keeps all four actions visible, including delete, at viewport edges',
  );
  await page.getByRole('button', { name: 'Next', exact: true }).click();
  assert.equal(await page.locator('.task-row').count(), 3);
  await page.getByRole('button', { name: 'Previous', exact: true }).click();
  checks.push('Existing tasks, run statistics and client pagination');
  await page.getByRole('button', { name: 'Calendar', exact: true }).click();
  assert.equal(await page.locator('.schedule-calendar-day').count(), 42);
  await page.screenshot({
    path: resolve(output, 'calendar-en.png'),
    fullPage: true,
  });
  await page.getByRole('button', { name: 'List', exact: true }).click();
  checks.push('Calendar projects Runtime plans into device time zone');
  await page
    .getByRole('button', { name: 'New scheduled task', exact: true })
    .click();
  const dialog = page.getByRole('dialog');
  await dialog.getByLabel('Name', { exact: true }).fill('Created from desktop');
  await dialog
    .getByLabel('Prompt', { exact: true })
    .fill('Generate a project summary');
  await dialog
    .getByLabel('Schedule type', { exact: true })
    .selectOption('Weekly');
  await dialog.getByLabel('Sat', { exact: true }).check();
  await page.screenshot({
    path: resolve(output, 'create-en.png'),
    fullPage: true,
  });
  await dialog
    .getByRole('button', { name: 'Save schedule', exact: true })
    .click();
  await dialog.waitFor({ state: 'hidden' });
  await page
    .getByRole('button', { name: 'Created from desktop', exact: true })
    .waitFor();
  let request = await page.evaluate(
    () =>
      globalThis.__designRequests
        .filter((entry) => entry.method === 'Schedule.Create')
        .at(-1).arg,
  );
  assert.equal(request.schedule.type, 'Weekly');
  assert.deepEqual(request.schedule.weekdays, [1, 2, 3, 4, 5, 6]);
  assert.equal(request.schedule.timezone, 'Asia/Shanghai');
  assert.ok(request.clientToken);
  checks.push(
    'Creates a structured weekly schedule through the native adapter',
  );
  await menu('Daily project summary', 'Edit');
  await dialog.getByLabel('Name', { exact: true }).fill('Renamed task');
  await dialog
    .getByRole('button', { name: 'Save schedule', exact: true })
    .click();
  await dialog.waitFor({ state: 'hidden' });
  request = await page.evaluate(
    () =>
      globalThis.__designRequests
        .filter((entry) => entry.method === 'Schedule.Update')
        .at(-1).arg,
  );
  assert.equal(request.expectedVersion, 1);
  assert.equal(request.jitterSeconds, 60);
  assert.equal(request.timeoutSeconds, 7200);
  checks.push('Editing retains hidden settings and uses expectedVersion');
  await page.getByRole('button', { name: 'Renamed task', exact: true }).click();
  await dialog.getByRole('button', { name: 'Run now', exact: true }).click();
  await dialog.getByText('run-1', { exact: true }).waitFor();
  await page.screenshot({
    path: resolve(output, 'history-en.png'),
    fullPage: true,
  });
  await page.keyboard.press('Escape');
  await page
    .getByRole('button', {
      name: 'Disable schedule · Renamed task',
      exact: true,
    })
    .click();
  await page
    .getByRole('button', {
      name: 'Enable schedule · Renamed task',
      exact: true,
    })
    .waitFor();
  checks.push('Immediate run receipt, history and task pause');
  await menu('Renamed task', 'Duplicate task');
  await dialog
    .getByRole('button', { name: 'Save schedule', exact: true })
    .click();
  await dialog.waitFor({ state: 'hidden' });
  await page
    .getByRole('button', { name: 'Renamed task (copy)', exact: true })
    .waitFor();
  checks.push('Copies a task into a new server task');
  await menu('Renamed task (copy)', 'Delete');
  await dialog.getByRole('button', { name: 'Delete', exact: true }).click();
  await dialog.waitFor({ state: 'hidden' });
  assert.equal(
    await page
      .getByRole('button', { name: 'Renamed task (copy)', exact: true })
      .count(),
    0,
  );
  checks.push('Explicit delete confirmation and list refresh');
  await page.evaluate(() => {
    globalThis.__scheduleFixture.failure = 'businessIdentityRequired';
  });
  await page.getByRole('button', { name: 'Refresh', exact: true }).click();
  await page
    .getByRole('alert')
    .filter({ hasText: 'Tasks are scoped to a user' })
    .waitFor();
  assert.equal(
    await page.getByText('No scheduled tasks yet', { exact: true }).count(),
    0,
  );
  await page.evaluate(() => {
    globalThis.__scheduleFixture.failure = '';
  });
  await page.getByRole('button', { name: 'Refresh', exact: true }).click();
  await page
    .getByRole('button', { name: 'Created from desktop', exact: true })
    .waitFor();
  checks.push(
    'Identity error is distinct from empty results and refresh recovers',
  );
  await page.evaluate(() => globalThis.__designLocale('zh-Hans'));
  await page.getByRole('heading', { name: '定时任务', exact: true }).waitFor();
  await page.setViewportSize({ width: 780, height: 900 });
  await checkMenu('Created from desktop', 'menu-zh-narrow.png', '操作');
  await page.screenshot({
    path: resolve(output, 'list-zh-narrow.png'),
    fullPage: true,
  });
  const overflow = await page.evaluate(
    () => document.documentElement.scrollWidth > window.innerWidth,
  );
  assert.equal(overflow, false);
  await page.getByRole('button', { name: '新建定时任务', exact: true }).click();
  await page.screenshot({
    path: resolve(output, 'create-zh-narrow.png'),
    fullPage: true,
  });
  await page.keyboard.press('Tab');
  assert.equal(
    await page
      .getByRole('dialog')
      .evaluate((node) => node.contains(document.activeElement)),
    true,
  );
  await page.keyboard.press('Escape');
  checks.push('Chinese localization, narrow layout and modal keyboard focus');
  await page.evaluate(() => globalThis.__scheduleFixture.tasks.splice(1));
  await page.getByRole('button', { name: '刷新', exact: true }).click();
  await page.waitForFunction(
    () => document.querySelectorAll('.task-row').length === 1,
  );
  await page.setViewportSize({ width: 1400, height: 850 });
  await checkMenu('Created from desktop', 'menu-zh-single-task.png', '操作');
  checks.push(
    'Single-task list keeps delete visible without stretching the menu',
  );
  assert.deepEqual(errors, []);
  await writeFile(
    resolve(output, 'report.json'),
    JSON.stringify({ engine, checks, errors, output }, null, 2),
  );
  console.log(JSON.stringify({ checks, output }, null, 2));
} catch (error) {
  await page.screenshot({
    path: resolve(output, 'failure.png'),
    fullPage: true,
  });
  await writeFile(
    resolve(output, 'failure.json'),
    JSON.stringify({ error: String(error), errors, checks }, null, 2),
  );
  throw error;
} finally {
  await browser.close();
}
