import { createServer } from 'node:http';
import type { AddressInfo } from 'node:net';
import { connect } from 'node:net';
import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import { installClientErrorHandler } from '../../src/adapters/inbound/http/client-error.js';
import { createLogger } from '../../src/platform/logger.js';
import { createTestApp, testConfig } from '../support/harness.js';

// Node's HTTP client refuses to build a request this large and supertest never sees the socket.
function rawRequest(port: number, request: string): Promise<string> {
  return new Promise((resolve, reject) => {
    const socket = connect(port, '127.0.0.1', () => {
      socket.write(request);
    });
    let received = '';
    socket.setEncoding('utf8');
    socket.on('data', (chunk: string) => {
      received += chunk;
    });
    socket.on('end', () => {
      resolve(received);
    });
    socket.on('error', reject);
  });
}

describe('requests rejected by the HTTP parser', () => {
  const lines: string[] = [];
  // The rest of the suite runs silent; here the log level is what the assertion is about.
  const logger = createLogger(testConfig({ LOG_LEVEL: 'warn' }), {
    write(line: string) {
      lines.push(line);
    },
  });
  const { app } = createTestApp();
  const server = createServer(app);
  installClientErrorHandler(server, logger);

  beforeAll(async () => {
    await new Promise<void>((resolve) => {
      server.listen(0, '127.0.0.1', resolve);
    });
  });

  afterAll(async () => {
    await new Promise<void>((resolve, reject) => {
      server.close((error) => {
        if (error) reject(error);
        else resolve();
      });
    });
  });

  it('answers an oversized header block with the 431 problem document', async () => {
    const { port } = server.address() as AddressInfo;
    const response = await rawRequest(
      port,
      [
        'POST /api/v1/statistics?trace=1 HTTP/1.1',
        'Host: stats-api',
        'Content-Type: application/json',
        `Authorization: Bearer ${'x'.repeat(32 * 1024)}`,
        '',
        '',
      ].join('\r\n'),
    );

    expect(response).toContain('HTTP/1.1 431 Request Header Fields Too Large');
    expect(response).toContain('Content-Type: application/problem+json');

    const body: unknown = JSON.parse(response.slice(response.indexOf('\r\n\r\n') + 4));
    expect(body).toMatchObject({
      type: 'urn:proyectot:problem:request-header-fields-too-large',
      title: 'Request header fields too large',
      status: 431,
      // Recovered from the raw request line, without the query string.
      instance: '/api/v1/statistics',
    });
    expect(body).not.toHaveProperty('errors');

    const levels = lines.map((line) => (JSON.parse(line) as { level: string }).level);
    expect(levels).not.toContain('error');
    expect(levels).toContain('warn');
  });

  it('leaves a header block that fits to the application', async () => {
    const { port } = server.address() as AddressInfo;
    const response = await rawRequest(
      port,
      [
        'POST /api/v1/statistics HTTP/1.1',
        'Host: stats-api',
        'Content-Type: application/json',
        `Authorization: Bearer ${'x'.repeat(6 * 1024)}`,
        'Content-Length: 0',
        'Connection: close',
        '',
        '',
      ].join('\r\n'),
    );

    expect(response).toContain('HTTP/1.1 401 Unauthorized');
    expect(response).toContain('urn:proyectot:problem:unauthorized');
  });
});
