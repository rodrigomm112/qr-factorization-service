import request from 'supertest';
import { describe, expect, it } from 'vitest';
import { createTestApp } from '../support/harness.js';

describe('health and documentation endpoints', () => {
  const { app } = createTestApp();

  it.each(['/health/live', '/health/ready'])('answers %s with application/health+json', async (path) => {
    const response = await request(app).get(path);

    expect(response.status).toBe(200);
    expect(response.headers['content-type']).toBe('application/health+json');
    expect(response.body).toEqual({
      status: 'pass',
      service: 'stats-api',
      version: expect.stringMatching(/^\d+\.\d+\.\d+/),
      uptimeSeconds: expect.any(Number),
    });
    expect(response.body.uptimeSeconds).toBeGreaterThanOrEqual(0);
  });

  it('serves the OpenAPI contract as application/yaml', async () => {
    const response = await request(app).get('/openapi.yaml');

    expect(response.status).toBe(200);
    expect(response.headers['content-type']).toBe('application/yaml');
    expect(response.text).toContain('openapi: 3.1.0');
    expect(response.text).toContain('/api/v1/statistics');
    // The contract documents the documentation endpoints themselves.
    expect(response.text).toContain('\n  /openapi.yaml:');
    expect(response.text).toContain('\n  /docs:');
  });

  it('redirects /docs to /docs/, as the contract declares', async () => {
    const response = await request(app).get('/docs');

    expect(response.status).toBe(301);
    expect(response.headers['location']).toBe('/docs/');
  });

  it('rejects a write method on the contract', async () => {
    await request(app).post('/openapi.yaml').send({}).expect(405);
  });

  it('serves Swagger UI pointing at the local contract', async () => {
    const response = await request(app).get('/docs/');

    expect(response.status).toBe(200);
    expect(response.headers['content-type']).toMatch(/text\/html/);
    expect(response.text).toContain('swagger-ui');

    // The bootstrap is an external swagger-ui-init.js, so only the styles need the escape hatch.
    const csp = response.headers['content-security-policy'] ?? '';
    expect(csp).toContain("script-src 'self'");
    expect(csp).not.toContain("script-src 'self' 'unsafe-inline'");
    expect(csp).toContain("style-src 'self' 'unsafe-inline'");
    expect(response.text).not.toMatch(/<script(?![^>]*\ssrc=)[^>]*>[^<]/);

    const bootstrap = await request(app).get('/docs/swagger-ui-init.js');
    expect(bootstrap.status).toBe(200);
    expect(bootstrap.text).toContain('/openapi.yaml');
  });

  it('serves the Swagger UI assets from the bundle, never from a CDN', async () => {
    const response = await request(app).get('/docs/swagger-ui.css');
    expect(response.status).toBe(200);
    expect(response.headers['content-type']).toMatch(/text\/css/);
  });
});
