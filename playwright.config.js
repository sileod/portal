const { defineConfig } = require('@playwright/test');

module.exports = defineConfig({
  testDir: './tests/browser',
  fullyParallel: false,
  retries: process.env.CI ? 1 : 0,
  reporter: process.env.CI ? 'github' : 'line',
  use: {
    baseURL: 'http://127.0.0.1:18082',
    permissions: ['clipboard-read', 'clipboard-write'],
    screenshot: 'only-on-failure',
    trace: 'retain-on-failure'
  },
  webServer: {
    command: 'go run ./internal/webui/testdata/server',
    url: 'http://127.0.0.1:18082/healthz',
    reuseExistingServer: false,
    timeout: 120000
  },
  projects: [{ name: 'chromium', use: { browserName: 'chromium' } }]
});
