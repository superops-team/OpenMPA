import assert from 'node:assert/strict';
import { writeFile } from 'node:fs/promises';
import { resolve } from 'node:path';

export async function verifyDesktopInteractions({
  context,
  page,
  origin,
  output,
  errors,
}) {
  await page.setViewportSize({ width: 1280, height: 900 });
  await page.emulateMedia({
    colorScheme: 'light',
    reducedMotion: 'no-preference',
  });
  await page.evaluate(() =>
    globalThis.__designNavigate('themeChanged', 'light'),
  );
  await page.evaluate(() => globalThis.__designNavigate('home'));
  await page.getByText('Review complete', { exact: true }).waitFor();
  const message = page.getByRole('textbox', { name: 'Message' });
  await message.focus();
  const composerFocus = await page
    .locator('.composer-field')
    .evaluate((element) => {
      const textarea = element.querySelector('textarea');
      const outer = getComputedStyle(element);
      const inner = getComputedStyle(textarea);
      return {
        outerBorderColor: outer.borderColor,
        innerBorderWidth: inner.borderWidth,
        innerOutlineStyle: inner.outlineStyle,
        innerOutlineWidth: inner.outlineWidth,
      };
    });
  assert.equal(
    composerFocus.innerOutlineStyle,
    'none',
    'The composer textarea must not draw a second focus ring',
  );
  assert.equal(
    composerFocus.innerBorderWidth,
    '0px',
    'The composer textarea must not draw an inner focus border',
  );
  assert.equal(
    await page.locator('.chat-persona').count(),
    0,
    'Desktop Chat must not duplicate native identity in a floating header',
  );
  const disclosureTrigger = page
    .locator('.turn-process .ui-disclosure-trigger')
    .first();
  const disclosureBox = await disclosureTrigger.boundingBox();
  assert.ok(disclosureBox, 'Turn process disclosure must be reachable');
  await page.mouse.move(
    disclosureBox.x + disclosureBox.width / 2,
    disclosureBox.y + disclosureBox.height / 2,
  );
  await page.mouse.down();
  const disclosurePressedTransform = await disclosureTrigger.evaluate(
    (element) => getComputedStyle(element).transform,
  );
  const disclosureChevronMotion = await disclosureTrigger
    .locator('svg')
    .evaluate((element) => {
      const style = getComputedStyle(element);
      return {
        property: style.transitionProperty,
        duration: style.transitionDuration,
      };
    });
  await page.mouse.up();
  assert.equal(
    disclosurePressedTransform,
    'none',
    'Full-width disclosure rows must not scale or distort on press',
  );
  assert.equal(disclosureChevronMotion.property, 'transform');
  assert.equal(disclosureChevronMotion.duration, '0.15s');
  const chatRect = () =>
    page.locator('.chat-column').evaluate((element) => {
      const rect = element.getBoundingClientRect();
      return { x: rect.x, y: rect.y, width: rect.width, height: rect.height };
    });
  const beforeDialog = await chatRect();
  const chatLauncher = page.getByRole('button', { name: 'Bot settings' });
  await chatLauncher.click();
  const dialog = page.getByRole('dialog', { name: 'Assistant settings' });
  await dialog.waitFor();
  const layer = page.locator('body > .ui-modal-layer');
  assert.equal(
    await layer.count(),
    1,
    'Assistant settings must use one body portal',
  );
  assert.deepEqual(
    await chatRect(),
    beforeDialog,
    'Opening Assistant settings must not reflow Chat',
  );
  const wideDialog = await dialog.evaluate((element) => {
    const root = getComputedStyle(document.documentElement);
    return {
      width: element.getBoundingClientRect().width,
      expectedMax: parseFloat(
        root.getPropertyValue('--layout-settings-column'),
      ),
    };
  });
  assert.ok(
    wideDialog.width <= wideDialog.expectedMax,
    'Assistant settings must use the settings-column width token',
  );
  await page.waitForTimeout(220);
  const agentsWideImage = 'assistant-dialog-agents-light-1280.png';
  await page.screenshot({ path: resolve(output, agentsWideImage) });
  const tabState = async () =>
    page.getByRole('tab').evaluateAll((tabs) => {
      const root = getComputedStyle(document.documentElement);
      const selected = root.getPropertyValue('--color-card-selected').trim();
      const expectedSelected = `rgb(${[1, 3, 5]
        .map((index) => parseInt(selected.slice(index, index + 2), 16))
        .join(', ')})`;
      return {
        expectedSelected,
        tabs: tabs.map((tab) => ({
          label: tab.textContent?.trim(),
          selected: tab.getAttribute('aria-selected'),
          background: getComputedStyle(tab).backgroundColor,
        })),
      };
    });
  await page.getByRole('tab', { name: 'Skills' }).click();
  const immediateTabs = await tabState();
  const skillLayout = await dialog.evaluate((element) => {
    const list = element.querySelector('.assistant-skill-catalog');
    const rows = [...element.querySelectorAll('.assistant-skill-option')].map(
      (row) => {
        const description = row.querySelector('small');
        const rect = row.getBoundingClientRect();
        const descriptionRect = description?.getBoundingClientRect();
        return {
          height: rect.height,
          scrollHeight: row.scrollHeight,
          descriptionBottom: descriptionRect?.bottom,
          rowBottom: rect.bottom,
        };
      },
    );
    return {
      count: rows.length,
      listClientHeight: list?.clientHeight,
      listScrollHeight: list?.scrollHeight,
      overflowY: list ? getComputedStyle(list).overflowY : '',
      rows,
    };
  });
  const importSkillText = page.getByRole('textbox', {
    name: 'Import skill text',
  });
  await importSkillText.focus();
  const assistantControlFocus = await importSkillText.evaluate((element) => {
    const style = getComputedStyle(element);
    const root = getComputedStyle(document.documentElement);
    const focused = root
      .getPropertyValue('--color-input-border-focused')
      .trim();
    const expectedBorder = `rgb(${[1, 3, 5]
      .map((index) => parseInt(focused.slice(index, index + 2), 16))
      .join(', ')})`;
    return {
      borderColor: style.borderColor,
      outlineStyle: style.outlineStyle,
      outlineWidth: style.outlineWidth,
      boxShadow: style.boxShadow,
      expectedBorder,
    };
  });
  assert.equal(
    assistantControlFocus.outlineStyle,
    'none',
    'Assistant controls must use the same single focus border as Composer',
  );
  assert.equal(
    assistantControlFocus.borderColor,
    assistantControlFocus.expectedBorder,
  );
  assert.equal(assistantControlFocus.boxShadow, 'none');
  assert.ok(skillLayout.count >= 8, 'Long skill catalogs must be represented');
  assert.ok(
    skillLayout.rows.every(
      (row) =>
        row.height >= row.scrollHeight &&
        row.descriptionBottom <= row.rowBottom,
    ),
    'Skill descriptions must remain inside content-sized rows',
  );
  assert.ok(
    skillLayout.listScrollHeight > skillLayout.listClientHeight &&
      skillLayout.overflowY === 'auto',
    'Long skill catalogs must scroll inside their own list',
  );
  assert.deepEqual(
    immediateTabs.tabs.map((tab) => tab.selected),
    ['false', 'true'],
    'Skills must become the active tab',
  );
  assert.equal(
    immediateTabs.tabs[1].background,
    immediateTabs.expectedSelected,
    'The newly selected tab must be visibly selected immediately',
  );
  assert.notEqual(
    immediateTabs.tabs[0].background,
    immediateTabs.expectedSelected,
    'The previous tab must stop looking selected immediately',
  );
  await page.getByRole('tab', { name: 'Skills' }).focus();
  await page.keyboard.press('ArrowLeft');
  assert.deepEqual(
    (await tabState()).tabs.map((tab) => tab.selected),
    ['true', 'false'],
    'Arrow keys must move selection between Assistant settings tabs',
  );
  await page.getByRole('tab', { name: 'Skills' }).click();
  await page.waitForTimeout(220);
  const skillsWideImage = 'assistant-dialog-skills-light-1280.png';
  await page.screenshot({ path: resolve(output, skillsWideImage) });
  const narrowPage = await context.newPage();
  narrowPage.on('pageerror', (error) =>
    errors.push(error.stack || error.message),
  );
  await narrowPage.setViewportSize({ width: 320, height: 720 });
  await narrowPage.goto(origin);
  await narrowPage.waitForFunction(
    () => globalThis.__designShell?.selectedAssistant === 'runtime-design',
  );
  await narrowPage.evaluate(() =>
    globalThis.__designNavigate('selectSession', 'session-design'),
  );
  await narrowPage.getByText('Review complete', { exact: true }).waitFor();
  await narrowPage.getByRole('button', { name: 'Bot settings' }).click();
  const narrowDialogLocator = narrowPage.getByRole('dialog', {
    name: 'Assistant settings',
  });
  await narrowDialogLocator.waitFor();
  await narrowPage.getByRole('tab', { name: 'Skills' }).click();
  await narrowPage.waitForTimeout(220);
  const narrowDialog = await narrowDialogLocator.evaluate((element) => {
    const grid = element.querySelector('.assistant-settings-grid');
    const card = element.querySelector('.ui-modal-card');
    const footer = element.querySelector('.ui-modal-footer');
    return {
      dialogWidth: element.getBoundingClientRect().width,
      documentWidth: document.documentElement.scrollWidth,
      columns: grid ? getComputedStyle(grid).gridTemplateColumns : '',
      cardBottom: card?.getBoundingClientRect().bottom,
      footerBottom: footer?.getBoundingClientRect().bottom,
      viewportHeight: innerHeight,
    };
  });
  assert.ok(
    narrowDialog.dialogWidth <= 320,
    'Dialog must fit a 320px viewport',
  );
  assert.equal(
    narrowDialog.documentWidth,
    320,
    'Dialog must not cause overflow',
  );
  assert.equal(
    narrowDialog.columns.split(' ').length,
    1,
    'Skills must use one column on a 320px viewport',
  );
  assert.ok(
    narrowDialog.footerBottom <= narrowDialog.viewportHeight,
    'The modal footer must remain visible in a 320px viewport',
  );
  assert.ok(
    narrowDialog.footerBottom <= narrowDialog.cardBottom,
    'The modal body must scroll behind a stable footer',
  );
  const skillsNarrowImage = 'assistant-dialog-skills-light-320.png';
  const cdp = await narrowPage.context().newCDPSession(narrowPage);
  const captureSkills = await cdp.send('Page.captureScreenshot', {
    format: 'png',
    fromSurface: true,
    captureBeyondViewport: false,
  });
  await writeFile(
    resolve(output, skillsNarrowImage),
    Buffer.from(captureSkills.data, 'base64'),
  );
  await narrowPage.getByRole('tab', { name: 'AGENTS.md' }).click();
  await narrowPage.evaluate(
    () =>
      new Promise((resolve) =>
        requestAnimationFrame(() => requestAnimationFrame(resolve)),
      ),
  );
  const agentsNarrowImage = 'assistant-dialog-agents-light-320.png';
  const captureAgents = await cdp.send('Page.captureScreenshot', {
    format: 'png',
    fromSurface: true,
    captureBeyondViewport: false,
  });
  await writeFile(
    resolve(output, agentsNarrowImage),
    Buffer.from(captureAgents.data, 'base64'),
  );
  await narrowPage.close();
  const feedbackPage = await context.newPage();
  feedbackPage.on('pageerror', (error) =>
    errors.push(error.stack || error.message),
  );
  await feedbackPage.setViewportSize({ width: 920, height: 557 });
  await feedbackPage.goto(origin);
  await feedbackPage.waitForFunction(
    () => globalThis.__designShell?.selectedAssistant === 'runtime-design',
  );
  await feedbackPage.evaluate(() =>
    globalThis.__designNavigate('selectSession', 'session-design'),
  );
  await feedbackPage.getByText('Review complete', { exact: true }).waitFor();
  await feedbackPage.getByRole('textbox', { name: 'Message' }).focus();
  const composerFeedbackImage = 'composer-focus-light-920.png';
  await feedbackPage.screenshot({
    path: resolve(output, composerFeedbackImage),
  });
  const feedbackDisclosure = feedbackPage
    .locator('.turn-process .ui-disclosure-trigger')
    .first();
  const feedbackDisclosureBox = await feedbackDisclosure.boundingBox();
  assert.ok(feedbackDisclosureBox, 'Feedback disclosure must be visible');
  await feedbackPage.mouse.move(
    feedbackDisclosureBox.x + feedbackDisclosureBox.width / 2,
    feedbackDisclosureBox.y + feedbackDisclosureBox.height / 2,
  );
  await feedbackPage.mouse.down();
  const disclosureFeedbackImage = 'disclosure-pressed-light-920.png';
  await feedbackPage.screenshot({
    path: resolve(output, disclosureFeedbackImage),
  });
  await feedbackPage.mouse.up();
  await feedbackPage.getByRole('button', { name: 'Bot settings' }).click();
  await feedbackPage.getByRole('tab', { name: 'Skills' }).click();
  await feedbackPage
    .getByRole('textbox', { name: 'Import skill text' })
    .focus();
  await feedbackPage.waitForTimeout(220);
  const assistantFeedbackImage = 'assistant-dialog-skills-focus-light-920.png';
  await feedbackPage.screenshot({
    path: resolve(output, assistantFeedbackImage),
  });
  await feedbackPage
    .getByRole('button', { name: 'Close assistant settings' })
    .click();
  await feedbackPage.evaluate(() =>
    globalThis.__designNavigate('selectSettingsSection', 'Runtime'),
  );
  await feedbackPage
    .getByRole('heading', { name: 'Runtime', exact: true })
    .waitFor();
  const runtimeAlignment = await feedbackPage.evaluate(() => {
    const select = document.querySelector('.desktop-inline select');
    const input = document.querySelector('.desktop-inline input');
    const action = [...document.querySelectorAll('button')].find((button) =>
      button.textContent?.includes('List current account'),
    );
    const rect = (element) => element?.getBoundingClientRect().toJSON();
    return {
      select: rect(select),
      input: rect(input),
      action: rect(action),
    };
  });
  assert.ok(
    Math.abs(runtimeAlignment.select.height - runtimeAlignment.input.height) <
      1,
  );
  assert.equal(runtimeAlignment.action.height, runtimeAlignment.input.height);
  assert.ok(
    Math.abs(runtimeAlignment.action.bottom - runtimeAlignment.input.bottom) <
      0.5,
    'Inline form actions must align to input control geometry',
  );
  const secondaryRuntime = feedbackPage
    .locator('.desktop-runtime')
    .filter({ hasText: 'Secondary design assistant' });
  await secondaryRuntime.getByRole('button', { name: 'Use Runtime' }).click();
  await secondaryRuntime.getByRole('button', { name: 'Selected' }).waitFor();
  assert.equal(
    await secondaryRuntime.getAttribute('data-selected'),
    'true',
    'The bound Runtime row must project the authoritative selected state',
  );
  const runtimeAlignmentImage = 'settings-runtime-alignment-light-920.png';
  await feedbackPage.screenshot({
    path: resolve(output, runtimeAlignmentImage),
  });
  await feedbackPage.close();
  await page.getByRole('button', { name: 'Close assistant settings' }).click();
  await dialog.waitFor({ state: 'detached' });
  assert.deepEqual(
    await chatRect(),
    beforeDialog,
    'Closing Assistant settings must preserve Chat geometry',
  );
  assert.equal(
    await chatLauncher.evaluate(
      (element) => document.activeElement === element,
    ),
    true,
    'Closing Assistant settings must restore focus to its launcher',
  );
  await page.evaluate(() =>
    globalThis.__designNavigate('selectSettingsSection', 'Assistant'),
  );
  await page.getByRole('heading', { name: 'Assistant', exact: true }).waitFor();
  await page.getByRole('button', { name: 'Open assistant settings' }).click();
  await dialog.waitFor();
  assert.equal(
    await page.locator('body > .ui-modal-layer').count(),
    1,
    'The Settings launcher must reuse the body-portal dialog',
  );
  await page.getByRole('button', { name: 'Close assistant settings' }).click();
  await dialog.waitFor({ state: 'detached' });
  const assistantDialog = {
    beforeDialog,
    wideDialog,
    immediateTabs,
    skillLayout,
    assistantControlFocus,
    narrowDialog,
    settingsLauncherUsesPortal: true,
    images: [
      agentsWideImage,
      skillsWideImage,
      agentsNarrowImage,
      skillsNarrowImage,
    ],
    pass: true,
  };
  const visualFeedback = {
    composerFocus,
    chatPersonaRemoved: true,
    disclosurePressedTransform,
    disclosureChevronMotion,
    images: [
      composerFeedbackImage,
      disclosureFeedbackImage,
      assistantFeedbackImage,
      runtimeAlignmentImage,
    ],
    runtimeAlignment,
  };
  return { assistantDialog, visualFeedback };
}
