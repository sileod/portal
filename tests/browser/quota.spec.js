const { test, expect } = require('@playwright/test');

test('harness quotas appear in the left box and host settings only when available', async ({ page }) => {
  let quotas = {
    portal: {
      codex: { ok: true, windows: [
        { name: '5h', remaining: 78, reset_at: '2026-09-28T18:00:00Z' },
        { name: '7d', remaining: 75 },
      ] },
      opencode: { ok: false, error: 'no OpenCode login found' },
    },
  };
  await page.addInitScript(() => { localStorage.portalOrientation = 'vertical'; });
  await page.route('/api/sessions', route => route.fulfill({
    contentType: 'application/json',
    body: JSON.stringify({
      version: 'browser-test', host_count: 2, hosts: ['portal', 'remote'], sessions: [],
      host_quotas: quotas,
    }),
  }));
  await page.goto('/');
  await expect(page.locator('#quotaSidebar')).toContainText('Harness quotas');
  await expect(page.locator('#quotaSidebar')).toContainText('portal · Codex 5h');
  await expect(page.locator('#quotaSidebar')).toContainText('78% left');
  await expect(page.locator('#quotaSidebar')).not.toContainText('remote');

  await page.locator('#portalNav').click();
  await expect(page.locator('#sessionOverview')).toContainText('Codex · 7d');
  await expect(page.locator('#sessionOverview')).toContainText('OpenCode');
  await expect(page.locator('#sessionOverview .quotarow', { hasText: 'OpenCode' }))
    .toHaveAttribute('title', 'no OpenCode login found');

  quotas = {};
  await expect(page.locator('#quotaSidebar')).toBeEmpty();
  await expect(page.locator('#sessionOverview')).not.toContainText('Harness quotas');
});
