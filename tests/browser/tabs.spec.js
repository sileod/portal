const { test, expect } = require('@playwright/test');

async function mockSessions(page, names) {
  const killed = [];
  await page.route('/api/sessions', route => route.fulfill({
    contentType: 'application/json',
    body: JSON.stringify({
      version: 'browser-test', host_count: 1, hosts: ['test-host'],
      sessions: names.filter(n => !killed.includes(n)).map(session => ({ host: 'test-host', session })),
    }),
  }));
  await page.route('/api/session', async route => {
    const body = route.request().postDataJSON();
    if (body.action === 'kill') killed.push(body.session);
    await route.fulfill({ status: 204 });
  });
  return killed;
}

const tabNames = page => page.locator('.tab .tabname').allTextContents();

test('dragging a tab switches to a persistent manual order', async ({ page }) => {
  await mockSessions(page, ['alpha', 'beta', 'gamma']);
  await page.goto('/');
  await expect.poll(() => tabNames(page)).toEqual(['alpha', 'beta', 'gamma']);
  const gamma = page.locator('.tabwrap', { hasText: 'gamma' });
  const alpha = page.locator('.tabwrap', { hasText: 'alpha' });
  await gamma.dragTo(alpha, { targetPosition: { x: 5, y: 5 } });
  await expect.poll(() => tabNames(page)).toEqual(['gamma', 'alpha', 'beta']);
  expect(await page.evaluate(() => localStorage.portalTabOrder)).toBe('manual');
  await page.reload();
  await expect.poll(() => tabNames(page)).toEqual(['gamma', 'alpha', 'beta']);
});

test('each tab has a close button that kills its session', async ({ page }) => {
  const killed = await mockSessions(page, ['alpha', 'beta']);
  await page.goto('/');
  await expect.poll(() => tabNames(page)).toEqual(['alpha', 'beta']);
  page.on('dialog', d => d.accept());
  const beta = page.locator('.tabwrap', { hasText: 'beta' });
  await beta.hover();
  await beta.locator('.tabclose').click();
  await expect.poll(() => killed).toEqual(['beta']);
  await expect.poll(() => tabNames(page)).toEqual(['alpha']);
});
