import { createHmac } from 'node:crypto';
import request from 'supertest';
import jwt from 'jsonwebtoken';
import { describe, expect, it } from 'vitest';
import {
  bearer,
  createTestApp,
  fixture,
  mintToken,
  TEST_AUDIENCE,
  TEST_ISSUER,
  TEST_SECRET,
} from '../support/harness.js';

const BODY = { matrices: [[[1, 0], [0, 1]]] };

function base64url(value: string): string {
  return Buffer.from(value, 'utf8').toString('base64url');
}

/** `alg: none` token: a well-formed JWT with an empty signature. */
function algNoneToken(): string {
  const header = base64url(JSON.stringify({ alg: 'none', typ: 'JWT' }));
  const payload = base64url(
    JSON.stringify({
      iss: TEST_ISSUER,
      aud: TEST_AUDIENCE,
      sub: 'demo-client',
      exp: Math.floor(Date.now() / 1000) + 300,
    }),
  );
  return `${header}.${payload}.`;
}

// jsonwebtoken only checks `exp` when present, so without a guard this token never expires.
function tokenWithoutExpiry(): string {
  return jwt.sign({ scope: 'qr:compute stats:compute' }, TEST_SECRET, {
    algorithm: 'HS256',
    issuer: TEST_ISSUER,
    audience: TEST_AUDIENCE,
    subject: 'demo-client',
  });
}

function forgedSignature(): string {
  const [header, payload] = mintToken().split('.');
  const signature = createHmac('sha256', 'a-completely-different-secret-0123456789')
    .update(`${header ?? ''}.${payload ?? ''}`)
    .digest('base64url');
  return `${header ?? ''}.${payload ?? ''}.${signature}`;
}

describe('bearer authentication on POST /api/v1/statistics', () => {
  const { app } = createTestApp();

  it('accepts a well-formed token', async () => {
    const response = await request(app)
      .post('/api/v1/statistics')
      .set('Authorization', bearer())
      .send(BODY);
    expect(response.status).toBe(200);
  });

  it('answers 401 with the shared problem document when the token is missing', async () => {
    const response = await request(app).post('/api/v1/statistics').send(BODY);

    expect(response.status).toBe(401);
    expect(response.headers['content-type']).toBe('application/problem+json');
    // No credentials: advertise the scheme, do not claim the token is invalid.
    expect(response.headers['www-authenticate']).toBe('Bearer realm="proyectot"');

    const golden = fixture('problem.unauthorized.json');
    expect(response.body).toMatchObject({
      type: golden['type'],
      title: golden['title'],
      status: 401,
      detail: golden['detail'],
      instance: '/api/v1/statistics',
    });
    expect(response.body).not.toHaveProperty('errors');
    expect(Object.keys(response.body).sort()).toEqual(Object.keys(golden).sort());
  });

  const rejected: ReadonlyArray<readonly [string, () => string]> = [
    ['a forged signature', forgedSignature],
    ['an alg:none token', algNoneToken],
    ['a token for another audience', () => mintToken({ audience: 'qr-api' })],
    ['a token from another issuer', () => mintToken({ issuer: 'evil-issuer' })],
    ['an expired token', () => mintToken({ expiresIn: '-1h' })],
    ['a token that never expires', tokenWithoutExpiry],
    ['a token signed with another secret', () => mintToken({}, 'another-secret-that-is-long-enough-01234')],
    ['garbage instead of a JWT', () => 'not-a-json-web-token'],
  ];

  it.each(rejected)('rejects %s', async (_name, tokenFactory) => {
    const response = await request(app)
      .post('/api/v1/statistics')
      .set('Authorization', `Bearer ${tokenFactory()}`)
      .send(BODY);

    expect(response.status).toBe(401);
    expect(response.headers['www-authenticate']).toBe('Bearer realm="proyectot", error="invalid_token"');
    expect(response.body.detail).toBe('The access token is missing, expired or invalid.');
  });

  const malformedHeaders = [
    ['an empty Authorization header', ''],
    ['a scheme without a token', 'Bearer'],
    ['a scheme glued to the token', 'Bearerabc.def.ghi'],
    ['another auth scheme', 'Basic dXNlcjpwYXNz'],
  ] as const;

  it.each(malformedHeaders)('rejects %s', async (_name, header) => {
    const response = await request(app)
      .post('/api/v1/statistics')
      .set('Authorization', header)
      .send(BODY);
    expect(response.status).toBe(401);
  });

  it('accepts a token that is still inside the 30 s clock tolerance', async () => {
    const issuedAt = Math.floor(Date.now() / 1000) - 20;
    const token = jwt.sign({ iat: issuedAt, exp: issuedAt + 10 }, TEST_SECRET, {
      algorithm: 'HS256',
      issuer: TEST_ISSUER,
      audience: TEST_AUDIENCE,
    });
    const response = await request(app)
      .post('/api/v1/statistics')
      .set('Authorization', `Bearer ${token}`)
      .send(BODY);
    expect(response.status).toBe(200);
  });

  it('does not require a token on the health endpoints', async () => {
    await request(app).get('/health/live').expect(200);
    await request(app).get('/health/ready').expect(200);
  });
});
