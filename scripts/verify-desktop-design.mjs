import { chromium } from 'playwright';
import assert from 'node:assert/strict';
import { mkdir, writeFile } from 'node:fs/promises';
import process from 'node:process';
import console from 'node:console';
import { resolve } from 'node:path';
import { createHash } from 'node:crypto';
import { execFileSync } from 'node:child_process';
import { existsSync, readFileSync } from 'node:fs';
import { installDesktopFixture } from './desktop-design-fixture.mjs';
import { verifyDesktopInteractions } from './verify-desktop-interactions.mjs';

const origin = process.argv[2] || 'http://127.0.0.1:4173';
assert.equal(
  new URL(origin).hostname,
  '127.0.0.1',
  'Design verification requires a local server',
);
const runId = new Date().toISOString().replaceAll(':', '-');
const output = resolve('.openmpa/design-verification', runId);
const files = execFileSync(
  'git',
  [
    'ls-files',
    '-z',
    '--cached',
    '--others',
    '--exclude-standard',
    'src/client',
    'src/design',
    'src/build',
    'design',
    'desktop/internal/design',
    'desktop/internal/host',
    'scripts/verify-desktop-design.mjs',
    'scripts/verify-desktop-interactions.mjs',
    'scripts/desktop-design-fixture.mjs',
  ],
  { encoding: 'utf8' },
)
  .split('\0')
  .filter((file) => file && existsSync(file));
const sourceHash = createHash('sha256');
for (const file of [...new Set(files)].sort())
  sourceHash.update(file).update('\0').update(readFileSync(file)).update('\0');
const sourceDigest = sourceHash.digest('hex');
await mkdir(output, { recursive: true });
const browser = await chromium.launch({ headless: true, channel: 'chrome' });
const context = await browser.newContext({
  viewport: { width: 1280, height: 900 },
  reducedMotion: 'reduce',
});
await context.addInitScript(installDesktopFixture);
const page = await context.newPage();
const errors = [];
page.on('pageerror', (error) => errors.push(error.stack || error.message));
const results = [];
let assistantDialog;
let visualFeedback;
try {
  await page.goto(origin);
  await page.waitForFunction(
    () => globalThis.__designShell?.selectedAssistant === 'runtime-design',
  );
  await page.evaluate(() =>
    globalThis.__designNavigate('selectSession', 'session-design'),
  );
  await page.getByText('Review complete', { exact: true }).waitFor();
  const settings = [
    'Account',
    'Runtime',
    'Assistant',
    'Authentication',
    'Deployment',
    'Theme',
    'Language',
  ];
  const journeys = [
    ...settings.map((id) => ({
      name: `settings-${id.toLowerCase()}`,
      action: 'selectSettingsSection',
      id,
    })),
    { name: 'channels', action: 'channels' },
    { name: 'schedules', action: 'tasks' },
    { name: 'knowledge', action: 'knowledge' },
    { name: 'chat', action: 'home' },
    { name: 'trace', action: 'trace' },
  ];
  for (const theme of ['light', 'dark']) {
    await page.evaluate(
      (value) => globalThis.__designNavigate('themeChanged', value),
      theme,
    );
    for (const journey of journeys) {
      await page.evaluate(
        ({ action, id }) => globalThis.__designNavigate(action, id),
        journey,
      );
      if (journey.name.startsWith('settings-'))
        await page
          .getByRole('heading', { name: journey.id, exact: true })
          .waitFor();
      if (journey.name === 'channels')
        await page
          .getByRole('heading', {
            name: 'Channels and templates.',
            exact: true,
          })
          .waitFor();
      if (journey.name === 'schedules')
        await page
          .getByRole('heading', {
            name: 'A little follow-through.',
            exact: true,
          })
          .waitFor();
      if (journey.name === 'chat')
        await page.getByText('Review complete', { exact: true }).waitFor();
      if (journey.name === 'knowledge')
        await page.getByRole('combobox').first().selectOption('session-design');
      if (journey.name === 'trace')
        await page.getByRole('dialog', { name: 'Turn trace' }).waitFor();
      await page.waitForFunction(
        () => !document.querySelector('[aria-busy="true"]'),
      );
      for (const width of [1280, 768, 320]) {
        await page.setViewportSize({ width, height: 900 });
        await page.evaluate(
          () =>
            new Promise((resolve) =>
              requestAnimationFrame(() => requestAnimationFrame(resolve)),
            ),
        );
        const metrics = await page.evaluate(() => ({
          theme: document.documentElement.dataset.theme,
          viewport: innerWidth,
          width: document.documentElement.scrollWidth,
          body: document.body.scrollWidth,
          main: document.querySelector('main')?.getBoundingClientRect().width,
          primary: [
            ...document.querySelectorAll('button.primary:not(:disabled)'),
          ].map((element) => ({
            text: element.textContent.trim(),
            background: getComputedStyle(element).backgroundColor,
          })),
          expectedPrimary: (() => {
            const value = getComputedStyle(document.documentElement)
              .getPropertyValue('--color-primary')
              .trim();
            return `rgb(${[1, 3, 5].map((i) => parseInt(value.slice(i, i + 2), 16)).join(', ')})`;
          })(),
        }));
        const image = `${journey.name}-${theme}-${width}.png`;
        assert.equal(errors.length, 0, errors.join('\n'));
        assert.equal(metrics.theme, theme);
        await page.screenshot({ path: resolve(output, image) });
        results.push({
          journey: journey.name,
          theme,
          width,
          metrics,
          image,
          pass:
            metrics.width <= width &&
            metrics.body <= width &&
            metrics.primary.every(
              (button) => button.background === metrics.expectedPrimary,
            ),
        });
      }
      await page.setViewportSize({ width: 1280, height: 900 });
      if (journey.name === 'trace') {
        await page.getByRole('button', { name: 'Tree', exact: true }).click();
        await page
          .getByRole('button', { name: 'Read DESIGN.md', exact: false })
          .click();
        await page.keyboard.press('Escape');
        await page
          .getByRole('dialog', { name: 'Turn trace' })
          .waitFor({ state: 'detached' });
      }
    }
  }
  assert.equal(
    results.length,
    72,
    'All production page/theme/width combinations must run',
  );
  ({ assistantDialog, visualFeedback } = await verifyDesktopInteractions({
    context,
    page,
    origin,
    output,
    errors,
  }));
} catch (error) {
  errors.push(error.message);
} finally {
  const missing = await page.evaluate(() => globalThis.__designMissing ?? []);
  const report = {
    runId,
    sourceDigest,
    browser: browser.version(),
    environment:
      'production React components, simulated MyGo transport, no cloud calls',
    results,
    assistantDialog,
    visualFeedback,
    errors,
    missing: [...new Set(missing)],
  };
  await writeFile(
    resolve(output, 'report.json'),
    JSON.stringify(report, null, 2),
  );
  await browser.close();
  console.log(
    JSON.stringify(
      {
        screens: results.length,
        overflows: results.filter((item) => !item.pass),
        errors,
        missing: report.missing,
        report: resolve(output, 'report.json'),
      },
      null,
      2,
    ),
  );
  if (errors.length || missing.length || results.some((item) => !item.pass))
    process.exitCode = 1;
}
