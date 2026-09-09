import request from 'supertest';
import { describe, expect, it } from 'vitest';
import { bearer, createTestApp } from '../support/harness.js';

const BODY = { matrices: [[[1, 0], [0, 1]]] };
const UUID_PATTERN = /^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/;

describe('HTTP contract', () => {
  const { app } = createTestApp();

  describe('X-Request-ID', () => {
    it('echoes a valid inbound correlation id', async () => {
      const id = '00000000-0000-4000-8000-000000000000';
      const response = await request(app)
        .post('/api/v1/statistics')
        .set('Authorization', bearer())
        .set('X-Request-ID', id)
        .send(BODY);

      expect(response.headers['x-request-id']).toBe(id);
      expect(response.body.requestId).toBe(id);
    });

    it('generates a UUID v4 when the inbound id is missing or malformed', async () => {
      const generated = await request(app).get('/health/live');
      expect(generated.headers['x-request-id']).toMatch(UUID_PATTERN);

      const rejected = await request(app)
        .post('/api/v1/statistics')
        .set('Authorization', bearer())
        .set('X-Request-ID', 'definitely-not-a-uuid')
        .send(BODY);
      expect(rejected.headers['x-request-id']).toMatch(UUID_PATTERN);
      expect(rejected.body.requestId).not.toBe('definitely-not-a-uuid');
    });

    it('is present on error responses and equal to the body requestId', async () => {
      const response = await request(app).get('/does-not-exist');
      expect(response.headers['x-request-id']).toBe(response.body.requestId);
    });
  });

  describe('routing failures', () => {
    it('answers 404 for an unknown path', async () => {
      const response = await request(app).get('/does-not-exist');

      expect(response.status).toBe(404);
      expect(response.headers['content-type']).toBe('application/problem+json');
      expect(response.body).toMatchObject({
        type: 'urn:proyectot:problem:not-found',
        title: 'Resource not found',
        status: 404,
        instance: '/does-not-exist',
      });
    });

    it('answers 405 for a known path with the wrong method', async () => {
      const response = await request(app).get('/api/v1/statistics');

      expect(response.status).toBe(405);
      expect(response.headers['allow']).toBe('POST');
      expect(response.body).toMatchObject({
        type: 'urn:proyectot:problem:method-not-allowed',
        title: 'Method not allowed',
        status: 405,
        instance: '/api/v1/statistics',
      });
    });

    it('answers 405 on the health endpoints for a write method', async () => {
      const response = await request(app).post('/health/live').send({});
      expect(response.status).toBe(405);
      expect(response.headers['allow']).toBe('GET, HEAD');
    });

    it('strips the query string from instance', async () => {
      const response = await request(app).get('/nope?debug=1');
      expect(response.body.instance).toBe('/nope');
    });
  });

  describe('request body limits', () => {
    it('answers 413 when the body exceeds MAX_BODY_BYTES', async () => {
      const { app: tiny } = createTestApp({ MAX_BODY_BYTES: '256' });
      const response = await request(tiny)
        .post('/api/v1/statistics')
        .set('Authorization', bearer())
        .send({ matrices: [Array.from({ length: 40 }, () => [1, 2, 3, 4])] });

      expect(response.status).toBe(413);
      expect(response.body).toMatchObject({
        type: 'urn:proyectot:problem:payload-too-large',
        title: 'Payload too large',
        status: 413,
        detail: 'The request body exceeds the 256 byte limit.',
      });
    });

    it('answers 415 when the content type is not application/json', async () => {
      const response = await request(app)
        .post('/api/v1/statistics')
        .set('Authorization', bearer())
        .type('text/plain')
        .send('matrices');

      expect(response.status).toBe(415);
      expect(response.body).toMatchObject({
        type: 'urn:proyectot:problem:unsupported-media-type',
        title: 'Unsupported media type',
        status: 415,
        detail: 'Content-Type must be application/json.',
      });
    });

    it('accepts application/json with an explicit charset', async () => {
      const response = await request(app)
        .post('/api/v1/statistics')
        .set('Authorization', bearer())
        .type('application/json; charset=utf-8')
        .send(JSON.stringify(BODY));
      expect(response.status).toBe(200);
    });

    it('answers 400 for a syntactically invalid body', async () => {
      const response = await request(app)
        .post('/api/v1/statistics')
        .set('Authorization', bearer())
        .type('application/json')
        .send('{"matrices": [');

      expect(response.status).toBe(400);
      expect(response.body.type).toBe('urn:proyectot:problem:malformed-json');
    });

    const emptyBodies: ReadonlyArray<readonly [string, string | undefined]> = [
      ['no body and no content type', undefined],
      ['a declared JSON content type but nothing in it', 'application/json'],
    ];

    it.each(emptyBodies)('answers 400 for %s', async (_name, contentType) => {
      // An empty payload is not a JSON document: a syntax failure, not a missing member.
      let pending = request(app).post('/api/v1/statistics').set('Authorization', bearer());
      if (contentType !== undefined) pending = pending.type(contentType);
      const response = await pending.send();

      expect(response.status).toBe(400);
      expect(response.body.type).toBe('urn:proyectot:problem:malformed-json');
      expect(response.body.detail).toBe('The request body is not valid JSON.');
      expect(response.body).not.toHaveProperty('errors');
    });
  });

  describe('rate limiting', () => {
    it('answers 429 with draft-8 headers and Retry-After', async () => {
      const { app: limited } = createTestApp({ RATE_LIMIT_MAX: '2', RATE_LIMIT_WINDOW_MS: '60000' });
      const send = (): request.Test =>
        request(limited).post('/api/v1/statistics').set('Authorization', bearer()).send(BODY);

      const first = await send();
      expect(first.status).toBe(200);
      expect(first.headers['ratelimit']).toMatch(/^"default";r=1;t=\d+$/);

      const second = await send();
      expect(second.status).toBe(200);
      expect(second.headers['ratelimit']).toMatch(/^"default";r=0;t=\d+$/);
      // qr-api's spelling, without the `pk=:<hashed ip>:` express-rate-limit would append.
      expect(second.headers['ratelimit-policy']).toBe('"default";q=2;w=60');
      expect(second.headers['x-ratelimit-limit']).toBeUndefined();

      const third = await send();
      expect(third.status).toBe(429);
      expect(third.headers['content-type']).toBe('application/problem+json');
      expect(third.headers['ratelimit']).toMatch(/^"default";r=0;t=\d+$/);
      expect(third.headers['ratelimit-policy']).toBe('"default";q=2;w=60');
      expect(Number(third.headers['retry-after'])).toBeGreaterThan(0);
      expect(third.body).toMatchObject({
        type: 'urn:proyectot:problem:too-many-requests',
        title: 'Too many requests',
        status: 429,
        detail: 'Rate limit exceeded. Retry after the window resets.',
      });
    });

    it('never rate limits the health endpoints', async () => {
      const { app: limited } = createTestApp({ RATE_LIMIT_MAX: '1' });
      for (let i = 0; i < 5; i += 1) {
        await request(limited).get('/health/ready').expect(200);
      }
    });
  });

  describe('security headers', () => {
    it('locks the API surface down to default-src none', async () => {
      const response = await request(app).get('/health/live');

      expect(response.headers['content-security-policy']).toContain("default-src 'none'");
      expect(response.headers['x-content-type-options']).toBe('nosniff');
      expect(response.headers['x-frame-options']).toBe('DENY');
      expect(response.headers['referrer-policy']).toBe('no-referrer');
      expect(response.headers['strict-transport-security']).toBeDefined();
      expect(response.headers['x-powered-by']).toBeUndefined();
    });

    it('omits HSTS outside production', async () => {
      const { app: dev } = createTestApp({ APP_ENV: 'development' });
      const response = await request(dev).get('/health/live');
      expect(response.headers['strict-transport-security']).toBeUndefined();
    });
  });

  describe('CORS', () => {
    it('allows a configured origin', async () => {
      const response = await request(app)
        .get('/health/ready')
        .set('Origin', 'http://localhost:8081');
      expect(response.headers['access-control-allow-origin']).toBe('http://localhost:8081');
      expect(response.headers['access-control-expose-headers']).toContain('X-Request-ID');
      expect(response.headers['access-control-allow-credentials']).toBeUndefined();
    });

    it('answers the preflight of the statistics endpoint', async () => {
      const response = await request(app)
        .options('/api/v1/statistics')
        .set('Origin', 'http://localhost:5173')
        .set('Access-Control-Request-Method', 'POST')
        .set('Access-Control-Request-Headers', 'authorization,content-type');

      expect(response.status).toBe(204);
      expect(response.headers['access-control-allow-origin']).toBe('http://localhost:5173');
      expect(response.headers['access-control-allow-headers']).toContain('Authorization');
    });

    it('sends no CORS headers to an origin that is not on the allow-list', async () => {
      const response = await request(app)
        .get('/health/ready')
        .set('Origin', 'https://evil.example');
      expect(response.status).toBe(200);
      expect(response.headers['access-control-allow-origin']).toBeUndefined();
    });
  });
});

describe('CORS preflight outside the allow-list', () => {
  it('answers 204 without CORS headers and without touching the rate limiter', async () => {
    const { app } = createTestApp();
    const response = await request(app)
      .options('/api/v1/statistics')
      .set('Origin', 'http://evil.example')
      .set('Access-Control-Request-Method', 'POST');
    expect(response.status).toBe(204);
    expect(response.headers['access-control-allow-origin']).toBeUndefined();
    expect(response.headers['ratelimit']).toBeUndefined();
  });
});
