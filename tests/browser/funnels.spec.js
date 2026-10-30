const { test, expect } = require('@playwright/test');

test('hosted apps tab below quotas shows safe links and updates when removed', async ({ page }) => {
  let funnels = { portal: [{ url: 'https://apps.example:10000/' }, { url: 'https://apps.example/opencode' }, { url: 'javascript:alert(1)' }] };
  await page.addInitScript(() => { localStorage.portalOrientation = 'vertical'; });
  await page.route('/api/sessions', route => route.fulfill({ contentType: 'application/json', body: JSON.stringify({
    host_count: 1, hosts: ['portal'], sessions: [], host_funnels: funnels,
  }) }));
  await page.goto('/');
  await page.locator('#funnelNav').click();
  const box = page.locator('#funnelRoutes');
  await expect(box).toBeVisible();
  await expect(box.locator('a')).toHaveCount(2);
  await expect(box.locator('a').last()).toHaveText('apps.example/opencode');
  await expect(box.locator('a').last()).toHaveAttribute('href', 'https://apps.example/opencode');
  await expect(box.locator('a').last()).toHaveAttribute('rel', 'noopener noreferrer');
  expect(await page.locator('#funnelNav').evaluate(el => el.previousElementSibling.id)).toBe('quotaSidebar');
  funnels = {};
  await expect(box).toContainText('No public Funnel routes found');
  await page.locator('#portalNav').click();
  await expect(page.locator('#funnelPanel')).toBeHidden();
  await expect(page.locator('#portalPanel')).toBeVisible();
});
