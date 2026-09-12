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
