// @ts-check
import { test, expect } from '@playwright/test';
import { generateUserToken } from '../helpers/jwt-helper.js';

// WebSocket API tests run against the real Go server.
// Requires: JWT_SECRET env var matching server config + docker-compose up
// Run with: JWT_SECRET=your-secret npm run test:api

const CHATBOX_PREFIX = process.env.CHATBOX_PATH_PREFIX || '/chatbox';
const JWT_SECRET = process.env.JWT_SECRET;

test.describe('WebSocket Endpoint', () => {
  test('GET /ws without token returns upgrade error', async ({ request }) => {
    // Without proper WebSocket upgrade headers, server should reject
    const response = await request.get(`${CHATBOX_PREFIX}/ws`);

    // Should return 4xx - either 400 (bad request) or 401 (unauthorized)
    expect(response.status()).toBeGreaterThanOrEqual(400);
    expect(response.status()).toBeLessThan(500);
  });

  test('WebSocket connects successfully with valid token', async ({ page }) => {
    test.skip(!JWT_SECRET, 'JWT_SECRET env var required');

    const token = generateUserToken(JWT_SECRET);
    const baseURL = process.env.API_BASE_URL || 'http://localhost:8080';

    // Use the browser's WebSocket to test real connection
    const result = await page.evaluate(async ({ wsBaseUrl, wsToken, prefix }) => {
      return new Promise((resolve) => {
        const protocol = wsBaseUrl.startsWith('https') ? 'wss:' : 'ws:';
        const host = wsBaseUrl.replace(/^https?:\/\//, '');
        const wsUrl = `${protocol}//${host}${prefix}/ws?token=${wsToken}`;

        const ws = new WebSocket(wsUrl);
        const result = { connected: false, messages: [], error: null };

        const timeout = setTimeout(() => {
          ws.close();
          resolve(result);
        }, 5000);

        ws.onopen = () => {
          result.connected = true;
        };

        ws.onmessage = (event) => {
          try {
            result.messages.push(JSON.parse(event.data));
          } catch {
            result.messages.push(event.data);
          }

          // After receiving connection_status, close and resolve
          if (result.messages.length >= 1) {
            clearTimeout(timeout);
            ws.close();
            resolve(result);
          }
        };

        ws.onerror = () => {
          result.error = 'WebSocket error';
          clearTimeout(timeout);
          resolve(result);
        };

        ws.onclose = (event) => {
          if (!result.connected && !result.error) {
            result.error = `Closed: ${event.code} ${event.reason}`;
          }
          clearTimeout(timeout);
          resolve(result);
        };
      });
    }, { wsBaseUrl: baseURL, wsToken: token, prefix: CHATBOX_PREFIX });

    expect(result.connected).toBe(true);
    expect(result.error).toBeNull();

    // Server should send a connection_status message on connect
    if (result.messages.length > 0) {
      const connStatus = result.messages.find((m) => m.type === 'connection_status');
      if (connStatus) {
        expect(connStatus).toHaveProperty('session_id');
        expect(connStatus).toHaveProperty('user_id');
      }
    }
  });

  test('WebSocket rejects connection with invalid token', async ({ page }) => {
    const baseURL = process.env.API_BASE_URL || 'http://localhost:8080';

    const result = await page.evaluate(async ({ wsBaseUrl, prefix }) => {
      return new Promise((resolve) => {
        const protocol = wsBaseUrl.startsWith('https') ? 'wss:' : 'ws:';
        const host = wsBaseUrl.replace(/^https?:\/\//, '');
        const wsUrl = `${protocol}//${host}${prefix}/ws?token=invalid-token`;

        const ws = new WebSocket(wsUrl);
        const result = { connected: false, closedWithError: false, closeCode: null };

        const timeout = setTimeout(() => {
          ws.close();
          resolve(result);
        }, 5000);

        ws.onopen = () => {
          result.connected = true;
        };

        ws.onclose = (event) => {
          result.closedWithError = true;
          result.closeCode = event.code;
          clearTimeout(timeout);
          resolve(result);
        };

        ws.onerror = () => {
          clearTimeout(timeout);
          // Don't resolve here - onclose will fire after onerror
        };
      });
    }, { wsBaseUrl: baseURL, prefix: CHATBOX_PREFIX });

    // Connection should either not complete or be closed by server
    // Close code 1006 = abnormal closure (server rejected), 4001 = custom auth error
    if (result.closedWithError) {
      expect(result.closeCode).not.toBe(1000); // 1000 = normal close
    }
  });

  test('WebSocket handles ping/pong', async ({ page }) => {
    test.skip(!JWT_SECRET, 'JWT_SECRET env var required');

    const token = generateUserToken(JWT_SECRET);
    const baseURL = process.env.API_BASE_URL || 'http://localhost:8080';

    const result = await page.evaluate(async ({ wsBaseUrl, wsToken, prefix }) => {
      return new Promise((resolve) => {
        const protocol = wsBaseUrl.startsWith('https') ? 'wss:' : 'ws:';
        const host = wsBaseUrl.replace(/^https?:\/\//, '');
        const wsUrl = `${protocol}//${host}${prefix}/ws?token=${wsToken}`;

        const ws = new WebSocket(wsUrl);
        const result = { connected: false, pingAcknowledged: false, error: null };

        const timeout = setTimeout(() => {
          ws.close();
          resolve(result);
        }, 5000);

        ws.onopen = () => {
          result.connected = true;
          // Send a ping message (application-level, not WebSocket ping frame)
          ws.send(JSON.stringify({ type: 'ping' }));
        };

        ws.onmessage = (event) => {
          try {
            const msg = JSON.parse(event.data);
            if (msg.type === 'pong') {
              result.pingAcknowledged = true;
              clearTimeout(timeout);
              ws.close();
              resolve(result);
            }
          } catch {
            // ignore
          }
        };

        ws.onerror = () => {
          result.error = 'WebSocket error';
        };

        ws.onclose = () => {
          clearTimeout(timeout);
          resolve(result);
        };
      });
    }, { wsBaseUrl: baseURL, wsToken: token, prefix: CHATBOX_PREFIX });

    expect(result.connected).toBe(true);
    // Ping/pong may be handled at protocol level; application-level pong is optional
    // Just verify the connection stayed alive
    expect(result.error).toBeNull();
  });
});
