const { test, expect } = require('@playwright/test');

async function mockSessions(page, names) {
  const killed = [];
  await page.route('/api/sessions', route => route.fulfill({
    contentType: 'application/json',
    body: JSON.stringify({
      version: 'browser-test', host_count: 1, hosts: ['test-host'],
      sessions: (typeof names === 'function' ? names() : names).map(n => typeof n === 'string' ? { session: n } : n)
        .filter(s => !killed.includes(s.session)).map(s => ({ host: 'test-host', ...s })),
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

test('tab dots show which terminals need input, are working, or have news', async ({ page }) => {
  const now = Math.floor(Date.now() / 1000);
  let doneActivity = now - 60;
  await mockSessions(page, () => [
    { session: 'alpha' },
    { session: 'asking', state: 'waiting', last_activity: now },
    { session: 'busy', state: 'working', last_activity: now },
    { session: 'done', last_activity: doneActivity },
  ]);
  await page.goto('/');
  await expect.poll(() => tabNames(page)).toEqual(['alpha', 'asking', 'busy', 'done']);
  doneActivity = now;
  const dot = name => page.locator('.tabwrap', { hasText: name }).locator('.unread');
  await expect(dot('asking')).toHaveClass(/waiting/);
  await expect(dot('busy')).toHaveClass(/working/);
  await expect(dot('done')).toHaveAttribute('title', 'New output since you last looked');
  await expect(dot('alpha')).toHaveCount(0);
  await expect(page).toHaveTitle('(1) Portal');
});

test('the sort button cycles tab orders, starting with newest first', async ({ page }) => {
  await mockSessions(page, [
    { session: 'alpha', created: 100 },
    { session: 'beta', created: 300 },
    { session: 'gamma', created: 200 },
  ]);
  await page.goto('/');
  await expect.poll(() => tabNames(page)).toEqual(['alpha', 'beta', 'gamma']);
  await page.locator('#sortTabs').click();
  await expect.poll(() => tabNames(page)).toEqual(['beta', 'gamma', 'alpha']);
  await expect(page.locator('#sortTabs')).toHaveAttribute('title', /newest first/);
  expect(await page.evaluate(() => localStorage.portalTabOrder)).toBe('newest');
  await page.reload();
  await expect.poll(() => tabNames(page)).toEqual(['beta', 'gamma', 'alpha']);
  await page.locator('#sortTabs').click();
  await expect(page.locator('#sortTabs')).toHaveAttribute('title', /recent output/);
});

test('newest first falls back to when this browser first saw a terminal', async ({ page }) => {
  let names = ['alpha', 'beta'];
  await mockSessions(page, () => names);
  await page.addInitScript(() => { localStorage.portalTabOrder = 'newest' });
  await page.goto('/');
  await expect.poll(() => tabNames(page)).toEqual(['alpha', 'beta']);
  await page.waitForTimeout(1100);
  names = ['alpha', 'beta', 'aardvark'];
  await expect.poll(() => tabNames(page)).toEqual(['aardvark', 'alpha', 'beta']);
});
