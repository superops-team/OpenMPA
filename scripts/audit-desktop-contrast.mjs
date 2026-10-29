import { chromium } from 'playwright';
import assert from 'node:assert/strict';
import { mkdir, writeFile } from 'node:fs/promises';
import { resolve } from 'node:path';
import { installDesktopFixture } from './desktop-design-fixture.mjs';

const origin = process.argv[2] || 'http://127.0.0.1:4174/desktop.html';
assert.equal(new URL(origin).hostname, '127.0.0.1');
const output = resolve('.openmpa/contrast-review');
await mkdir(output, { recursive: true });
const browser = await chromium.launch({ headless: true, channel: 'chrome' });
const context = await browser.newContext({
  viewport: { width: 1280, height: 900 },
  reducedMotion: 'reduce',
});
await context.addInitScript(installDesktopFixture);
const page = await context.newPage();
const results = [];
const errors = [];
page.on('pageerror', (error) => errors.push(error.message));

function audit() {
  const parse = (value) => {
    const parts = value.match(/[\d.]+/g)?.map(Number);
    return parts?.length >= 3
      ? [...parts.slice(0, 3), parts[3] ?? 1]
      : undefined;
  };
  const blend = (fg, bg) =>
    fg.slice(0, 3).map((value, i) => value * fg[3] + bg[i] * (1 - fg[3]));
  const luminance = (rgb) =>
    rgb
      .map((value) => {
        const s = value / 255;
        return s <= 0.04045 ? s / 12.92 : ((s + 0.055) / 1.055) ** 2.4;
      })
      .reduce((sum, value, i) => sum + value * [0.2126, 0.7152, 0.0722][i], 0);
  const background = (element) => {
    const chain = [];
    for (let current = element; current; current = current.parentElement)
      chain.unshift(current);
    let bg = [255, 255, 255];
    for (const node of chain) {
      const style = getComputedStyle(node);
      if (style.backgroundImage !== 'none' || style.opacity !== '1')
        return undefined;
      const color = parse(style.backgroundColor);
      if (color) bg = blend(color, bg);
    }
    return bg;
  };
  const inspect = (element, text, pseudo) => {
    if (
      !text.trim() ||
      element.closest(
        '[inert], [aria-hidden="true"], :disabled, .sr-only, script, style, svg',
      )
    )
      return;
    const rect = element.getBoundingClientRect();
    if (
      !rect.width ||
      !rect.height ||
      rect.bottom <= 0 ||
      rect.top >= innerHeight ||
      rect.right <= 0 ||
      rect.left >= innerWidth
    )
      return;
    for (let node = element; node; node = node.parentElement) {
      const style = getComputedStyle(node);
      if (style.visibility === 'hidden' || style.display === 'none') return;
      if (style.overflowY !== 'visible') {
        const clip = node.getBoundingClientRect();
        if (rect.top >= clip.bottom || rect.bottom <= clip.top) return;
      }
    }
    const style = getComputedStyle(element, pseudo);
    const foreground = parse(style.color);
    const bg = background(element);
    if (!bg || !foreground) return;
    foreground[3] *= Number(style.opacity);
    const a = luminance(blend(foreground, bg));
    const b = luminance(bg);
    const ratio = (Math.max(a, b) + 0.05) / (Math.min(a, b) + 0.05);
    const large =
      parseFloat(style.fontSize) >= 24 ||
      (parseFloat(style.fontSize) >= 18.66 && Number(style.fontWeight) >= 700);
    const minimum = large ? 3 : 4.5;
    if (ratio + 0.01 < minimum)
      failures.push({
        text: text.trim().slice(0, 100),
        element: element.tagName.toLowerCase(),
        class: element.className,
        color: style.color,
        background: bg,
        ratio: Math.round(ratio * 100) / 100,
        minimum,
        pseudo,
      });
    checked++;
  };
  let checked = 0;
  const failures = [];
  const walker = document.createTreeWalker(document.body, NodeFilter.SHOW_TEXT);
  while (walker.nextNode())
    inspect(walker.currentNode.parentElement, walker.currentNode.textContent);
  for (const element of document.querySelectorAll('input,textarea,select')) {
    if (element instanceof HTMLSelectElement)
      inspect(element, element.selectedOptions[0]?.textContent || '');
    else
      inspect(
        element,
        element.value || element.placeholder || '',
        !element.value ? '::placeholder' : undefined,
      );
  }
  return { checked, failures };
}

try {
  await page.goto(origin);
  await page.getByText('Review complete', { exact: true }).waitFor();
  for (const theme of ['light', 'dark']) {
    await page.evaluate(
      (theme) => globalThis.__designNavigate('themeChanged', theme),
      theme,
    );
    for (const section of [
      'Account',
      'Runtime',
      'Assistant',
      'Authentication',
      'Deployment',
      'Theme',
      'Language',
      'home',
      'tasks',
      'channels',
      'knowledge',
      'trace',
    ]) {
      if (section === 'trace')
        await page.evaluate(() => globalThis.__designNavigate('home'));
      await page.evaluate(
        (section) =>
          globalThis.__designNavigate(
            /^[A-Z]/.test(section) ? 'selectSettingsSection' : section,
            section,
          ),
        section,
      );
      if (section === 'trace')
        await page.getByRole('dialog', { name: 'Turn trace' }).waitFor();
      await page.waitForFunction(
        () => !document.querySelector('[aria-busy="true"]'),
      );
      await page.evaluate(
        () =>
          new Promise((resolve) =>
            requestAnimationFrame(() => requestAnimationFrame(resolve)),
          ),
      );
      results.push({ theme, section, ...(await page.evaluate(audit)) });
      if (section === 'trace')
        await page
          .getByRole('button', { name: 'Close trace', exact: true })
          .click();
    }
    await page.evaluate(() => globalThis.__designNavigate('home'));
    await page
      .getByRole('button', { name: 'Assistant settings', exact: true })
      .click();
    await page.getByRole('textbox', { name: 'Assistant name' }).waitFor();
    await page.evaluate(
      () =>
        new Promise((resolve) =>
          requestAnimationFrame(() => requestAnimationFrame(resolve)),
        ),
    );
    results.push({
      theme,
      section: 'assistant-dialog',
      ...(await page.evaluate(audit)),
    });
    await page.getByRole('tab', { name: 'Skills', exact: true }).click();
    results.push({
      theme,
      section: 'assistant-skills',
      ...(await page.evaluate(audit)),
    });
    await page.getByRole('button', { name: 'Dismiss', exact: true }).click();
  }
  await writeFile(
    resolve(output, 'report.json'),
    JSON.stringify({ origin, results, errors }, null, 2),
  );
  console.log(
    JSON.stringify(
      {
        checked: results.reduce((n, r) => n + r.checked, 0),
        failures: results.filter((r) => r.failures.length),
        errors,
        output,
      },
      null,
      2,
    ),
  );
  if (errors.length || results.some((r) => r.failures.length))
    process.exitCode = 1;
} finally {
  await browser.close();
}
