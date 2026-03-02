// JWT token generator for E2E tests.
// Used by API-level tests that hit the real server.
// Requires JWT_SECRET environment variable matching the server's config.

import { createHmac } from 'node:crypto';

function base64UrlEncode(data) {
  return Buffer.from(data)
    .toString('base64')
    .replace(/\+/g, '-')
    .replace(/\//g, '_')
    .replace(/=+$/, '');
}

/**
 * Generate a signed JWT token for testing.
 * @param {object} options
 * @param {string} options.userId - User ID claim
 * @param {string} [options.name] - User name claim
 * @param {string[]} [options.roles] - User roles
 * @param {string} [options.secret] - JWT secret (defaults to JWT_SECRET env var)
 * @param {number} [options.expiresInSeconds] - Token expiry in seconds from now (default: 1 hour)
 * @returns {string} Signed JWT token
 */
export function generateTestJWT({
  userId,
  name,
  roles = ['user'],
  secret = process.env.JWT_SECRET,
  expiresInSeconds = 3600,
}) {
  if (!secret) {
    throw new Error('JWT_SECRET environment variable is required for API tests');
  }

  const header = { alg: 'HS256', typ: 'JWT' };

  const now = Math.floor(Date.now() / 1000);
  const payload = {
    user_id: userId,
    name: name || userId,
    roles,
    iat: now,
    exp: now + expiresInSeconds,
  };

  const encodedHeader = base64UrlEncode(JSON.stringify(header));
  const encodedPayload = base64UrlEncode(JSON.stringify(payload));
  const signingInput = `${encodedHeader}.${encodedPayload}`;

  const signature = createHmac('sha256', secret)
    .update(signingInput)
    .digest('base64')
    .replace(/\+/g, '-')
    .replace(/\//g, '_')
    .replace(/=+$/, '');

  return `${signingInput}.${signature}`;
}

/**
 * Generate a user JWT token for testing.
 */
export function generateUserToken(secret) {
  return generateTestJWT({
    userId: 'e2e-test-user',
    name: 'E2E Test User',
    roles: ['user'],
    secret,
  });
}

/**
 * Generate an admin JWT token for testing.
 */
export function generateAdminToken(secret) {
  return generateTestJWT({
    userId: 'e2e-test-admin',
    name: 'E2E Test Admin',
    roles: ['admin'],
    secret,
  });
}
