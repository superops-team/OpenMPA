import assert from 'node:assert/strict';
import { mkdir, writeFile } from 'node:fs/promises';
import { resolve } from 'node:path';
import { pathToFileURL } from 'node:url';
import { chromium } from 'playwright';
import { installDesktopFixture } from './desktop-design-fixture.mjs';
import { installChatInteractionFixture } from './chat-interaction-fixture.mjs';

export async function verifyChatInteractions({
  browser,
  origin,
  output,
  recordOnly = false,
}) {
  await mkdir(output, { recursive: true });
  const results = [];
  const errors = [];
  for (const locale of ['en', 'zh-Hans']) {
    for (const reducedMotion of ['no-preference', 'reduce']) {
      const context = await browser.newContext({
        viewport: { width: 920, height: 620 },
        reducedMotion,
      });
      await context.addInitScript(installDesktopFixture);
      await context.addInitScript(installChatInteractionFixture);
      const page = await context.newPage();
      page.on('pageerror', (error) => errors.push(error.message));
      try {
        await page.goto(origin);
        await page.waitForFunction(
          () => globalThis.__designShell?.selectedAssistant,
        );
        await page.evaluate(() =>
          globalThis.__designNavigate('selectSession', 'session-design'),
        );
        await page.locator('.composer-field textarea').waitFor();
        await page.evaluate(
          (locale) => globalThis.__designLocale(locale),
          locale,
        );
        const input = page.locator('.composer-field textarea');
        await input.fill(
          'Introduce yourself\nwith enough detail\nfor a short example',
        );
        const expandedHeight = await input.evaluate(
          (element) => element.getBoundingClientRect().height,
        );
        await page.evaluate(() => {
          const input = document.querySelector('.composer-field textarea');
          input.addEventListener(
            'keydown',
            () => {
              globalThis.__sendFrame = new Promise((resolve) =>
                requestAnimationFrame(() =>
                  resolve({
                    value: input.value,
                    height: input.getBoundingClientRect().height,
                    at: performance.now(),
                  }),
                ),
              );
            },
            { once: true },
          );
        });
        await input.press('Enter');
        const firstFrame = await page.evaluate(() => globalThis.__sendFrame);
        const pendingBeforeReceipt = await page.evaluate(
          () => !!globalThis.__chatSubmitted,
        );
        if (!recordOnly) {
          assert.equal(
            firstFrame.value,
            '',
            'Shared draft must clear on the first rendered frame',
          );
          assert.ok(
            firstFrame.height < expandedHeight,
            'Multiline composer must shrink before the next paint',
          );
        }
        assert.equal(pendingBeforeReceipt, true);
        await input.fill('New unsent draft');
        await page.evaluate(() => globalThis.__acceptChatSubmission());
        await page.waitForFunction(() => globalThis.__chatEmit);
        const process = page
          .locator('.conversation-turn')
          .last()
          .locator('.turn-process');
        await process.locator('.ui-status-row').waitFor();
        const runningMotion = await page
          .locator('.composer-action')
          .evaluate((element) => ({
            ring: getComputedStyle(element, '::after').content,
            ringAnimation: getComputedStyle(element, '::after').animationName,
            iconAnimation: getComputedStyle(element.querySelector('svg'))
              .animationName,
          }));
        await page.evaluate(() =>
          globalThis.__chatEmit({
            kind: 'toolCall',
            toolId: 'command-one',
            toolName: 'commandExecution',
            toolStatus: 'running',
            toolSubject: 'pwd',
            partial: true,
          }),
        );
        await process.locator('.ui-status-row').nth(1).waitFor();
        const thinkingAfterTool = await process.innerText();
        await page.evaluate(() =>
          globalThis.__chatEmit({
            kind: 'toolResult',
            toolId: 'command-one',
            toolName: 'commandExecution',
            toolStatus: 'success',
            durationMs: 423,
            outputPreview: '/workspace',
          }),
        );
        await page.evaluate(() => globalThis.__settleChat());
        await page
          .getByText('This is the completed answer.', { exact: true })
          .waitFor();
        await page.evaluate(
          () =>
            new Promise((resolve) =>
              requestAnimationFrame(() => requestAnimationFrame(resolve)),
            ),
        );
        const text = await process.innerText();
        const open = await process
          .locator('.ui-disclosure-trigger')
          .getAttribute('aria-expanded');
        const draft = await input.inputValue();
        const result = {
          locale,
          reducedMotion,
          firstFrame,
          expandedHeight,
          thinkingAfterTool,
          text,
          open,
          draft,
          runningMotion,
        };
        results.push(result);
        await page.screenshot({
          path: resolve(output, `${locale}-${reducedMotion}.png`),
        });
        if (!recordOnly) {
          assert.ok(
            !/Thinking|思考中/.test(thinkingAfterTool),
            'A reasoning segment must settle when the next step begins',
          );
          assert.ok(
            !/Thinking|思考中|Working|执行中/.test(text),
            'Terminal turn must not retain running labels',
          );
          assert.ok(
            text.includes('423'),
            'Tool timing must survive settlement',
          );
          assert.equal(
            open,
            'true',
            'Live process stays open after completion',
          );
          assert.equal(
            draft,
            'New unsent draft',
            'Late receipt must preserve later input',
          );
          assert.equal(runningMotion.ring, 'none');
          assert.equal(runningMotion.iconAnimation, 'none');
        }
      } finally {
        await context.close();
      }
    }
  }
  await writeFile(
    resolve(output, 'report.json'),
    JSON.stringify({ results, errors }, null, 2),
  );
  assert.deepEqual(errors, []);
  return {
    cases: results.length,
    output,
    errors,
    firstFrameDrafts: results.map((item) => item.firstFrame.value),
  };
}
if (import.meta.url === pathToFileURL(resolve(process.argv[1])).href) {
  const browser = await chromium.launch({ headless: true, channel: 'chrome' });
  try {
    console.log(
      JSON.stringify(
        await verifyChatInteractions({
          browser,
          origin: process.argv[2] || 'http://127.0.0.1:4181/desktop.html',
          output: resolve(
            '.openmpa/chat-interactions',
            new Date().toISOString().replaceAll(':', '-'),
          ),
          recordOnly: process.argv.includes('--record-only'),
        }),
        null,
        2,
      ),
    );
  } finally {
    await browser.close();
  }
}
