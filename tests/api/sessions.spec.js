// @ts-check
import { test, expect } from '@playwright/test';
import { generateUserToken, generateAdminToken } from '../helpers/jwt-helper.js';

// API tests run against the real Go server.
// Requires: JWT_SECRET env var matching server config + docker-compose up
// Run with: JWT_SECRET=your-secret npm run test:api

const CHATBOX_PREFIX = process.env.CHATBOX_PATH_PREFIX || '/chatbox';
const JWT_SECRET = process.env.JWT_SECRET;

test.describe('Sessions API', () => {
  test.describe('user sessions endpoint', () => {
    test('GET /sessions returns 401 without token', async ({ request }) => {
      const response = await request.get(`${CHATBOX_PREFIX}/sessions`);

      expect(response.status()).toBe(401);
    });

    test('GET /sessions returns 401 with invalid token', async ({ request }) => {
      const response = await request.get(`${CHATBOX_PREFIX}/sessions`, {
        headers: { Authorization: 'Bearer invalid-token-here' },
      });

      expect(response.status()).toBe(401);
    });

    test('GET /sessions returns 200 with valid token', async ({ request }) => {
      test.skip(!JWT_SECRET, 'JWT_SECRET env var required');

      const token = generateUserToken(JWT_SECRET);
      const response = await request.get(`${CHATBOX_PREFIX}/sessions`, {
        headers: { Authorization: `Bearer ${token}` },
      });

      expect(response.status()).toBe(200);

      const body = await response.json();
      expect(body).toHaveProperty('sessions');
      expect(Array.isArray(body.sessions)).toBe(true);
    });
  });

  test.describe('admin sessions endpoint', () => {
    test('GET /admin/sessions returns 401 without token', async ({ request }) => {
      const response = await request.get(`${CHATBOX_PREFIX}/admin/sessions`);

      expect(response.status()).toBe(401);
    });

    test('GET /admin/sessions returns 403 with non-admin token', async ({ request }) => {
      test.skip(!JWT_SECRET, 'JWT_SECRET env var required');

      const token = generateUserToken(JWT_SECRET);
      const response = await request.get(`${CHATBOX_PREFIX}/admin/sessions`, {
        headers: { Authorization: `Bearer ${token}` },
      });

      // Should be 401 or 403 - user role doesn't have admin access
      expect([401, 403]).toContain(response.status());
    });

    test('GET /admin/sessions returns 200 with admin token', async ({ request }) => {
      test.skip(!JWT_SECRET, 'JWT_SECRET env var required');

      const token = generateAdminToken(JWT_SECRET);
      const response = await request.get(`${CHATBOX_PREFIX}/admin/sessions`, {
        headers: { Authorization: `Bearer ${token}` },
      });

      expect(response.status()).toBe(200);
    });
  });

  test.describe('admin metrics endpoint', () => {
    test('GET /admin/metrics returns 401 without token', async ({ request }) => {
      const response = await request.get(`${CHATBOX_PREFIX}/admin/metrics`);

      expect(response.status()).toBe(401);
    });

    test('GET /admin/metrics returns 200 with admin token', async ({ request }) => {
      test.skip(!JWT_SECRET, 'JWT_SECRET env var required');

      const token = generateAdminToken(JWT_SECRET);
      const response = await request.get(`${CHATBOX_PREFIX}/admin/metrics`, {
        headers: { Authorization: `Bearer ${token}` },
      });

      expect(response.status()).toBe(200);

      const body = await response.json();
      expect(body).toHaveProperty('active_sessions');
      expect(body).toHaveProperty('total_sessions');
    });
  });

  test.describe('admin takeover endpoint', () => {
    test('POST /admin/takeover/:sessionID returns 401 without token', async ({ request }) => {
      const response = await request.post(`${CHATBOX_PREFIX}/admin/takeover/fake-session-id`);

      expect(response.status()).toBe(401);
    });

    test('POST /admin/takeover/:sessionID returns error with non-admin token', async ({ request }) => {
      test.skip(!JWT_SECRET, 'JWT_SECRET env var required');

      const token = generateUserToken(JWT_SECRET);
      const response = await request.post(`${CHATBOX_PREFIX}/admin/takeover/fake-session-id`, {
        headers: { Authorization: `Bearer ${token}` },
      });

      expect([401, 403]).toContain(response.status());
    });
  });
});
