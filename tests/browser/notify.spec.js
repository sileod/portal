const { test, expect } = require('@playwright/test');

test('notification settings load from and save to the hub', async ({ page }) => {
  let saved = null;
  await page.route('/api/notify', async route => {
    if (route.request().method() === 'POST') saved = JSON.parse(route.request().postData());
    await route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify(saved ? { ...saved, last_sent: saved.test ? 1700000000 : 0 } : {
        server: 'https://ntfy.sh', topic: '', waiting: true, finished: true, idle: false, hosts: false,
        min_work_seconds: 30, idle_minutes: 10,
      }),
    });
  });
  await page.goto('/');
  await page.locator('#portalNav').click();
  await expect(page.locator('#notifyWaiting')).toBeChecked();
  await expect(page.locator('#notifyMinWork')).toHaveValue('30');
  await expect(page.locator('#notifyStatus')).toHaveText('Notifications are off');
  await page.locator('#notifyTopic').fill('portal-751');
  await page.locator('#notifyIdle').check();
  await page.locator('#notifyIdleMinutes').fill('5');
  await page.locator('#notifyTest').click();
  await expect.poll(() => saved).toMatchObject({
    test: true, topic: 'portal-751', idle: true, idle_minutes: 5, finished: true, base_url: 'http://127.0.0.1:18082',
  });
  await expect(page.locator('#notifyStatus')).toContainText('Last sent');
});

test('sidebar tools stay fully visible in the narrowest vertical tab bar', async ({ page }) => {
  await page.addInitScript(() => { localStorage.portalSidebarWidth = '140' });
  await page.goto('/');
  await expect(page.locator('#rename')).toBeVisible();
  const clipped = await page.evaluate(() => [...document.querySelectorAll('.tool')]
    .filter(b => !b.hidden && b.scrollWidth > b.clientWidth + 1).map(b => b.id));
  expect(clipped).toEqual([]);
});
