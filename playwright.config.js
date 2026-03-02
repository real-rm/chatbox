// @ts-check
import { defineConfig, devices } from '@playwright/test';

// API server URL (real Go backend - requires docker-compose up)
const API_BASE_URL = process.env.API_BASE_URL || 'http://localhost:8080';
// Static file server URL (serves web/ for frontend tests)
const STATIC_URL = 'http://localhost:3333';

/**
 * @see https://playwright.dev/docs/test-configuration
 */
export default defineConfig({
  testDir: './tests',
  fullyParallel: true,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 2 : 0,
  workers: process.env.CI ? 1 : undefined,
  reporter: [
    ['html'],
    ['list'],
  ],

  use: {
    trace: 'on-first-retry',
    screenshot: 'only-on-failure',
  },

  projects: [
    // ── Frontend tests (mocked APIs, served via static server) ──
    {
      name: 'frontend-chromium',
      testDir: './tests/frontend',
      use: {
        ...devices['Desktop Chrome'],
        baseURL: STATIC_URL,
      },
    },
    {
      name: 'frontend-firefox',
      testDir: './tests/frontend',
      use: {
        ...devices['Desktop Firefox'],
        baseURL: STATIC_URL,
      },
    },
    {
      name: 'frontend-webkit',
      testDir: './tests/frontend',
      use: {
        ...devices['Desktop Safari'],
        baseURL: STATIC_URL,
      },
    },

    // ── API tests (real server, no browser needed) ──
    {
      name: 'api',
      testDir: './tests/api',
      use: {
        baseURL: API_BASE_URL,
      },
    },
  ],

  webServer: {
    command: 'node tests/static-server.js',
    url: STATIC_URL,
    reuseExistingServer: !process.env.CI,
    timeout: 10_000,
  },
});
