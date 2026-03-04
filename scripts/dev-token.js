#!/usr/bin/env node
// Dev token generator — produces signed JWT tokens and ready-to-use URLs.
// Usage:
//   node scripts/dev-token.js                     # user token (default)
//   node scripts/dev-token.js --role admin         # admin token
//   node scripts/dev-token.js --user-id fred       # custom user ID
//   node scripts/dev-token.js --expires 86400      # 24h expiry
//   JWT_SECRET=xxx node scripts/dev-token.js       # explicit secret

import { createHmac, randomUUID } from "node:crypto";
import { readFileSync } from "node:fs";
import { resolve, dirname } from "node:path";
import { fileURLToPath } from "node:url";

const __dirname = dirname(fileURLToPath(import.meta.url));
const projectRoot = resolve(__dirname, "..");

// --- Parse CLI flags ---
function parseArgs(args) {
  const opts = {
    role: "user",
    userId: "",
    name: "",
    expires: 3600,
    secret: "",
    port: 8080,
    prefix: "/chatbox",
    webPort: 3333,
  };

  for (let i = 0; i < args.length; i++) {
    switch (args[i]) {
      case "--role":
        opts.role = args[++i];
        break;
      case "--user-id":
        opts.userId = args[++i];
        break;
      case "--name":
        opts.name = args[++i];
        break;
      case "--expires":
        opts.expires = parseInt(args[++i], 10);
        break;
      case "--secret":
        opts.secret = args[++i];
        break;
      case "--port":
        opts.port = parseInt(args[++i], 10);
        break;
      case "--prefix":
        opts.prefix = args[++i];
        break;
      case "--web-port":
        opts.webPort = parseInt(args[++i], 10);
        break;
      case "--help":
        console.log(`Usage: node scripts/dev-token.js [options]

Options:
  --role <user|admin>    Token role (default: user)
  --user-id <id>         User ID claim (default: dev-<role>)
  --name <name>          Display name (default: Dev <Role>)
  --expires <seconds>    Expiry in seconds (default: 3600)
  --secret <secret>      JWT secret (default: read from config.local.toml or JWT_SECRET env)
  --port <port>          Server port (default: 8080)
  --prefix <path>        Path prefix (default: /chatbox)
  --web-port <port>      Static dev server port for frontend pages (default: 3333)
  --help                 Show this help`);
        process.exit(0);
    }
  }

  if (!opts.userId) opts.userId = `dev-${opts.role}`;
  if (!opts.name)
    opts.name = `Dev ${opts.role.charAt(0).toUpperCase() + opts.role.slice(1)}`;

  return opts;
}

// --- Extract JWT secret from config.local.toml ---
function readSecretFromConfig() {
  const configPaths = ["config.local.toml", "config.toml"];
  for (const name of configPaths) {
    try {
      const content = readFileSync(resolve(projectRoot, name), "utf-8");
      const match = content.match(/^\s*jwt_secret\s*=\s*"([^"]+)"/m);
      if (match) return match[1];
    } catch {
      // file not found, try next
    }
  }
  return "";
}

// --- JWT generation ---
function base64UrlEncode(data) {
  return Buffer.from(data)
    .toString("base64")
    .replace(/\+/g, "-")
    .replace(/\//g, "_")
    .replace(/=+$/, "");
}

function generateJWT({ userId, name, roles, secret, expiresInSeconds }) {
  const header = { alg: "HS256", typ: "JWT" };
  const now = Math.floor(Date.now() / 1000);
  const payload = {
    user_id: userId,
    name,
    roles,
    iat: now,
    exp: now + expiresInSeconds,
  };

  const encodedHeader = base64UrlEncode(JSON.stringify(header));
  const encodedPayload = base64UrlEncode(JSON.stringify(payload));
  const signingInput = `${encodedHeader}.${encodedPayload}`;

  const signature = createHmac("sha256", secret)
    .update(signingInput)
    .digest("base64")
    .replace(/\+/g, "-")
    .replace(/\//g, "_")
    .replace(/=+$/, "");

  return `${signingInput}.${signature}`;
}

// --- Main ---
const opts = parseArgs(process.argv.slice(2));

// Resolve secret: CLI flag > env var > config file
const secret = opts.secret || process.env.JWT_SECRET || readSecretFromConfig();
if (!secret) {
  console.error("Error: No JWT secret found.");
  console.error(
    "Provide via: --secret <value>, JWT_SECRET env var, or jwt_secret in config.local.toml",
  );
  process.exit(1);
}

const roles = [opts.role];
const token = generateJWT({
  userId: opts.userId,
  name: opts.name,
  roles,
  secret,
  expiresInSeconds: opts.expires,
});

const encodedToken = encodeURIComponent(token);
const serverBase = `http://localhost:${opts.port}`;
const webBase = `http://localhost:${opts.webPort}`;
const prefix = opts.prefix;

const expiresAt = new Date(Date.now() + opts.expires * 1000).toLocaleString();

console.log("");
console.log(`  Role:       ${opts.role}`);
console.log(`  User ID:    ${opts.userId}`);
console.log(`  Name:       ${opts.name}`);
console.log(`  Expires:    ${expiresAt} (${opts.expires}s)`);
console.log("");
console.log("  Token:");
console.log(`  ${token}`);
console.log("");
const apiParam = `&api=localhost:${opts.port}`;
console.log("  --- Web Pages (requires: make dev-server) ---");
console.log(
  `  Sessions:   ${webBase}/sessions.html?token=${encodedToken}${apiParam}`,
);
console.log(
  `  Chat:       ${webBase}/chat.html?token=${encodedToken}${apiParam}`,
);
console.log(
  `  Admin:      ${webBase}/admin.html?token=${encodedToken}${apiParam}`,
);
console.log("");
console.log("  --- API Endpoints (Go server) ---");
console.log(
  `  WebSocket:  ws://localhost:${opts.port}${prefix}/ws?token=${encodedToken}`,
);
console.log(
  `  Sessions:   ${serverBase}${prefix}/sessions  (Authorization: Bearer <token>)`,
);
console.log(`  Health:     ${serverBase}${prefix}/healthz`);
console.log(`  Readiness:  ${serverBase}${prefix}/readyz`);
console.log(`  Metrics:    ${serverBase}/metrics`);
console.log("");
console.log("  --- curl examples ---");
console.log(
  `  curl -H "Authorization: Bearer ${token}" ${serverBase}${prefix}/sessions`,
);
if (opts.role === "admin") {
  console.log(
    `  curl -H "Authorization: Bearer ${token}" ${serverBase}${prefix}/admin/sessions`,
  );
  console.log(
    `  curl -H "Authorization: Bearer ${token}" ${serverBase}${prefix}/admin/metrics`,
  );
}
console.log("");
