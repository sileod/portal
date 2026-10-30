const { test, expect } = require('@playwright/test');

test('system theme and explicit overrides apply to the sidebar and panels', async ({ page }) => {
  await page.emulateMedia({ colorScheme: 'dark' });
  await page.addInitScript(() => { if (!localStorage.portalTheme) localStorage.portalTheme = 'system'; });
  await page.goto('/');
  await page.locator('#portalNav').click();
  const bar = page.locator('.bar');
  await expect(bar).toHaveCSS('background-color', 'rgb(24, 24, 24)');
  await expect(page.locator('#portalPanel')).toHaveCSS('background-color', 'rgb(17, 17, 17)');
  await page.emulateMedia({ colorScheme: 'light' });
  await expect(bar).toHaveCSS('background-color', 'rgb(245, 245, 245)');
  await page.locator('#setTheme').selectOption('dark');
  await expect(bar).toHaveCSS('background-color', 'rgb(24, 24, 24)');
  await page.reload();
  await expect(bar).toHaveCSS('background-color', 'rgb(24, 24, 24)');
  await page.locator('#portalNav').click();
  await page.locator('#setTheme').selectOption('light');
  await expect(bar).toHaveCSS('background-color', 'rgb(245, 245, 245)');
});

for (const orientation of ['vertical', 'horizontal']) {
  test(`phone navigation preserves ${orientation} preference and gives the stage full width`, async ({ page }) => {
    await page.setViewportSize({ width: 390, height: 844 });
    await page.addInitScript(value => { localStorage.portalOrientation = value; }, orientation);
    await page.goto('/');
    const menu = page.getByRole('button', { name: 'Open navigation', exact: true });
    await expect(menu).toBeVisible();
    await expect(page.locator('.bar')).toBeHidden();
    expect((await page.locator('#stage').boundingBox()).width).toBe(390);
    await menu.click();
    await expect(page.locator('.bar')).toBeVisible();
    await page.locator('#funnelNav').click();
    await expect(page.locator('#funnelPanel')).toBeVisible();
    await expect(page.locator('.bar')).toBeHidden();
    await menu.click();
    await page.locator('#portalNav').click();
    await expect(page.locator('#portalPanel')).toBeVisible();
    expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBe(390);
    await menu.click();
    await page.locator('#mobileBackdrop').click({ position: { x: 380, y: 300 } });
    await expect(page.locator('.bar')).toBeHidden();
    await page.setViewportSize({ width: 1280, height: 800 });
    await expect(menu).toBeHidden();
    await expect(page.locator('.bar')).toBeVisible();
    expect(await page.evaluate(() => localStorage.portalOrientation)).toBe(orientation);
    const desktopBar = await page.locator('.bar').boundingBox();
    expect(orientation === 'vertical' ? desktopBar.width : desktopBar.height).toBe(orientation === 'vertical' ? 220 : 40);
  });
}

test('phone terminal refits after a viewport height change and navigation', async ({ page }) => {
  await page.setViewportSize({ width: 320, height: 640 });
  await page.goto('/');
  await expect(page.locator('.term.active .xterm-screen')).toBeVisible();
  const fits = () => page.evaluate(() => {
    const terminal = document.querySelector('.term.active').getBoundingClientRect();
    const screen = document.querySelector('.term.active .xterm-screen').getBoundingClientRect();
    return screen.width > terminal.width * .9 && screen.width <= terminal.width && screen.height > terminal.height * .9 && screen.height <= terminal.height;
  });
  await expect.poll(fits).toBe(true);
  await page.setViewportSize({ width: 390, height: 360 });
  await expect.poll(fits).toBe(true);
  await page.locator('#mobileMenu').click();
  await page.locator('#portalNav').click();
  await page.locator('#mobileMenu').click();
  await page.locator('.tab').first().click();
  await expect(page.locator('.bar')).toBeHidden();
  await expect.poll(fits).toBe(true);
});
