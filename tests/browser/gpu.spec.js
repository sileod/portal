const { test, expect } = require('@playwright/test');

test('available GPU memory leads in the sidebar and host settings', async ({ page }) => {
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
  await expect(page.locator('#gpuSidebar')).toContainText('GPU memory available');
  await expect(page.locator('#gpuSidebar')).toContainText('portal · GPU 0');
  await expect(page.locator('#gpuSidebar')).toContainText('21.3 GiB free');
  await expect(page.locator('#gpuSidebar .gpusidebarrow')).toHaveAttribute('title', /42% utilization/);
  await page.locator('#portalNav').click();
  await expect(page.locator('#sessionOverview')).toContainText('NVIDIA A30');
  await expect(page.locator('#sessionOverview')).toContainText('21.3 / 24.0 GiB free · 42% GPU');

  gpus = [];
  await expect(page.locator('#sessionOverview')).toContainText('GPU memory unavailable');
  await expect(page.locator('#gpuSidebar')).toBeEmpty();
});
