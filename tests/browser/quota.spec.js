const { test, expect } = require('@playwright/test');

test('harness quotas appear in the left box and host settings only when available', async ({ page }) => {
  const resetIn = minutes => new Date(Date.now() + minutes * 60000).toISOString();
  let quotas = {
    portal: {
      codex: { ok: true, windows: [
        { name: '5h', remaining: 78, reset_at: resetIn(120) },
        { name: '7d', remaining: 75, reset_at: resetIn(4 * 1440) },
      ] },
      claude: { ok: true, windows: [
        { name: '5h', remaining: 94, reset_at: resetIn(30) },
        { name: '7d', remaining: 31, reset_at: resetIn(1440) },
      ] },
      agy: { ok: true, windows: [
        { name: 'Gemini Models / weekly', remaining: 0, reset_at: resetIn(3 * 1440) },
        { name: 'Claude and GPT models / weekly', remaining: 60, reset_at: resetIn(5 * 1440) },
        { name: 'Claude and GPT models / 5h', remaining: 100, reset_at: resetIn(240) },
      ] },
      opencode: { ok: false, error: 'no OpenCode login found' },
    },
    remote: { codex: { ok: false, error: 'not configured' } },
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
  await expect(page.locator('#quotaSidebar')).toContainText('Harness quotas · 5h / Week');
  await expect(page.locator('#quotaSidebar')).not.toContainText('remote');
  await expect(page.locator('#quotaSidebar')).not.toContainText('unavailable');
  const rows = page.locator('#quotaSidebar .quotasidebarrow');
  await expect(rows).toHaveCount(3);
  await expect(rows.nth(0)).toHaveText('portal · Codex78%·2h / 75%·4d');
  await expect(rows.nth(1)).toHaveText('portal · Claude94%·30m / 31%·1d');
  await expect(rows.nth(2)).toHaveText('portal · Antigravity– / 0%·3d');
  await expect(rows.nth(2)).toHaveAttribute('title', /Gemini Models \/ weekly: 0% left/);
  expect(await rows.nth(0).locator('span').last().evaluate(el => el.scrollWidth <= el.clientWidth)).toBe(true);
  expect(await page.locator('#quotaSidebar').evaluate(el => el.nextElementSibling.id)).toBe('portalNav');

  await page.locator('#portalNav').click();
  await expect(page.locator('#sessionOverview .quotarow', { hasText: 'Codex' })).toHaveText('Codex78%·2h / 75%·4d');
  await expect(page.locator('#sessionOverview .quotarow', { hasText: 'Antigravity' })).toHaveText('Antigravity– / 0%·3d');
  await expect(page.locator('#sessionOverview')).not.toContainText('OpenCode');
  await expect(page.locator('#sessionOverview .quotarow', { hasText: 'unavailable' })).toHaveCount(0);

  await page.locator('#setQuotaVisibility').selectOption('hide');
  await expect(page.locator('#quotaSidebar')).toBeEmpty();
  await expect(page.locator('#sessionOverview')).toContainText('Codex78%·2h / 75%·4d');
  await page.reload();
  await expect(page.locator('#quotaSidebar')).toBeEmpty();
  await page.locator('#portalNav').click();
  await expect(page.locator('#setQuotaVisibility')).toHaveValue('hide');
  await page.locator('#setQuotaVisibility').selectOption('show');
  await expect(page.locator('#quotaSidebar')).toContainText('portal · Codex78%·2h / 75%·4d');

  quotas = {};
  await expect(page.locator('#quotaSidebar')).toBeEmpty();
  await expect(page.locator('#sessionOverview')).not.toContainText('Harness quotas');
});
