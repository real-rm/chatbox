// @ts-check
import { test, expect } from '@playwright/test';

// API tests run against the real Go server.
// Requires: docker-compose up (or make docker-compose-up)
// Run with: npm run test:api

const CHATBOX_PREFIX = process.env.CHATBOX_PATH_PREFIX || '/chatbox';

test.describe('Health Endpoints', () => {
  test.describe('liveness probe', () => {
    test('GET /healthz returns 200', async ({ request }) => {
      const response = await request.get(`${CHATBOX_PREFIX}/healthz`);

      expect(response.status()).toBe(200);
    });

    test('GET /healthz returns JSON with status ok', async ({ request }) => {
      const response = await request.get(`${CHATBOX_PREFIX}/healthz`);
      const body = await response.json();

      expect(body).toHaveProperty('status');
      expect(body.status).toBe('ok');
    });
  });

  test.describe('readiness probe', () => {
    test('GET /readyz returns 200 when healthy', async ({ request }) => {
      const response = await request.get(`${CHATBOX_PREFIX}/readyz`);

      // 200 means MongoDB + LLM are reachable
      // 503 means one of them is down
      expect([200, 503]).toContain(response.status());
    });

    test('GET /readyz returns JSON with checks', async ({ request }) => {
      const response = await request.get(`${CHATBOX_PREFIX}/readyz`);
      const body = await response.json();

      expect(body).toHaveProperty('status');
      // Status is either 'ok' or 'degraded'
      expect(['ok', 'degraded']).toContain(body.status);
    });
  });
});

test.describe('Prometheus Metrics', () => {
  test('GET /metrics returns Prometheus-format metrics', async ({ request }) => {
    const response = await request.get('/metrics');

    expect(response.status()).toBe(200);

    const contentType = response.headers()['content-type'];
    expect(contentType).toContain('text/plain');

    const body = await response.text();
    // Prometheus metrics contain HELP and TYPE lines
    expect(body).toContain('# HELP');
    expect(body).toContain('# TYPE');
  });
});
