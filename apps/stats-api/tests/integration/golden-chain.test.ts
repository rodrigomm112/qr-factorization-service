import request from 'supertest';
import { describe, expect, it } from 'vitest';
import { bearer, createTestApp, fixture, stripVolatile } from '../support/harness.js';

// Fixtures captured from the full qr-api -> stats-api stack: a drift fails here, not in the demo.
describe('golden fixtures of the qr-api -> stats-api chain', () => {
  const { app } = createTestApp();

  it('matches statistics.response.qr-tall-3x2 for the real Householder factors', async () => {
    const response = await request(app)
      .post('/api/v1/statistics')
      .set('Authorization', bearer())
      .send(fixture('statistics.request.qr-tall-3x2.json'));

    expect(response.status).toBe(200);
    expect(response.headers['content-type']).toMatch(/^application\/json/);
    // Every digit: the compensated sums of the 15 doubles must reproduce the frozen totals.
    expect(stripVolatile(response.body)).toEqual(
      stripVolatile(fixture('statistics.response.qr-tall-3x2.json')),
    );
  });

  it('matches problem.malformed-json for a body that is not JSON', async () => {
    const response = await request(app)
      .post('/api/v1/statistics')
      .set('Authorization', bearer())
      .type('application/json')
      .send('{"matrices": [');

    expect(response.status).toBe(400);
    expect(response.headers['content-type']).toBe('application/problem+json');
    expect(stripVolatile(response.body)).toEqual(
      stripVolatile(fixture('problem.malformed-json.json')),
    );
  });
});
