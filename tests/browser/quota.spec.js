const { test, expect } = require('@playwright/test');

test('harness quotas appear in the left box and host settings only when available', async ({ page }) => {
  let quotas = {
    portal: {
      codex: { ok: true, windows: [
        { name: '5h', remaining: 78, reset_at: '2026-09-28T18:00:00Z' },
        { name: '7d', remaining: 75 },
      ] },
      claude: { ok: true, windows: [
        { name: '5h', remaining: 94 },
        { name: '7d', remaining: 31 },
      ] },
      agy: { ok: true, windows: [
        { name: 'Gemini Models / weekly', remaining: 0 },
        { name: 'Claude and GPT models / weekly', remaining: 60 },
        { name: 'Claude and GPT models / 5h', remaining: 100 },
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
  await expect(rows.nth(0)).toHaveText('portal · Codex78% / 75%');
  await expect(rows.nth(1)).toHaveText('portal · Claude94% / 31%');
  await expect(rows.nth(2)).toHaveText('portal · Antigravity– / 0%');
  await expect(rows.nth(2)).toHaveAttribute('title', /Gemini Models \/ weekly: 0% left/);
  expect(await page.locator('#quotaSidebar').evaluate(el => el.nextElementSibling.id)).toBe('portalNav');

  await page.locator('#portalNav').click();
  await expect(page.locator('#sessionOverview .quotarow', { hasText: 'Codex' })).toHaveText('Codex78% / 75%');
  await expect(page.locator('#sessionOverview .quotarow', { hasText: 'Antigravity' })).toHaveText('Antigravity– / 0%');
  await expect(page.locator('#sessionOverview')).not.toContainText('OpenCode');
  await expect(page.locator('#sessionOverview .quotarow', { hasText: 'unavailable' })).toHaveCount(0);

  await page.locator('#setQuotaVisibility').selectOption('hide');
  await expect(page.locator('#quotaSidebar')).toBeEmpty();
  await expect(page.locator('#sessionOverview')).toContainText('Codex78% / 75%');
  await page.reload();
  await expect(page.locator('#quotaSidebar')).toBeEmpty();
  await page.locator('#portalNav').click();
  await expect(page.locator('#setQuotaVisibility')).toHaveValue('hide');
  await page.locator('#setQuotaVisibility').selectOption('show');
  await expect(page.locator('#quotaSidebar')).toContainText('portal · Codex78% / 75%');

  quotas = {};
  await expect(page.locator('#quotaSidebar')).toBeEmpty();
  await expect(page.locator('#sessionOverview')).not.toContainText('Harness quotas');
});
