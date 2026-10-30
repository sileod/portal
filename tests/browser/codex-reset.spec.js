const { test, expect } = require('@playwright/test');

test.use({ timezoneId: 'America/Los_Angeles' });

async function resetFixture(page, resetAt) {
  await page.route('/api/sessions', route => route.fulfill({ contentType: 'application/json', body: JSON.stringify({
    host_count: 1, hosts: ['test-host'], sessions: [{ host: 'test-host', session: 'codex', codex_reset_at: resetAt }],
  }) }));
  await page.goto('/');
  await expect(page.locator('#schedule')).toBeVisible();
}

test('schedule preloads proceed one minute after the detected Codex reset', async ({ page }) => {
  const resetAt = Math.floor(Date.now() / 1000) + 7200;
  let scheduled;
  await page.route('/api/session', async route => { scheduled = route.request().postDataJSON(); await route.fulfill({ status: 204 }); });
  await resetFixture(page, resetAt);
  await expect(page.locator('#schedule')).toHaveAttribute('title', /Codex reset \+ 1 minute/);
  const prompts = [];
  page.on('dialog', async dialog => {
    prompts.push({ message: dialog.message(), value: dialog.defaultValue() });
    await dialog.accept(dialog.defaultValue());
  });
  await page.locator('#schedule').click();
  await expect.poll(() => scheduled).toBeTruthy();
  expect(prompts[0].message).toContain('Detected Codex reset');
  expect(prompts[0].value).toMatch(/^\d+s$/);
  expect(prompts[1].value).toBe('proceed');
  expect(scheduled.text).toBe('proceed');
  expect(scheduled.repeat).toBe(1);
  expect(Math.abs(Date.now() / 1000 + parseInt(scheduled.delay, 10) - resetAt - 60)).toBeLessThan(5);
});

test('the suggested delay remains editable', async ({ page }) => {
  let scheduled;
  await page.route('/api/session', async route => { scheduled = route.request().postDataJSON(); await route.fulfill({ status: 204 }); });
  await resetFixture(page, Math.floor(Date.now() / 1000) + 7200);
  page.on('dialog', dialog => dialog.accept(dialog.message().startsWith('First send') ? '10m' : dialog.defaultValue()));
  await page.locator('#schedule').click();
  await expect.poll(() => scheduled?.delay).toBe('10m');
});

for (const resetAt of [undefined, 1]) {
  test(`missing or expired reset (${resetAt}) keeps the normal schedule prompt`, async ({ page }) => {
    await resetFixture(page, resetAt);
    let firstPrompt;
    page.on('dialog', async dialog => { firstPrompt = dialog.defaultValue(); await dialog.dismiss(); });
    await page.locator('#schedule').click();
    expect(firstPrompt).toBe('5h');
    await expect(page.locator('#schedule')).not.toHaveAttribute('title', /Codex reset/);
  });
}
