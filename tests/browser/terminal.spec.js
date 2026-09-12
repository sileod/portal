const { test, expect } = require('@playwright/test');

test('terminal selection does not hijack copying from the paste dialog', async ({ page, browserName }) => {
  test.skip(browserName !== 'chromium', 'Requires browser clipboard permission');
  await page.goto('/');
  await expect(page.locator('.xterm-screen')).toBeVisible();
  await expect.poll(() => page.evaluate(() => activePortalTerminal()?.term.buffer.active.getLine(0)?.translateToString(true))).toContain('Portal clipboard fixture text');
  await page.evaluate(() => {
    activePortalTerminal().term.select(0, 0, 6);
    promptPortalPaste();
  });
  await expect.poll(() => page.evaluate(() => portalSelection())).toBe('Portal');
  const input = page.locator('textarea').last();
  await input.fill('edited clipboard text');
  await input.selectText();
  await page.keyboard.press('Control+c');
  await expect.poll(() => page.evaluate(() => navigator.clipboard.readText())).toBe('edited clipboard text');
});

test('keyboard paste traverses a real isolated tmux PTY', async ({ page, browserName }) => {
  test.skip(browserName !== 'chromium', 'Requires browser clipboard permission');
  await page.addInitScript(() => {
    const Native = WebSocket;
    window.WebSocket = class extends Native {
      constructor(url, protocols) { super(url + '&fixture=tmux', protocols); }
    };
  });
  await page.goto('/');
  const content = () => page.evaluate(() => {
    const b = activePortalTerminal()?.term.buffer.active;
    return b ? Array.from({ length: b.length }, (_, i) => b.getLine(i)?.translateToString(true)).join('\n') : '';
  });
  await expect.poll(content).toContain('Portal tmux fixture text');
  await page.evaluate(() => navigator.clipboard.writeText('private synthetic tmux paste'));
  await page.locator('.xterm-screen').click();
  await page.keyboard.press('Control+v');
  await expect.poll(content).toContain('private synthetic tmux paste');
});

test('keyboard paste reaches the focused terminal', async ({ page, browserName }) => {
  test.skip(browserName !== 'chromium', 'Requires browser clipboard permission');
  await page.goto('/');
  await expect(page.locator('.xterm-screen')).toBeVisible();
  await expect.poll(() => page.evaluate(() => activePortalTerminal()?.term.modes.mouseTrackingMode)).toBe('drag');
  await page.evaluate(() => navigator.clipboard.writeText('Portal synthetic paste payload'));
  await page.locator('#newtab').focus();
  await page.locator('.xterm-screen').click();
  await expect(page.locator('.xterm-helper-textarea')).toBeFocused();
  await page.keyboard.press('Control+v');
  await expect.poll(() => page.evaluate(() => {
    const b = activePortalTerminal().term.buffer.active;
    return Array.from({ length: b.length }, (_, i) => b.getLine(i)?.translateToString(true)).join('\n');
  })).toContain('Portal paste round trip received');
});

