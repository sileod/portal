const { test, expect } = require('@playwright/test');

test('a planned message can be canceled from the Portal panel', async ({ page }) => {
  await page.goto('/');
  await page.evaluate(() => fetch('/api/test/enable-schedule', { method: 'POST' }));
  page.on('dialog', dialog => dialog.accept());
  await page.reload();
  await page.locator('#portalNav').click();
  const row = page.locator('.schedulerow');
  await expect(row).toContainText('synthetic planned message');
  await row.getByRole('button', { name: 'Cancel' }).click();
  await expect(page.locator('.scheduleempty')).toHaveText('No pending scheduled messages.');
  await expect(page.locator('#toast')).toContainText('Canceled scheduled message');
});

test('only the next occurrence of a repeating message can be skipped', async ({ page }) => {
  await page.goto('/');
  await page.evaluate(() => fetch('/api/test/enable-schedule', { method: 'POST' }));
  await page.reload();
  await page.locator('#portalNav').click();
  const row = page.locator('.schedulerow');
  const skip = row.getByRole('button', { name: 'Skip next' });
  await expect(skip).toBeEnabled();
  await skip.click();
  await expect(row).toContainText('next skipped');
  await expect(skip).toBeDisabled();
  await expect(row).toContainText('synthetic planned message');
  await expect(page.locator('#toast')).toContainText('Next scheduled message skipped');
});
