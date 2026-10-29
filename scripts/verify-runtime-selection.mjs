import assert from 'node:assert/strict';
import { mkdir, writeFile } from 'node:fs/promises';
import { resolve } from 'node:path';
import { pathToFileURL } from 'node:url';
import { chromium } from 'playwright';
import { installDesktopFixture } from './desktop-design-fixture.mjs';

export async function verifyRuntimeSelection({
  browser,
  origin,
  output,
  recordOnly = false,
}) {
  assert.equal(new URL(origin).hostname, '127.0.0.1');
  await mkdir(output, { recursive: true });
  const results = [];
  const errors = [];
  try {
    for (const width of [1280, 760, 480, 320]) {
      for (const locale of ['en', 'zh-Hans']) {
        for (const theme of ['light', 'dark']) {
          const context = await browser.newContext({
            viewport: { width, height: 620 },
            reducedMotion: 'no-preference',
          });
          await context.addInitScript(installDesktopFixture);
          const page = await context.newPage();
          page.on('pageerror', (error) => errors.push(error.message));
          await page.goto(origin);
          await page.waitForFunction(() => globalThis.__designShell);
          await page.evaluate(
            ({ locale, theme }) => {
              globalThis.__designLocale(locale);
              globalThis.__designNavigate('themeChanged', theme);
              globalThis.__designOverrides['Platform.ListRuntimes'] = () => ({
                ok: true,
                value: {
                  complete: true,
                  runtimes: Array.from({ length: 12 }, (_, index) => ({
                    runtimeId:
                      index === 0
                        ? 'runtime-design'
                        : index === 6
                          ? 'runtime-secondary'
                          : `runtime-long-identifier-${index}-abcdef0123456789`,
                    name: `Runtime ${index + 1}: A long assistant name for checking line wrapping and alignment`,
                    status: 'Ready',
                    currentVersionNumber: 3,
                  })),
                },
              });
              globalThis.__designNavigate('selectSettingsSection', 'Runtime');
            },
            { locale, theme },
          );
          const rows = page.locator('.desktop-runtime');
          await rows.nth(11).waitFor();
          const target = rows.nth(6);
          const button = target.getByRole('button');
          await page.evaluate(() => {
            globalThis.__originalRuntimeRows = [
              ...document.querySelectorAll('.desktop-runtime'),
            ];
          });
          for (const outcome of ['failure', 'success']) {
            await page.evaluate((outcome) => {
              globalThis.__designOverrides['Platform.BindRuntime'] = (
                arg,
                original,
              ) =>
                new Promise((resolve) => {
                  globalThis.__finishRuntimeBind = () =>
                    resolve(
                      outcome === 'success'
                        ? { ok: true, value: original(arg) }
                        : {
                            ok: false,
                            error: {
                              code: 'testBindFailure',
                              message:
                                'Runtime selection failed. Please try again.',
                            },
                          },
                    );
                });
            }, outcome);
            await button.scrollIntoViewIfNeeded();
            await page.mouse.move(0, 0);
            if (outcome === 'failure') await button.focus();
            await page.evaluate(() => {
              globalThis.__runtimeGeometry = () => {
                const main = document.querySelector('.desktop-settings');
                const geometry = (element) => {
                  const rect = element.getBoundingClientRect();
                  return {
                    x: rect.x,
                    y: rect.y,
                    width: rect.width,
                    height: rect.height,
                  };
                };
                return {
                  scrollTop: main.scrollTop,
                  scrollHeight: main.scrollHeight,
                  clientWidth: main.clientWidth,
                  heading: geometry(main.querySelector('.page-heading')),
                  rows: [...main.querySelectorAll('.desktop-runtime')].map(
                    (row) => ({
                      row: geometry(row),
                      text: geometry(row.firstElementChild),
                      button: geometry(row.querySelector('button')),
                    }),
                  ),
                };
              };
              globalThis.__runtimeBefore = globalThis.__runtimeGeometry();
              globalThis.__runtimeSamples = [];
              globalThis.__captureRuntime = true;
              const capture = () => {
                if (!globalThis.__captureRuntime) return;
                globalThis.__runtimeSamples.push(
                  globalThis.__runtimeGeometry(),
                );
                requestAnimationFrame(capture);
              };
              requestAnimationFrame(capture);
            });
            if (outcome === 'failure') await button.press('Enter');
            else await button.click();
            await page.waitForFunction(() => globalThis.__finishRuntimeBind);
            assert.equal(await target.getAttribute('data-selecting'), 'true');
            assert.equal(await button.isDisabled(), true);
            await page.screenshot({
              path: resolve(
                output,
                `${width}-${locale}-${theme}-${outcome}-pending.png`,
              ),
            });
            await page.evaluate(() => globalThis.__finishRuntimeBind());
            await page.waitForFunction(
              () =>
                !document.querySelector(
                  '.desktop-runtime[data-selecting="true"]',
                ),
            );
            await page.evaluate(
              () =>
                new Promise((resolve) => {
                  const until = performance.now() + 250;
                  const settle = () =>
                    performance.now() >= until
                      ? resolve()
                      : requestAnimationFrame(settle);
                  requestAnimationFrame(settle);
                }),
            );
            const data = await page.evaluate(() => {
              globalThis.__captureRuntime = false;
              return {
                before: globalThis.__runtimeBefore,
                samples: globalThis.__runtimeSamples,
                after: globalThis.__runtimeGeometry(),
              };
            });
            const delta = (before, current) => {
              if (typeof before === 'number') return Math.abs(current - before);
              return Math.max(
                ...Object.keys(before).map((key) =>
                  delta(before[key], current[key]),
                ),
              );
            };
            // Button press feedback may transform the button; row and text layout
            // must remain fixed throughout binding. Button dimensions are checked
            // again after the pointer is released.
            const layout = (value) => ({
              ...value,
              rows: value.rows.map(({ row, text }) => ({ row, text })),
            });
            const maxLayoutShift = Math.max(
              ...data.samples.map((sample) =>
                delta(layout(data.before), layout(sample)),
              ),
              delta(layout(data.before), layout(data.after)),
            );
            const finalButtonShift = Math.max(
              ...data.before.rows.map((row, index) =>
                delta(row.button, data.after.rows[index].button),
              ),
            );
            await page.screenshot({
              path: resolve(
                output,
                `${width}-${locale}-${theme}-${outcome}-after.png`,
              ),
            });
            const result = {
              width,
              locale,
              theme,
              outcome,
              maxLayoutShift,
              finalButtonShift,
              ...data,
            };
            results.push(result);
            assert.equal(
              await target.getAttribute('data-selected'),
              String(outcome === 'success'),
            );
            if (outcome === 'failure')
              assert.equal(await button.isEnabled(), true);
          }
          assert.equal(
            await page.evaluate(() =>
              globalThis.__originalRuntimeRows.every(
                (row, index) =>
                  row === document.querySelectorAll('.desktop-runtime')[index],
              ),
            ),
            true,
            'Binding must preserve the loaded Runtime rows',
          );
          await context.close();
        }
      }
    }
  } finally {
    await writeFile(
      resolve(output, 'report.json'),
      JSON.stringify({ origin, errors, results }, null, 2),
    );
  }
  assert.deepEqual(errors, []);
  if (!recordOnly) {
    for (const result of results) {
      assert.ok(
        result.maxLayoutShift < 0.5,
        `Runtime layout shifted: ${JSON.stringify({ width: result.width, locale: result.locale, theme: result.theme, outcome: result.outcome, delta: result.maxLayoutShift })}`,
      );
      assert.ok(
        result.finalButtonShift < 0.5,
        'Selection labels must not change button geometry',
      );
    }
  }
  return {
    output,
    errors,
    cases: results.length,
    maxLayoutShift: Math.max(...results.map((item) => item.maxLayoutShift)),
    maxButtonShift: Math.max(...results.map((item) => item.finalButtonShift)),
  };
}

if (import.meta.url === pathToFileURL(resolve(process.argv[1])).href) {
  const browser = await chromium.launch({ headless: true, channel: 'chrome' });
  try {
    const result = await verifyRuntimeSelection({
      browser,
      origin: process.argv[2] || 'http://127.0.0.1:4181/desktop.html',
      output: resolve(
        '.openmpa/runtime-selection',
        new Date().toISOString().replaceAll(':', '-'),
      ),
      recordOnly: process.argv.includes('--record-only'),
    });
    console.log(JSON.stringify(result, null, 2));
  } finally {
    await browser.close();
  }
}
