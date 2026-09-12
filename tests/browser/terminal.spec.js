const { test, expect } = require('@playwright/test');

test('terminal fits its viewport and copies selections', async ({ page, browserName }) => {
  const pageErrors = [];
  page.on('pageerror', error => pageErrors.push(error.message));

  await page.goto('/');
  await page.locator('.tab').click();
  const screen = page.locator('.xterm-screen');
  await expect(screen).toBeVisible();
  await expect.poll(() => page.evaluate(() => (
    activePortalTerminal()?.term.buffer.active.getLine(0)?.translateToString(true) || ''
  ))).toContain('Portal clipboard fixture text');
  await expect.poll(() => page.evaluate(() => activePortalTerminal()?.term.modes.mouseTrackingMode)).toBe('drag');

  const fitRatios = () => page.evaluate(() => {
    const terminal = document.querySelector('.term.active').getBoundingClientRect();
    const screen = document.querySelector('.xterm-screen').getBoundingClientRect();
    return {
      widthRatio: screen.width / terminal.width,
      heightRatio: screen.height / terminal.height
    };
  });
  await expect.poll(async () => (await fitRatios()).widthRatio).toBeGreaterThan(0.9);
  await expect.poll(async () => (await fitRatios()).heightRatio).toBeGreaterThan(0.9);
  expect(pageErrors).toEqual([]);

  const box = await screen.boundingBox();
  await page.mouse.move(box.x + 8, box.y + 8);
  await page.mouse.down();
  await page.mouse.move(box.x + 245, box.y + 8, { steps: 12 });
  await page.mouse.up();
  const pointerSelection = await page.evaluate(() => portalSelection());
  expect(pointerSelection).toContain('clipboard fixture text');
  await expect(page.locator('.xterm-selection > div')).not.toHaveCount(0);
  await page.waitForTimeout(1700);
  expect(await page.evaluate(() => portalSelection())).toBe(pointerSelection);

  await page.evaluate(() => {
    window.portalObservedCopies = [];
    document.addEventListener('copy', event => {
      window.portalObservedCopies.push(event.clipboardData?.getData('text/plain') || '');
    });
  });
  await page.locator('#copy').click();
  await expect.poll(() => page.evaluate(() => window.portalObservedCopies.at(-1))).toBe(pointerSelection);
  if (browserName === 'chromium') {
    await expect.poll(() => page.evaluate(() => navigator.clipboard.readText())).toBe(pointerSelection);
  }

  const keyboardSelection = await page.evaluate(() => {
    const terminal = activePortalTerminal().term;
    terminal.select(0, 1, 21);
    return terminal.getSelection();
  });
  await page.keyboard.press('Control+c');
  await expect.poll(() => page.evaluate(() => window.portalObservedCopies.at(-1))).toBe(keyboardSelection);
  if (browserName === 'chromium') {
    await expect.poll(() => page.evaluate(() => navigator.clipboard.readText())).toBe(keyboardSelection);
  }
});
