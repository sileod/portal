const { test, expect } = require('@playwright/test');

test('GPU usage appears in the sidebar and host settings', async ({ page }) => {
  let gpus = [{ index: 0, name: 'NVIDIA A30', utilization: 42, memory_used: 2796, memory_total: 24576 }];
  await page.addInitScript(() => { localStorage.portalOrientation = 'vertical'; });
  await page.route('/api/sessions', route => route.fulfill({
    contentType: 'application/json',
    body: JSON.stringify({
      version: 'browser-test', host_count: 1, hosts: ['portal'], sessions: [],
      host_gpus: gpus.length ? { portal: gpus } : {},
    }),
  }));
  await page.goto('/');
  await expect(page.locator('#gpuSidebar')).toContainText('portal · GPU 0');
  await expect(page.locator('#gpuSidebar')).toContainText('42%');
  await page.locator('#portalNav').click();
  await expect(page.locator('#sessionOverview')).toContainText('NVIDIA A30');
  await expect(page.locator('#sessionOverview')).toContainText('2796/24576 MiB');

  gpus = [];
  await expect(page.locator('#sessionOverview')).toContainText('GPU usage unavailable');
  await expect(page.locator('#gpuSidebar')).toBeEmpty();
});
