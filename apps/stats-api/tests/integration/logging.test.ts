import request from 'supertest';
import { describe, expect, it } from 'vitest';
import { bearer, createTestApp, mintToken } from '../support/harness.js';

function collector(): { lines: string[]; write: (chunk: string) => void } {
  const lines: string[] = [];
  return { lines, write: (chunk: string): void => void lines.push(chunk) };
}

describe('structured logging', () => {
  it('never writes the bearer token or the request body to the log', async () => {
    const sink = collector();
    const { app } = createTestApp({ LOG_LEVEL: 'info' }, sink);
    const token = mintToken();

    const response = await request(app)
      .post('/api/v1/statistics')
      .set('Authorization', `Bearer ${token}`)
      .send({ matrices: [{ label: 'secret-matrix', values: [[98765432.1, 0], [0, 7]] }] });

    expect(response.status).toBe(200);
    const log = sink.lines.join('\n');
    expect(log).not.toContain(token);
    expect(log).toContain('[redacted]');
    expect(log).not.toContain('secret-matrix');
    expect(log).not.toContain('98765432');
  });

  it('correlates the access log line with the response header', async () => {
    const sink = collector();
    const { app } = createTestApp({ LOG_LEVEL: 'info' }, sink);

    const response = await request(app)
      .post('/api/v1/statistics')
      .set('Authorization', bearer())
      .send({ matrices: [[[1]]] });

    const entries = sink.lines.map((line) => JSON.parse(line) as Record<string, unknown>);
    const completed = entries.find((entry) => entry['req'] !== undefined);
    const loggedRequest = completed?.['req'] as { id: string; headers: Record<string, string> };
    expect(loggedRequest.id).toBe(response.headers['x-request-id']);
    expect(loggedRequest.headers['authorization']).toBe('[redacted]');
    expect(completed?.['level']).toBe('info');
    expect(completed?.['service']).toBe('stats-api');
  });

  it('logs client errors at warn level', async () => {
    const sink = collector();
    const { app } = createTestApp({ LOG_LEVEL: 'info' }, sink);

    await request(app).post('/api/v1/statistics').send({});

    const levels = sink.lines
      .map((line) => JSON.parse(line) as Record<string, unknown>)
      .map((entry) => entry['level']);
    expect(levels).toContain('warn');
  });

  it('keeps the health probes out of the access log', async () => {
    const sink = collector();
    const { app } = createTestApp({ LOG_LEVEL: 'info' }, sink);

    await request(app).get('/health/ready').expect(200);
    expect(sink.lines).toHaveLength(0);
  });
});
