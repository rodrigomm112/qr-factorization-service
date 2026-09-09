import { describe, expect, it } from 'vitest';
import { buildApp } from '../../src/adapters/inbound/http/app.js';
import { ConfigurationError, loadConfig } from '../../src/platform/config.js';
import { createLogger } from '../../src/platform/logger.js';

const SECRET = 'a-secret-long-enough-to-be-accepted-01234';

describe('loadConfig', () => {
  it('applies a default to every key except JWT_SECRET', () => {
    const config = loadConfig({ JWT_SECRET: SECRET });

    expect(config).toEqual({
      port: 3000,
      appEnv: 'development',
      logLevel: 'info',
      jwt: { secret: SECRET, issuer: 'qr-api', audience: 'stats-api' },
      corsAllowedOrigins: [],
      maxBodyBytes: 4_194_304,
      rateLimit: { max: 120, windowMs: 60_000 },
      diagonalTolerance: { absTol: 1e-12, relTol: 1e-9 },
      limits: {
        maxMatrices: 8,
        maxTotalElements: 100_000,
        maxMatrixRows: 100,
        maxMatrixCols: 100,
      },
      trustProxy: false,
      shutdownTimeoutMs: 10_000,
    });
  });

  it('fails fast when JWT_SECRET is missing', () => {
    expect(() => loadConfig({})).toThrow(ConfigurationError);
    expect(() => loadConfig({})).toThrow(/JWT_SECRET/);
  });

  it('fails fast when JWT_SECRET is shorter than 32 characters', () => {
    expect(() => loadConfig({ JWT_SECRET: 'too-short' })).toThrow(
      /JWT_SECRET: must be at least 32 characters/,
    );
  });

  it('ignores every environment variable it does not declare', () => {
    // One .env feeds both services; RATE_LIMIT_WINDOW is qr-api's Go duration, not milliseconds.
    const config = loadConfig({
      JWT_SECRET: SECRET,
      RATE_LIMIT_WINDOW: '1m',
      STATS_PORT: '3000',
      STATS_API_TIMEOUT: '5s',
      AUTH_CLIENT_SECRET_SHA256: 'deadbeef',
      SHUTDOWN_TIMEOUT: '10s',
      JWT_TTL: '1h',
      SOMETHING_ENTIRELY_UNKNOWN: 'x',
    });

    expect(config.rateLimit.windowMs).toBe(60_000);
    expect(config.port).toBe(3000);
    expect(config.shutdownTimeoutMs).toBe(10_000);
  });

  it('treats an empty value as unset', () => {
    const config = loadConfig({ JWT_SECRET: SECRET, LOG_LEVEL: '', PORT: '   ' });
    expect(config.logLevel).toBe('info');
    expect(config.port).toBe(3000);
  });

  it('coerces numbers and booleans from strings', () => {
    const config = loadConfig({
      JWT_SECRET: SECRET,
      PORT: '8080',
      MAX_BODY_BYTES: '1048576',
      DIAGONAL_ABS_TOLERANCE: '1e-10',
      DIAGONAL_REL_TOLERANCE: '1e-7',
      TRUST_PROXY: 'true',
      APP_ENV: 'production',
      LOG_LEVEL: 'debug',
    });

    expect(config.port).toBe(8080);
    expect(config.maxBodyBytes).toBe(1_048_576);
    expect(config.diagonalTolerance).toEqual({ absTol: 1e-10, relTol: 1e-7 });
    expect(config.trustProxy).toBe(true);
    expect(config.appEnv).toBe('production');
    expect(config.logLevel).toBe('debug');
  });

  it('parses the CORS allow-list and refuses the wildcard', () => {
    expect(
      loadConfig({ JWT_SECRET: SECRET, CORS_ALLOWED_ORIGINS: 'http://a.test, http://b.test ,' })
        .corsAllowedOrigins,
    ).toEqual(['http://a.test', 'http://b.test']);

    expect(() => loadConfig({ JWT_SECRET: SECRET, CORS_ALLOWED_ORIGINS: '*' })).toThrow(
      /CORS_ALLOWED_ORIGINS/,
    );
  });

  it('rejects values outside the accepted domain', () => {
    expect(() => loadConfig({ JWT_SECRET: SECRET, APP_ENV: 'staging' })).toThrow(/APP_ENV/);
    expect(() => loadConfig({ JWT_SECRET: SECRET, PORT: '0' })).toThrow(/PORT/);
    expect(() => loadConfig({ JWT_SECRET: SECRET, TRUST_PROXY: 'maybe' })).toThrow(/TRUST_PROXY/);
    expect(() => loadConfig({ JWT_SECRET: SECRET, MAX_MATRICES: '-1' })).toThrow(/MAX_MATRICES/);
  });

  it('trusts exactly one proxy hop only when TRUST_PROXY is set', () => {
    const build = (trustProxy: string): unknown => {
      const config = loadConfig({ JWT_SECRET: SECRET, TRUST_PROXY: trustProxy, LOG_LEVEL: 'silent' });
      return buildApp({ config, logger: createLogger(config) }).get('trust proxy');
    };

    expect(build('true')).toBe(1);
    expect(build('false')).toBe(false);
  });
});
