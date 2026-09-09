import request from 'supertest';
import { describe, expect, it, vi } from 'vitest';
import { bearer, createTestApp } from '../support/harness.js';

vi.mock('../../src/application/compute-statistics.usecase.js', () => ({
  computeStatistics: () => {
    throw new Error('boom: simulated failure inside the use case');
  },
}));

describe('unexpected failures', () => {
  it('answers a bare 500 and logs the stack trace with the requestId', async () => {
    const lines: string[] = [];
    const { app } = createTestApp({ LOG_LEVEL: 'error' }, {
      write: (chunk: string): void => {
        lines.push(chunk);
      },
    });

    const response = await request(app)
      .post('/api/v1/statistics')
      .set('Authorization', bearer())
      .send({ matrices: [[[1]]] });

    expect(response.status).toBe(500);
    expect(response.headers['content-type']).toBe('application/problem+json');
    expect(response.body).toMatchObject({
      type: 'urn:proyectot:problem:internal-error',
      title: 'Internal server error',
      status: 500,
      detail: 'An unexpected error occurred. Quote the requestId when reporting it.',
      instance: '/api/v1/statistics',
    });

    const serialized = JSON.stringify(response.body);
    expect(serialized).not.toContain('boom');
    expect(serialized).not.toContain('at ');
    expect(response.body).not.toHaveProperty('stack');
    expect(response.body).not.toHaveProperty('errors');

    const logged = lines
      .map((line) => JSON.parse(line) as Record<string, unknown>)
      .find((entry) => entry['msg'] === 'unhandled error while serving request');

    expect(logged).toBeDefined();
    expect(logged?.['requestId']).toBe(response.body.requestId);
    const error = logged?.['err'] as { message: string; stack: string };
    expect(error.message).toBe('boom: simulated failure inside the use case');
    expect(error.stack).toContain('Error: boom: simulated failure inside the use case');
  });
});
