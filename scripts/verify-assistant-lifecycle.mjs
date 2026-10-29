import assert from 'node:assert/strict';
import { chromium } from 'playwright';
import { mkdir, writeFile } from 'node:fs/promises';
import { resolve } from 'node:path';
import { installDesktopFixture } from './desktop-design-fixture.mjs';

const origin = process.argv[2] || 'http://127.0.0.1:4173';
assert.equal(new URL(origin).hostname, '127.0.0.1');
const output = resolve('.openmpa/design-review-20261029/assistant-lifecycle');
await mkdir(output, { recursive: true });
const browser = await chromium.launch({ headless: true, channel: 'chrome' });
const context = await browser.newContext({
  viewport: { width: 1280, height: 900 },
});
await context.addInitScript(installDesktopFixture);
const page = await context.newPage();
const errors = [];
page.on('pageerror', (error) => errors.push(error.message));
const evidence = [];
const geometry = (locator) =>
  locator.evaluate((node) => {
    const { x, y, width, height } = node.getBoundingClientRect();
    return { x, y, width, height };
  });
const fieldStyle = (locator) =>
  locator.evaluate((node) => {
    const style = getComputedStyle(node);
    return {
      border: style.borderWidth,
      outline: style.outlineStyle,
      shadow: style.boxShadow,
    };
  });
