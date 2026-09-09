import request from 'supertest';
import { describe, expect, it } from 'vitest';
import { bearer, createTestApp, fixture, stripVolatile } from '../support/harness.js';

describe('POST /api/v1/statistics', () => {
  const { app } = createTestApp();

  it('matches the identity-3x3 golden fixture', async () => {
    const response = await request(app)
      .post('/api/v1/statistics')
      .set('Authorization', bearer())
      .send(fixture('statistics.request.identity-3x3.json'));

    expect(response.status).toBe(200);
    expect(response.headers['content-type']).toMatch(/^application\/json/);
    expect(stripVolatile(response.body)).toEqual(
      stripVolatile(fixture('statistics.response.identity-3x3.json')),
    );
  });

  it('matches the mixed-2x2 golden fixture', async () => {
    const response = await request(app)
      .post('/api/v1/statistics')
      .set('Authorization', bearer())
      .send(fixture('statistics.request.mixed-2x2.json'));

    expect(response.status).toBe(200);
    expect(stripVolatile(response.body)).toEqual(
      stripVolatile(fixture('statistics.response.mixed-2x2.json')),
    );
  });

  it('normalizes the shorthand form to matrix-<index> labels', async () => {
    const response = await request(app)
      .post('/api/v1/statistics')
      .set('Authorization', bearer())
      .send(fixture('statistics.request.shorthand-2x2.json'));

    expect(response.status).toBe(200);

    const expected = fixture('statistics.response.mixed-2x2.json');
    const relabelled = {
      ...expected,
      matrices: (expected['matrices'] as Array<Record<string, unknown>>).map((matrix, index) => ({
        ...matrix,
        label: `matrix-${String(index)}`,
      })),
    };
    expect(stripVolatile(response.body)).toEqual(stripVolatile(relabelled));
  });

  it('accepts the canonical and the shorthand form in the same request', async () => {
    const response = await request(app)
      .post('/api/v1/statistics')
      .set('Authorization', bearer())
      .send({
        matrices: [
          { label: 'Q', values: [[1, 0], [0, 1]] },
          [[2, 3], [0, 4]],
        ],
      });

    expect(response.status).toBe(200);
    expect(response.body.matrices.map((m: { label: string }) => m.label)).toEqual(['Q', 'matrix-1']);
    expect(response.body.summary).toEqual({
      max: 4,
      min: 0,
      sum: 11,
      average: 1.375,
      count: 8,
      anyDiagonal: true,
    });
  });

  it('echoes the requestId of the response header in the body', async () => {
    const response = await request(app)
      .post('/api/v1/statistics')
      .set('Authorization', bearer())
      .send({ matrices: [[[1]]] });

    expect(response.status).toBe(200);
    expect(response.body.requestId).toBe(response.headers['x-request-id']);
    expect(response.body.meta.summationAlgorithm).toBe('neumaier');
    expect(response.body.meta.elapsedMs).toBeGreaterThanOrEqual(0);
  });

  it('keeps Neumaier precision end to end', async () => {
    const response = await request(app)
      .post('/api/v1/statistics')
      .set('Authorization', bearer())
      .send({ matrices: [[[1e16, 1, -1e16]]] });

    expect(response.status).toBe(200);
    expect(response.body.summary.sum).toBe(1);
  });

  it('accepts a matrix at the configured maximum size', async () => {
    const values = Array.from({ length: 100 }, () => Array.from({ length: 100 }, () => 1));
    const response = await request(app)
      .post('/api/v1/statistics')
      .set('Authorization', bearer())
      .send({ matrices: [{ label: 'big', values }] });

    expect(response.status).toBe(200);
    expect(response.body.summary).toMatchObject({ count: 10_000, sum: 10_000, average: 1 });
  });
});
