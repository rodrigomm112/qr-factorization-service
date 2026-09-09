import { randomUUID } from 'node:crypto';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import type { Express } from 'express';
import jwt from 'jsonwebtoken';
import type { SignOptions } from 'jsonwebtoken';
import type { DestinationStream } from 'pino';
import { buildApp } from '../../src/adapters/inbound/http/app.js';
import { loadConfig } from '../../src/platform/config.js';
import type { AppConfig } from '../../src/platform/config.js';
import { createLogger } from '../../src/platform/logger.js';

export const TEST_SECRET = 'integration-test-secret-0123456789abcdef';
export const TEST_ISSUER = 'qr-api';
export const TEST_AUDIENCE = 'stats-api';

const BASE_ENV: Record<string, string> = {
  JWT_SECRET: TEST_SECRET,
  JWT_ISSUER: TEST_ISSUER,
  JWT_AUDIENCE: TEST_AUDIENCE,
  APP_ENV: 'production',
  LOG_LEVEL: 'silent',
  CORS_ALLOWED_ORIGINS: 'http://localhost:8081,http://localhost:5173',
};

/** Built through the real loader, so every test exercises it too. */
export function testConfig(env: Record<string, string> = {}): AppConfig {
  return loadConfig({ ...BASE_ENV, ...env });
}

export interface TestApp {
  readonly app: Express;
  readonly config: AppConfig;
}

export function createTestApp(env: Record<string, string> = {}, destination?: DestinationStream): TestApp {
  const config = testConfig(env);
  const logger = createLogger(config, destination);
  return { app: buildApp({ config, logger }), config };
}

export function mintToken(overrides: SignOptions = {}, secret: string = TEST_SECRET): string {
  return jwt.sign({ scope: 'qr:compute stats:compute' }, secret, {
    algorithm: 'HS256',
    issuer: TEST_ISSUER,
    audience: TEST_AUDIENCE,
    subject: 'demo-client',
    expiresIn: '5m',
    jwtid: randomUUID(),
    ...overrides,
  });
}

export function bearer(token: string = mintToken()): string {
  return `Bearer ${token}`;
}

const FIXTURES_DIR = new URL('../../../../contracts/examples/', import.meta.url);

export function fixture(name: string): Record<string, unknown> {
  const path = fileURLToPath(new URL(name, FIXTURES_DIR));
  return JSON.parse(readFileSync(path, 'utf8')) as Record<string, unknown>;
}

const VOLATILE_KEYS = new Set(['requestId', 'timestamp', 'issuedAt', 'uptimeSeconds']);

/** Drops the fields problems.md §5 declares volatile, so a response can be deep-equalled. */
export function stripVolatile(value: unknown): unknown {
  if (Array.isArray(value)) return value.map(stripVolatile);
  if (typeof value !== 'object' || value === null) return value;
  const result: Record<string, unknown> = {};
  for (const [key, item] of Object.entries(value as Record<string, unknown>)) {
    if (VOLATILE_KEYS.has(key) || key.endsWith('Ms')) continue;
    result[key] = stripVolatile(item);
  }
  return result;
}
