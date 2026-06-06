// @ts-check
const { defineConfig } = require('@playwright/test');

const defaultPort = '8391';
const configuredBaseURL = process.env.PLAYWRIGHT_BASE_URL;
const port = process.env.PLAYWRIGHT_PORT || (configuredBaseURL && new URL(configuredBaseURL).port) || defaultPort;
const baseURL = configuredBaseURL || `http://127.0.0.1:${port}`;

module.exports = defineConfig({
  testDir: './tests',
  outputDir: process.env.PLAYWRIGHT_OUTPUT_DIR || 'test-results',
  timeout: 30_000,
  retries: 0,
  use: {
    baseURL,
    headless: true,
    serviceWorkers: 'block',
  },
  webServer: {
    command: `../../bin/agent-dashboard-web --port ${port}`,
    url: baseURL,
    reuseExistingServer: process.env.PLAYWRIGHT_REUSE_SERVER === '1',
    timeout: 15_000,
  },
  projects: [
    {
      name: 'chromium',
      use: { browserName: 'chromium' },
    },
  ],
});