test('terminal fits its viewport and copies selections', async ({ page, browserName }) => {
  const pageErrors = [];
  page.on('pageerror', error => pageErrors.push(error.message));

  await page.goto('/');
  const screen = page.locator('.xterm-screen');
  await expect(screen).toBeVisible();
  await expect.poll(() => page.evaluate(() => (
    activePortalTerminal()?.term.buffer.active.getLine(0)?.translateToString(true) || ''
  ))).toContain('Portal clipboard fixture text');
  await expect.poll(() => page.evaluate(() => activePortalTerminal()?.term.modes.mouseTrackingMode)).toBe('drag');

  const fitRatios = () => page.evaluate(() => {
    const terminal = document.querySelector('.term.active').getBoundingClientRect();
    const screen = document.querySelector('.xterm-screen').getBoundingClientRect();
    return {
      widthRatio: screen.width / terminal.width,
      heightRatio: screen.height / terminal.height
    };
  });
  await expect.poll(async () => (await fitRatios()).widthRatio).toBeGreaterThan(0.9);
  await expect.poll(async () => (await fitRatios()).heightRatio).toBeGreaterThan(0.9);
  expect(pageErrors).toEqual([]);

  const box = await screen.boundingBox();
  await page.mouse.move(box.x + 8, box.y + 8);
  await page.mouse.down();
  await page.mouse.move(box.x + 245, box.y + 8, { steps: 12 });
  await page.mouse.up();
  const pointerSelection = await page.evaluate(() => portalSelection());
  expect(pointerSelection).toContain('clipboard fixture text');
  await expect(page.locator('.xterm-selection > div')).not.toHaveCount(0);
  await page.waitForTimeout(1700);
  expect(await page.evaluate(() => portalSelection())).toBe(pointerSelection);

  await page.evaluate(() => activePortalTerminal().term.clearSelection());
  await page.keyboard.down('Alt');
  await page.mouse.move(box.x + 8, box.y + 8);
  await page.mouse.down();
  await page.mouse.move(box.x + 245, box.y + 8, { steps: 12 });
  await page.mouse.up();
  await page.keyboard.up('Alt');
  expect(await page.evaluate(() => portalSelection())).toContain('clipboard fixture text');

  await page.evaluate(() => {
    window.portalObservedCopies = [];
    document.addEventListener('copy', event => {
      window.portalObservedCopies.push(event.clipboardData?.getData('text/plain') || '');
    });
  });
  await page.locator('#copy').click();
  await expect.poll(() => page.evaluate(() => window.portalObservedCopies.at(-1))).toBe(pointerSelection);
  if (browserName === 'chromium') {
    await expect.poll(() => page.evaluate(() => navigator.clipboard.readText())).toBe(pointerSelection);
  }

  const keyboardSelection = await page.evaluate(() => {
    const terminal = activePortalTerminal().term;
    terminal.select(0, 1, 21);
    return terminal.getSelection();
  });
  await page.keyboard.press('Control+c');
  await expect.poll(() => page.evaluate(() => window.portalObservedCopies.at(-1))).toBe(keyboardSelection);
  if (browserName === 'chromium') {
    await expect.poll(() => page.evaluate(() => navigator.clipboard.readText())).toBe(keyboardSelection);
  }

  if (browserName === 'chromium') {
    await page.evaluate(() => navigator.clipboard.writeText('Portal synthetic paste payload'));
    await page.locator('#paste').click();
  } else {
    await page.evaluate(() => {
      const event = new Event('paste', { bubbles: true, cancelable: true });
      Object.defineProperty(event, 'clipboardData', {
        value: { getData: type => type === 'text/plain' ? 'Portal synthetic paste payload' : '' }
      });
      document.dispatchEvent(event);
    });
  }
  await expect.poll(() => page.evaluate(() => {
    const terminal = activePortalTerminal().term;
    const lines = [];
    for (let row = 0; row < terminal.buffer.active.length; row++) {
      lines.push(terminal.buffer.active.getLine(row)?.translateToString(true) || '');
    }
    return lines.join('\n');
  })).toContain('Portal paste round trip received');

  await page.evaluate(() => activePortalTerminal().term.clearSelection());
  await page.keyboard.down('Shift');
  await page.mouse.move(box.x + 8, box.y + 8);
  await page.mouse.down();
  await page.mouse.move(box.x + 245, box.y + 8, { steps: 12 });
  await page.mouse.up();
  await page.keyboard.up('Shift');
  expect(await page.evaluate(() => portalSelection())).toContain('clipboard fixture text');

  await page.evaluate(() => new Promise(resolve => {
    activePortalTerminal().term.write('\x1b[?1000l\x1b[?1002l\x1b[?1006l', resolve);
  }));
  await expect.poll(() => page.evaluate(() => activePortalTerminal().term.modes.mouseTrackingMode)).toBe('none');
  await page.mouse.move(box.x + 8, box.y + 8);
  await page.mouse.down();
  await page.mouse.move(box.x + 245, box.y + 8, { steps: 12 });
  await page.mouse.up();
  expect(await page.evaluate(() => portalSelection())).toContain('clipboard fixture text');
});