try {
  await page.goto(origin);
  await page.getByText('Review complete', { exact: true }).waitFor();
  const initialReads = await page.evaluate(
    () =>
      globalThis.__designCalls.filter((name) => name === 'Session.Read').length,
  );
  const message = page.getByRole('textbox', { name: 'Message', exact: true });
  const processToggle = page.getByRole('button', {
    name: 'Expand execution process',
    exact: true,
  });
  await processToggle.click();
  const reasoning = page.locator(
    '.ui-status-row[data-description="wrap"] .ui-status-row-copy small',
  );
  assert.equal(
    await reasoning.textContent(),
    'Checking the complete execution history.\nCompare live steps with the final persisted result.',
  );
  assert.equal(
    await reasoning.evaluate((node) => getComputedStyle(node).whiteSpace),
    'pre-wrap',
  );
  await page
    .getByRole('button', { name: 'Collapse execution process', exact: true })
    .click();
  await message.fill('Keep this unsent draft');
  const composer = await page.locator('.chat-composer').evaluate((node) => {
    const style = getComputedStyle(node);
    const field = node.querySelector('.composer-field');
    const backplate = node.querySelector('.composer-context');
    const input = node.querySelector('textarea');
    return {
      background: style.backgroundColor,
      shadow: style.boxShadow,
      radius: style.borderRadius,
      inset:
        (field.getBoundingClientRect().width -
          backplate.getBoundingClientRect().width) /
        2,
      placeholder: getComputedStyle(input, '::placeholder').color,
    };
  });
  assert.equal(composer.background, 'rgba(0, 0, 0, 0)');
  assert.equal(composer.shadow, 'none');
  assert.equal(composer.radius, '0px');
  assert.equal(composer.inset, 16);
  assert.equal(composer.placeholder, 'rgb(99, 99, 99)');
  evidence.push({ composer });
  await page.screenshot({ path: resolve(output, 'composer-light.png') });

  for (const theme of ['light', 'dark']) {
    await page.evaluate(
      (theme) => globalThis.__designNavigate('themeChanged', theme),
      theme,
    );
    const frame = await page.locator('.chat-composer').evaluate((node) => {
      const style = getComputedStyle(node);
      return {
        background: style.backgroundColor,
        shadow: style.boxShadow,
        radius: style.borderRadius,
      };
    });
    assert.deepEqual(
      frame,
      { background: 'rgba(0, 0, 0, 0)', shadow: 'none', radius: '0px' },
      `${theme} composer must have no outer surface`,
    );
    await page
      .getByRole('button', { name: 'Assistant settings', exact: true })
      .click();
    const dialog = page.getByRole('dialog', {
      name: 'Assistant settings',
      exact: true,
    });
    await dialog.waitFor();
    await dialog.evaluate(async () => {
      await Promise.allSettled(
        document.getAnimations().map((animation) => animation.finished),
      );
    });
    await dialog.getByRole('textbox', { name: 'Assistant name' }).waitFor();
    assert.equal(await page.locator('body > .ui-modal-layer').count(), 1);
    assert.equal(
      await page.locator('#root').evaluate((node) => node.inert),
      true,
    );
    assert.equal(
      await dialog
        .getByRole('button', { name: 'Dismiss' })
        .evaluate((node) => node === document.activeElement),
      false,
    );
    const before = await geometry(dialog);
    await dialog.getByRole('button', { name: /^Model:/ }).click();
    const filter = dialog.getByRole('textbox', { name: 'Filter models' });
    await filter.fill('alternate');
    assert.deepEqual(await fieldStyle(filter), {
      border: '0px',
      outline: 'none',
      shadow: 'none',
    });
    assert.equal(
      await dialog
        .getByRole('radio', { name: 'design-model', exact: true })
        .count(),
      0,
    );
    assert.equal(
      await dialog
        .getByRole('radio', { name: 'design-alternate', exact: true })
        .count(),
      1,
    );
    assert.deepEqual(await geometry(dialog), before);
    await page.screenshot({
      path: resolve(output, `model-search-${theme}.png`),
    });
    await page.keyboard.press('Escape');
    assert.equal(
      await dialog.count(),
      1,
      'Escape in picker must keep settings open',
    );
    await dialog.getByRole('tab', { name: 'Skills', exact: true }).click();
    assert.deepEqual(
      await geometry(dialog),
      before,
      'Switching tabs must preserve dialog geometry',
    );
    await dialog.getByRole('tab', { name: 'AGENTS.md', exact: true }).click();
    await page.screenshot({ path: resolve(output, `settings-${theme}.png`) });
    if (theme === 'light') {
      await dialog.getByRole('button', { name: /^Model:/ }).click();
      await dialog
        .getByRole('radio', { name: 'design-alternate', exact: true })
        .click();
      await dialog
        .getByRole('button', { name: 'Save assistant settings', exact: true })
        .click();
    } else {
      await dialog.getByRole('button', { name: 'Dismiss' }).click();
    }
    await dialog.waitFor({ state: 'detached' });
    assert.equal(await message.inputValue(), 'Keep this unsent draft');
    await page
      .getByRole('button', { name: 'Model: design-alternate', exact: true })
      .waitFor();
    evidence.push({ theme, dialog: before });
  }
  assert.equal(
    await page.evaluate(
      () =>
        globalThis.__designCalls.filter((name) => name === 'Session.Read')
          .length,
    ),
    initialReads,
  );
  assert.equal(
    await page.evaluate(
      () =>
        globalThis.__designCalls.filter(
          (name) => name === 'Session.ModelCatalog',
        ).length,
    ),
    1,
  );

  // A fresh page holds metadata requests so slow network behavior is exercised.
  await page.reload();
  await page.getByText('Review complete', { exact: true }).waitFor();
  await page.evaluate(() => {
    globalThis.__designOverrides['Session.SkillCatalog'] = (arg, next) =>
      new Promise((resolve) => {
        globalThis.__releaseSkills = () =>
          resolve({ ok: true, value: next(arg) });
      });
    let failed = false;
    globalThis.__designOverrides['Session.SaveAssistantConfig'] = (
      arg,
      next,
    ) => {
      if (!failed) {
        failed = true;
        return {
          ok: false,
          error: {
            code: 'localSaveFailed',
            message: 'Local save failed. Please retry.',
          },
        };
      }
      return { ok: true, value: next(arg) };
    };
    globalThis.__designNavigate('newConversation');
  });
  const creating = page.getByRole('dialog', {
    name: 'New assistant',
    exact: true,
  });
  const name = creating.getByRole('textbox', { name: 'Assistant name' });
  await name.fill('A persistent draft');
  assert.equal(
    await name.evaluate((node) => document.activeElement === node),
    true,
  );
  const initial = await geometry(creating);
  await page.evaluate(() => globalThis.__releaseSkills());
  assert.equal(await name.inputValue(), 'A persistent draft');
  assert.deepEqual(await geometry(creating), initial);
  await page.evaluate(() => globalThis.__designLocale('zh-Hans'));
  const chinese = page.getByRole('dialog', { name: '新建助手', exact: true });
  await chinese
    .getByRole('button', { name: '创建助手', exact: true })
    .waitFor();
  assert.equal(
    await chinese.getByRole('button', { name: '取消', exact: true }).count(),
    1,
  );
  assert.equal(
    await chinese
      .getByRole('textbox', { name: '助手名称', exact: true })
      .inputValue(),
    'A persistent draft',
  );
  assert.deepEqual(await geometry(chinese), initial);
  await page.screenshot({ path: resolve(output, 'create-zh.png') });
  await page.evaluate(() => globalThis.__designLocale('en'));
  await creating.getByRole('button', { name: /^Model:/ }).click();
  const modelSearch = creating.getByRole('textbox', { name: 'Filter models' });
  await modelSearch.fill('alternate');
  assert.deepEqual(await geometry(creating), initial);
  await creating
    .getByRole('radio', { name: 'design-alternate', exact: true })
    .click();
  await creating
    .getByRole('button', { name: 'Model: design-alternate', exact: true })
    .waitFor();
  await creating
    .getByRole('button', { name: 'Create assistant', exact: true })
    .click();
  await creating.getByRole('alert').waitFor();
  assert.equal(await name.inputValue(), 'A persistent draft');
  await creating
    .getByRole('button', { name: 'Create assistant', exact: true })
    .click();
  await creating.waitFor({ state: 'detached' });
  const creates = await page.evaluate(() =>
    globalThis.__designRequests
      .filter(({ method }) => method === 'Session.Create')
      .map(({ arg }) => arg.sessionId),
  );
  assert.equal(creates.length, 2);
  assert.equal(creates[0], creates[1]);
  const modelChanges = await page.evaluate(() =>
    globalThis.__designRequests.filter(
      ({ method }) => method === 'Session.UpdateExecutionConfig',
    ),
  );
  assert.equal(modelChanges.at(-1).arg.model, 'design-alternate');
  await page
    .getByRole('button', { name: 'Model: design-alternate', exact: true })
    .waitFor();
  assert.equal(
    await page.evaluate(() => globalThis.__designShell.sessions.length),
    2,
  );
  assert.deepEqual(errors, []);
  assert.deepEqual(await page.evaluate(() => globalThis.__designMissing), []);
  await writeFile(
    resolve(output, 'result.json'),
    JSON.stringify(
      { pass: true, evidence, sameCreationIdOnRetry: true, errors },
      null,
      2,
    ),
  );
  console.log(
    `Assistant lifecycle and rendered-style checks passed: ${output}`,
  );
} finally {
  await browser.close();
}
