// Browser tests for `authama serve`. The global setup builds a throwaway home and
// starts the server against it; see support/sandbox.ts. Run with `make e2e-web`.
import { defineConfig, devices } from '@playwright/test';

export default defineConfig({
  testDir: './tests',
  globalSetup: './support/sandbox.ts',
  // One server and one home: the write flows change it, so nothing runs at once.
  workers: 1,
  fullyParallel: false,
  forbidOnly: !!process.env.CI,
  retries: 0,
  timeout: 60_000,
  reporter: process.env.CI ? [['list'], ['html', { open: 'never' }]] : [['list']],
  use: {
    // Set by the global setup, which runs before the config's `use` is read by a worker.
    baseURL: process.env.AUTHAMA_E2E_URL,
    trace: 'retain-on-failure',
    screenshot: 'only-on-failure',
  },
  projects: [{ name: 'chromium', use: { ...devices['Desktop Chrome'] } }],
});
