import assert from 'node:assert/strict';
import { chromium } from 'playwright';
import { mkdir, writeFile } from 'node:fs/promises';
import { resolve } from 'node:path';
import { installDesktopFixture } from './desktop-design-fixture.mjs';
import { installStudioFixture } from './desktop-studio-fixture.mjs';

const origin = process.argv[2] || 'http://127.0.0.1:4178';
assert.equal(new URL(origin).hostname, '127.0.0.1');
const output = resolve(
  '.openmpa/studio-verification',
  new Date().toISOString().replaceAll(':', '-'),
);
await mkdir(output, { recursive: true });
const browser = await chromium.launch({ channel: 'chrome', headless: true });
const context = await browser.newContext({
  viewport: { width: 1280, height: 900 },
  reducedMotion: 'reduce',
});
await context.addInitScript(installDesktopFixture);
await context.addInitScript(installStudioFixture);
const page = await context.newPage();
page.setDefaultTimeout(10000);
page.on('dialog', (dialog) => dialog.accept());
const errors = [];
const checks = [];
page.on('pageerror', (error) => errors.push(error.message));
const navigate = async (name) => {
  await page
    .getByRole('button', { name: `Fixture ${name}`, exact: true })
    .click();
  for (const section of ['resources', 'collaboration'])
    assert.equal(
      await page.getByTestId(`studio-${section}`).isVisible(),
      name === section,
      `${section} must follow actual navigation visibility`,
    );
};
const check = (name) => checks.push(name);
try {
  await page.goto(origin);
  await navigate('chat');
  const composer = page.getByRole('textbox', { name: 'Message', exact: true });
  await composer.fill('Retained conversation draft');
  await navigate('resources');
  await page
    .getByRole('button', { name: 'Refresh skills', exact: true })
    .click();
  await page.getByText('release-review', { exact: true }).waitFor();
  await page
    .getByRole('button', { name: 'Use in conversation', exact: true })
    .first()
    .click();
  assert.match(await composer.inputValue(), /Retained conversation draft/);
  assert.match(await composer.inputValue(), /\$release-review/);
  check('Skill choice retains existing unsent conversation text');

  await navigate('resources');
  await page.getByRole('button', { name: 'MCP', exact: true }).click();
  await page
    .getByRole('button', { name: 'Refresh MCP servers', exact: true })
    .click();
  await page.getByText('read_contract', { exact: true }).waitFor();
  await page
    .getByText('Tool details are unavailable in this Runtime.', { exact: true })
    .waitFor();
  await page
    .getByText('No tools are listed for this server.', { exact: true })
    .waitFor();
  check('MCP tool details distinguish unavailable and empty tool lists');

  for (const [tab, reload, save, marker] of [
    [
      'AGENTS.md',
      'Reload AGENTS.md',
      'Save AGENTS.md',
      'Retained shared draft',
    ],
    [
      'Main Profile',
      'Reload Main Profile',
      'Save Main Profile',
      'Retained Main draft',
    ],
  ]) {
    await page.getByRole('button', { name: tab, exact: true }).click();
    await page.getByRole('button', { name: reload, exact: true }).click();
    const input = page.getByTestId(
      tab === 'AGENTS.md'
        ? 'resources-agentsMd-draft'
        : 'resources-mainProfile-draft',
    );
    await input.fill(marker);
    await page
      .getByRole('button', { name: 'Fixture conflict off', exact: true })
      .click();
    await page.getByRole('button', { name: save, exact: true }).click();
    await page
      .getByRole('status')
      .filter({ hasText: 'The resource changed.' })
      .waitFor();
    assert.equal(await input.inputValue(), marker);
    assert.equal(
      await page.getByRole('button', { name: save, exact: true }).isDisabled(),
      true,
    );
    await page.getByRole('button', { name: reload, exact: true }).click();
    assert.equal(await input.inputValue(), marker);
    await navigate('language');
    await page.getByText('中文', { exact: true }).click();
    await navigate('resources');
    assert.equal(
      await page.locator('textarea:visible').first().inputValue(),
      marker,
    );
    await navigate('language');
    await page.getByText('English', { exact: true }).click();
    await navigate('resources');
    await page
      .getByRole('button', { name: 'Fixture conflict on', exact: true })
      .click();
    check(
      `${tab} conflict, reread and language switch retain instruction draft`,
    );
  }

  await navigate('collaboration');
  await page
    .getByRole('button', { name: 'Refresh collaboration', exact: true })
    .click();
  await page
    .getByRole('heading', { name: 'Fixture collaborator', exact: true })
    .waitFor();
  await page.getByRole('button', { name: 'Unbind space', exact: true }).click();
  await page.getByText('Space binding saved.', { exact: true }).waitFor();
  await page
    .getByRole('button', { name: 'Refresh collaboration', exact: true })
    .click();
  await page
    .getByRole('button', { name: 'Bind selected space', exact: true })
    .click();
  await page.getByText('Space binding saved.', { exact: true }).waitFor();
  await page
    .getByRole('button', { name: 'Refresh collaboration', exact: true })
    .click();
  check('A2A unbind and bind require confirmation and a fresh revision');
  await page.getByTestId('collaboration-member').selectOption('a-fixture');
  await page.getByTestId('collaboration-alias').fill('auditor');
  await page
    .getByRole('button', { name: 'Save member alias', exact: true })
    .click();
  await page.getByText('Member alias saved.', { exact: true }).waitFor();
  await page
    .getByRole('button', { name: 'Refresh collaboration', exact: true })
    .click();
  await page.getByText('auditor', { exact: false }).first().waitFor();
  await page.getByTestId('collaboration-task').fill('Review this');
  await page
    .getByRole('button', { name: 'Use in conversation', exact: true })
    .click();
  assert.match(await composer.inputValue(), /^@auditor Review this/);
  assert.match(await composer.inputValue(), /Retained conversation draft/);
  assert.match(await composer.inputValue(), /\$release-review/);
  check(
    'Persisted collaboration alias adds an @mention before the retained draft',
  );

  const evidence = await page.evaluate(() => ({
    missing: globalThis.__designMissing,
    calls: globalThis.__designCalls,
    usageMethods: globalThis.__studioRequests.map((entry) => entry.method),
    view: globalThis.__designShell.view,
  }));
  assert.equal(evidence.missing.length, 0, evidence.missing.join(', '));
  assert.equal(evidence.calls.includes('Session.Submit'), false);
  assert.equal(evidence.calls.includes('Session.Send'), false);
  check('Choices do not submit a conversation or contact cloud services');
  assert.equal(await page.getByTestId('studio-resources').isVisible(), false);
  assert.equal(
    await page.getByTestId('studio-collaboration').isVisible(),
    false,
  );
  check('Retained resource and collaboration pages stay hidden during chat');
  await page.screenshot({ path: resolve(output, 'conversation-draft.png') });
  for (const name of ['resources', 'collaboration']) {
    await navigate(name);
    for (const width of [1280, 768, 320]) {
      await page.setViewportSize({ width, height: 900 });
      const layout = await page
        .getByTestId(`studio-${name}`)
        .evaluate((element) => ({
          width: element.getBoundingClientRect().width,
          clientWidth: element.clientWidth,
          scrollWidth: element.scrollWidth,
          clientHeight: element.clientHeight,
          scrollHeight: element.scrollHeight,
          overflowY: getComputedStyle(element).overflowY,
        }));
      assert.ok(layout.width <= width, JSON.stringify(layout));
      assert.ok(
        layout.scrollWidth <= layout.clientWidth,
        JSON.stringify(layout),
      );
      assert.ok(
        layout.scrollHeight <= layout.clientHeight ||
          layout.overflowY === 'auto',
        `Long ${name} content must be scrollable`,
      );
      if (width === 1280)
        await page.screenshot({ path: resolve(output, `${name}.png`) });
    }
    check(
      `${name} layout fits 1280, 768 and 320 pixels with vertical scrolling`,
    );
  }
  await writeFile(
    resolve(output, 'evidence.json'),
    JSON.stringify(evidence, null, 2),
  );
} catch (error) {
  errors.push(error.stack || error.message);
} finally {
  await writeFile(
    resolve(output, 'report.json'),
    JSON.stringify(
      {
        environment: 'Actual React pages, simulated MyGo IPC, no cloud calls',
        browser: browser.version(),
        checks,
        errors,
      },
      null,
      2,
    ),
  );
  await browser.close();
  console.log(
    JSON.stringify(
      { checks, errors, report: resolve(output, 'report.json') },
      null,
      2,
    ),
  );
  if (errors.length) process.exitCode = 1;
}
