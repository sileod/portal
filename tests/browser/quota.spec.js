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
        { name: 'Claude and GPT models / weekly', remaining: 60 },
        { name: 'Claude and GPT models / 5h', remaining: 100 },
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
  await expect(page.locator('#quotaSidebar')).toContainText('portal · Codex 7d');
  await expect(page.locator('#quotaSidebar')).toContainText('portal · Claude 5h');
  await expect(page.locator('#quotaSidebar')).toContainText('portal · Claude 7d');
  await expect(page.locator('#quotaSidebar')).toContainText('Antigravity Claude and GPT models / weekly');
  await expect(page.locator('#quotaSidebar')).toContainText('Antigravity Claude and GPT models / 5h');
  await expect(page.locator('#quotaSidebar')).not.toContainText('remote');
  expect(await page.locator('#quotaSidebar').evaluate(el => el.nextElementSibling.id)).toBe('portalNav');

  await page.locator('#portalNav').click();
  await expect(page.locator('#sessionOverview')).toContainText('Codex · 7d');
  await expect(page.locator('#sessionOverview')).toContainText('OpenCode');
  await expect(page.locator('#sessionOverview .quotarow', { hasText: 'OpenCode' }))
    .toHaveAttribute('title', 'no OpenCode login found');

  await page.locator('#setQuotaVisibility').selectOption('hide');
  await expect(page.locator('#quotaSidebar')).toBeEmpty();
  await expect(page.locator('#sessionOverview')).toContainText('Codex · 7d');
  await page.reload();
  await expect(page.locator('#quotaSidebar')).toBeEmpty();
  await page.locator('#portalNav').click();
  await expect(page.locator('#setQuotaVisibility')).toHaveValue('hide');
  await page.locator('#setQuotaVisibility').selectOption('show');
  await expect(page.locator('#quotaSidebar')).toContainText('portal · Codex 5h');

  quotas = {};
  await expect(page.locator('#quotaSidebar')).toBeEmpty();
  await expect(page.locator('#sessionOverview')).not.toContainText('Harness quotas');
});
