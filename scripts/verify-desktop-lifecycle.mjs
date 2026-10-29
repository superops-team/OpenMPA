import assert from 'node:assert/strict';
import { chromium } from 'playwright';
import { mkdir, writeFile } from 'node:fs/promises';
import { resolve } from 'node:path';
import { installDesktopFixture } from './desktop-design-fixture.mjs';
import { installLifecycleFixture } from './desktop-lifecycle-fixture.mjs';

const origin = process.argv[2] || 'http://127.0.0.1:4184';
assert.equal(new URL(origin).hostname, '127.0.0.1');
const output = resolve(
  '.openmpa/lifecycle-verification',
  new Date().toISOString().replaceAll(':', '-'),
);
await mkdir(output, { recursive: true });
const browser = await chromium.launch({ channel: 'chrome', headless: true });
const context = await browser.newContext({
  viewport: { width: 1280, height: 900 },
  reducedMotion: 'reduce',
});
await context.addInitScript(installDesktopFixture);
await context.addInitScript(installLifecycleFixture);
const page = await context.newPage();
page.setDefaultTimeout(10000);
const errors = [];
const checks = [];
page.on('pageerror', (error) => errors.push(error.message));
const button = (name) => page.getByRole('button', { name, exact: true });
const click = (name) => button(name).click();
async function widthCheck() {
  const result = await page.evaluate(() => ({
    page: document.documentElement.scrollWidth,
    viewport: innerWidth,
    dialog: document
      .querySelector('[role="dialog"]')
      ?.getBoundingClientRect()
      .toJSON(),
  }));
  assert.ok(result.page <= result.viewport, JSON.stringify(result));
  if (result.dialog)
    assert.ok(
      result.dialog.right <= result.viewport && result.dialog.left >= 0,
      JSON.stringify(result),
    );
}
try {
  await page.goto(origin);
  await click('Chat fixture');
  const composer = page.getByRole('textbox', { name: 'Message', exact: true });
  await composer.fill('Keep this draft');
  await click('Manage fixture');
  await page
    .getByRole('heading', { name: 'Manage Runtimes', exact: true })
    .waitFor();
  await click('Create MPA');
  const nameInput = page.getByRole('textbox', { name: 'Name', exact: true });
  for (const [attribute, value] of Object.entries({
    autocapitalize: 'none',
    autocorrect: 'off',
    autocomplete: 'off',
    spellcheck: 'false',
    writingsuggestions: 'false',
  }))
    assert.equal(await nameInput.getAttribute(attribute), value);
  assert.equal(
    await nameInput.evaluate((node) => getComputedStyle(node).textTransform),
    'none',
  );
  checks.push(
    'MPA names disable capitalization, correction, spelling and writing suggestions without changing typed case',
  );
  await page.getByRole('dialog').waitFor();
  await page
    .getByRole('textbox', { name: 'Name', exact: true })
    .fill('fixture-mpa');
  await page
    .getByRole('textbox', { name: 'Description', exact: true })
    .fill('A test MPA with independent resources');
  for (const theme of ['dark', 'light']) {
    await page.evaluate((theme) => {
      document.documentElement.dataset.theme = theme;
    }, theme);
    for (const width of [1280, 768, 390]) {
      await page.setViewportSize({ width, height: 900 });
      await widthCheck();
    }
  }
  await page.setViewportSize({ width: 1280, height: 900 });
  await widthCheck();
  await page.screenshot({ path: resolve(output, 'creation-light.png') });
  await click('Next');
  await page
    .getByText('Automatically create or reuse PostgreSQL Workspaces', {
      exact: false,
    })
    .waitFor();
  await click('Next');
  const bucket = page.getByRole('combobox', {
    name: 'TOS bucket',
    exact: true,
  });
  await page
    .getByText('Service region: cn-beijing · Loaded 2/2', { exact: true })
    .waitFor();
  assert.equal(await bucket.innerText(), 'Select an existing resource');
  await page.screenshot({ path: resolve(output, 'tos-storage-studio.png') });
  await bucket.focus();
  await page.keyboard.press('ArrowDown');
  await page
    .getByRole('searchbox', { name: 'Search buckets', exact: true })
    .fill('outputs');
  await page.keyboard.press('ArrowDown');
  await page.keyboard.press('Enter');
  assert.equal(await bucket.innerText(), 'agentkit-platform-fixture-outputs');
  await bucket.click();
  await click('Refresh buckets');
  assert.equal(await bucket.innerText(), 'agentkit-platform-fixture-outputs');
  await page.keyboard.press('Escape');
  assert.equal(await page.getByRole('dialog').isVisible(), true);
  await click('Clear bucket selection');
  assert.equal(await bucket.innerText(), 'Select an existing resource');
  await bucket.click();
  await page
    .getByRole('option', {
      name: 'agentkit-platform-fixture-outputs',
      exact: true,
    })
    .click();
  for (const theme of ['dark', 'light']) {
    await page.evaluate((theme) => {
      document.documentElement.dataset.theme = theme;
    }, theme);
    for (const width of [1280, 768, 390]) {
      await page.setViewportSize({ width, height: 900 });
      await widthCheck();
    }
  }
  await page.screenshot({ path: resolve(output, 'tos-picker-narrow.png') });
  await page.setViewportSize({ width: 1280, height: 900 });
  checks.push(
    'TOS buckets load automatically; search, refresh and clear preserve explicit selection',
  );
  assert.equal(
    await page.evaluate(
      () =>
        globalThis.__lifecycleRequests.filter(
          (row) => row.method === 'MPALifecycle.Create',
        ).length,
    ),
    0,
  );
  await page.evaluate(() => {
    globalThis.__holdCreation = true;
  });
  await click('Confirm creation');
  assert.equal(
    await page.evaluate(
      () =>
        globalThis.__lifecycleRequests.find(
          (row) => row.method === 'MPALifecycle.Create',
        ).request.input.tosBucket,
    ),
    'agentkit-platform-fixture-outputs',
  );
  const creationDialog = page.getByRole('dialog');
  const steps = creationDialog.locator('.mpa-creation-checklist');
  assert.equal(await steps.locator('li').count(), 11);
  assert.equal(
    await steps.locator('[data-stage="checking"]').getAttribute('data-state'),
    'completed',
  );
  assert.equal(
    await steps.locator('[data-stage="worker"]').getAttribute('data-state'),
    'running',
  );
  assert.equal(
    await steps.locator('[data-stage="verifying"]').getAttribute('data-state'),
    'pending',
  );
  for (const theme of ['dark', 'light']) {
    await page.evaluate((theme) => {
      document.documentElement.dataset.theme = theme;
    }, theme);
    for (const width of [1280, 768, 390]) {
      await page.setViewportSize({ width, height: 900 });
      await widthCheck();
    }
    await page.screenshot({
      path: resolve(output, `creation-steps-${theme}.png`),
    });
  }
  await page.setViewportSize({ width: 1280, height: 900 });
  await creationDialog.locator('details summary').click();
  await page.evaluate(() => {
    const task = globalThis.__lifecycleTasks[0];
    task.stage = 'openviking';
    task.visitedStages.push('openviking');
  });
  await click('Check status');
  const history = creationDialog.locator('details');
  assert.equal(
    await history.locator('summary').innerText(),
    'Diagnostic history',
  );
  assert.equal(await history.getAttribute('open'), null);
  await history.locator('summary').click();
  await creationDialog
    .getByText(
      "This record is from an earlier creation step. It does not describe the current step's state.",
      { exact: true },
    )
    .waitFor();
  assert.equal(
    await creationDialog.getByText('Waiting to retry', { exact: true }).count(),
    0,
  );
  await creationDialog.getByText('worker_agent_key', { exact: true }).waitFor();
  await page.screenshot({
    path: resolve(output, 'creation-running-history.png'),
  });
  checks.push(
    'Advancing past a step collapses its old retry diagnosis into history without implying it is still retrying',
  );
  await page.evaluate(() => {
    globalThis.__holdCreation = false;
  });
  await creationDialog.getByText('Succeeded', { exact: true }).waitFor();
  const completedStatus = creationDialog.getByRole('status');
  assert.equal(await completedStatus.innerText(), 'Succeeded');
  assert.equal(await steps.locator('[data-state="running"]').count(), 0);
  assert.equal(await steps.locator('[data-state="completed"]').count(), 11);
  assert.equal(await creationDialog.locator('details').count(), 0);
  assert.equal(
    await creationDialog.getByText('worker_agent_key', { exact: true }).count(),
    0,
  );
  await page.screenshot({
    path: resolve(output, 'creation-completed-clean.png'),
  });
  checks.push(
    'All steps retain their completed states after success, with no intermediate diagnostic or diagnostic-history entry',
  );
  checks.push(
    'Three-step creation submits once at final confirmation and reaches the durable task result',
  );
  await button('Close').last().click();
  await page.evaluate(() => {
    const previous = globalThis.__lifecycleTasks[0];
    globalThis.__lifecycleTasks.push({
      ...previous,
      taskId: crypto.randomUUID(),
      requestId: crypto.randomUUID(),
      name: 'failed-fixture',
      state: 'failed',
      stage: 'worker',
      visitedStages: previous.visitedStages.slice(0, 9),
      result: undefined,
      latestDiagnostic: {
        stage: 'worker',
        operation: 'get_worker',
        category: 'permission',
        attempt: 2,
        outcome: 'failed',
        errorCode: 'AccessDenied.Tool',
        requestId: '0123456789abcdef0123456789abcdef',
        exceptionType: 'HTTPError -> ApiError',
        httpStatus: 403,
        responseType: 'application/json',
      },
    });
  });
  await click('List current account');
  await page
    .locator('.mpa-history li')
    .filter({ hasText: 'failed-fixture' })
    .getByRole('button')
    .click();
  await creationDialog
    .getByRole('alert')
    .getByText(
      'The current credentials do not authorize this operation or are no longer valid.',
      { exact: true },
    )
    .waitFor();
  await creationDialog.getByText('Error details', { exact: true }).click();
  await creationDialog
    .getByText('HTTPError -> ApiError', { exact: true })
    .waitFor();
  assert.equal(
    await creationDialog
      .locator('[data-stage="worker"]')
      .getAttribute('data-state'),
    'failed',
  );
  await context.grantPermissions(['clipboard-read', 'clipboard-write'], {
    origin,
  });
  await click('Copy error information');
  await creationDialog.getByText('Copied', { exact: true }).waitFor();
  const copied = await page.evaluate(() => navigator.clipboard.readText());
  assert.ok(
    copied.includes('AccessDenied.Tool') && copied.includes('HTTP status: 403'),
  );
  await page.setViewportSize({ width: 390, height: 900 });
  await widthCheck();
  await page.screenshot({
    path: resolve(output, 'creation-error-details-narrow.png'),
  });
  await page.setViewportSize({ width: 1280, height: 900 });
  await page.screenshot({
    path: resolve(output, 'creation-error-details.png'),
  });
  await button('Close').last().click();
  checks.push(
    'Failure shows the Studio reason, stage, operation, exception chain, HTTP/response type and Request ID, with working copy',
  );
  await click('Chat fixture');
  assert.equal(await composer.inputValue(), 'Keep this draft');
  checks.push('Manager navigation preserves the conversation draft');
  await click('Manage fixture');
  const previewCountBefore = await page.evaluate(
    () =>
      globalThis.__lifecycleRequests.filter(
        (row) => row.method === 'MPALifecycle.PreviewDelete',
      ).length,
  );
  await page.evaluate(() => {
    globalThis.__deletePreviewFailure = true;
  });
  await click('Delete MPA');
  await page.getByRole('dialog').getByRole('alert').waitFor();
  assert.equal(
    await page.evaluate(
      () =>
        globalThis.__lifecycleRequests.filter(
          (row) => row.method === 'MPALifecycle.PreviewDelete',
        ).length,
    ),
    previewCountBefore + 1,
  );
  await page.getByText('Error details', { exact: true }).click();
  await page
    .getByText('Verify resource references and ownership', { exact: true })
    .waitFor();
  await page.getByText('list_runtimes', { exact: true }).waitFor();
  assert.equal(await button('Confirm deletion').count(), 0);
  await click('Copy error information');
  await page.getByText('Copied', { exact: true }).waitFor();
  await page.screenshot({
    path: resolve(output, 'deletion-preflight-error.png'),
  });
  await page.setViewportSize({ width: 390, height: 900 });
  await widthCheck();
  await page.screenshot({
    path: resolve(output, 'deletion-preflight-error-narrow.png'),
  });
  await page.setViewportSize({ width: 1280, height: 900 });
  await page.evaluate(() => {
    globalThis.__deletePreviewFailure = false;
  });
  await click('Review cleanup scope again');
  await page.getByText('t-worker-owned', { exact: true }).waitFor();
  assert.equal(await page.getByRole('dialog').getByRole('alert').count(), 0);
  assert.equal(
    await page.getByText('Error details', { exact: true }).count(),
    0,
  );
  await button('Close').last().click();
  checks.push(
    'Deletion preflight reports the real failed operation and safe details; confirmation stays absent until a successful recheck',
  );
  await click('Toggle active sessions');
  await click('Delete MPA');
  await page.getByText('Active sessions remain.', { exact: false }).waitFor();
  assert.equal(await button('Confirm deletion').isDisabled(), true);
  await button('Close').last().click();
  checks.push('Active sessions block confirmation without a delete call');
  await click('Toggle active sessions');
  await click('Delete MPA');
  await page.getByText('t-worker-owned', { exact: true }).waitFor();
  await page.getByText('ss-shared', { exact: true }).waitFor();
  await page.screenshot({ path: resolve(output, 'deletion-light.png') });
  for (const theme of ['dark', 'light']) {
    await page.evaluate((theme) => {
      document.documentElement.dataset.theme = theme;
    }, theme);
    for (const width of [1280, 768, 390]) {
      await page.setViewportSize({ width, height: 900 });
      await widthCheck();
    }
  }
  checks.push(
    'Creation and deletion dialogs render without horizontal overflow in light/dark themes and narrow windows',
  );
  await page.setViewportSize({ width: 1280, height: 900 });
  await button('Close').last().click();
  await click('Language fixture');
  await page.getByText('中文', { exact: true }).click();
  await click('Manage fixture');
  await click('删除 MPA');
  await page.getByRole('button', { name: '确认删除', exact: true }).waitFor();
  await page.screenshot({ path: resolve(output, 'deletion-zh.png') });
  await click('确认删除');
  await page.getByRole('dialog').getByText('已完成', { exact: true }).waitFor();
  assert.equal(
    await page.evaluate(
      () =>
        globalThis.__lifecycleRequests.filter(
          (row) => row.method === 'MPALifecycle.Delete',
        ).length,
    ),
    1,
  );
  assert.equal(
    await page.evaluate(
      () =>
        globalThis.__lifecycleRequests.filter(
          (row) => row.method === 'Platform.ResetRuntime',
        ).length,
    ),
    0,
  );
  checks.push(
    'Chinese deletion uses the reviewed fingerprint, submits once, and clears the deleted active connection',
  );
  await button('关闭').last().click();
  await page
    .getByText('当前账户、地域和项目下没有可见的 MPA Runtime。', {
      exact: true,
    })
    .waitFor();
  await page
    .locator('.mpa-history li')
    .filter({ hasText: 'fixture-mpa' })
    .getByRole('button')
    .click();
  await creationDialog
    .getByRole('status')
    .getByText('已完成', { exact: true })
    .waitFor();
  assert.equal(
    await creationDialog.locator('[data-state="running"]').count(),
    0,
  );
  await page.screenshot({ path: resolve(output, 'creation-completed-zh.png') });
  await button('关闭').last().click();
  await page
    .locator('.mpa-history li')
    .filter({ hasText: 'failed-fixture' })
    .getByRole('button')
    .click();
  await creationDialog
    .getByRole('alert')
    .getByText('当前凭据没有执行该操作的权限，或凭据已失效。', { exact: true })
    .waitFor();
  await creationDialog.getByText('错误详情', { exact: true }).click();
  await creationDialog.getByText('异常类型', { exact: true }).waitFor();
  await page.screenshot({
    path: resolve(output, 'creation-error-details-zh.png'),
  });
  await button('关闭').last().click();
  checks.push(
    'Chinese completed steps and detailed failure explanations render with the same state semantics',
  );
  await click('创建 MPA');
  await click('核对状态');
  await click('新建另一个 MPA');
  await page
    .getByRole('textbox', { name: '名称', exact: true })
    .fill('lowercase-mpa');
  await page
    .getByRole('textbox', { name: '描述', exact: true })
    .fill('用于检查存储设置');
  await click('下一步');
  await click('下一步');
  await page
    .getByText('实际服务区域：cn-beijing · 已加载 2/2', { exact: true })
    .waitFor();
  await page.screenshot({ path: resolve(output, 'tos-storage-studio-zh.png') });
  await button('关闭').last().click();
  assert.deepEqual(errors, []);
  assert.deepEqual(await page.evaluate(() => globalThis.__designMissing), []);
  await writeFile(
    resolve(output, 'report.json'),
    JSON.stringify({ checks, errors }, null, 2),
  );
  console.log(JSON.stringify({ output, checks }, null, 2));
} catch (error) {
  await page.screenshot({ path: resolve(output, 'failure.png') });
  await writeFile(resolve(output, 'failure.html'), await page.content());
  console.error('Browser failure evidence:', output);
  throw error;
} finally {
  await browser.close();
}
