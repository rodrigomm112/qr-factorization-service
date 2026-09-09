import express from 'express';
import type { Express } from 'express';
import request from 'supertest';
import { describe, expect, it } from 'vitest';
import { errorMiddleware } from '../../src/adapters/inbound/http/middleware/error.middleware.js';
import { requestIdMiddleware } from '../../src/adapters/inbound/http/middleware/request-id.middleware.js';
import { createLogger } from '../../src/platform/logger.js';
import { testConfig } from '../support/harness.js';

/** Minimal app that lets a specific failure reach the error middleware. */
function appThrowing(error: unknown): Express {
  const config = testConfig();
  const app = express();
  app.use(requestIdMiddleware());
  app.get('/boom', () => {
    throw error;
  });
  app.use(errorMiddleware(createLogger(config), config.maxBodyBytes));
  return app;
}

function taggedError(type: string): Error {
  return Object.assign(new Error(type), { type });
}

function namedError(name: string): Error {
  const error = new Error('token problem');
  error.name = name;
  return error;
}

describe('error middleware mapping', () => {
  const cases: ReadonlyArray<readonly [unknown, number, string]> = [
    [taggedError('entity.parse.failed'), 400, 'malformed-json'],
    [taggedError('entity.verify.failed'), 400, 'malformed-json'],
    [taggedError('request.aborted'), 400, 'malformed-json'],
    [taggedError('entity.too.large'), 413, 'payload-too-large'],
    [taggedError('request.size.invalid'), 413, 'payload-too-large'],
    [taggedError('encoding.unsupported'), 415, 'unsupported-media-type'],
    [taggedError('charset.unsupported'), 415, 'unsupported-media-type'],
    [namedError('JsonWebTokenError'), 401, 'unauthorized'],
    [namedError('TokenExpiredError'), 401, 'unauthorized'],
    [namedError('NotBeforeError'), 401, 'unauthorized'],
    [new Error('anything else'), 500, 'internal-error'],
    ['a thrown string', 500, 'internal-error'],
  ];

  it.each(cases)('maps %o to %i', async (error, status, slug) => {
    const response = await request(appThrowing(error)).get('/boom');

    expect(response.status).toBe(status);
    expect(response.body.type).toBe(`urn:proyectot:problem:${slug}`);
    expect(response.headers['content-type']).toBe('application/problem+json');
  });

  it('advertises invalid_token when a JWT error escapes the auth middleware', async () => {
    const response = await request(appThrowing(namedError('TokenExpiredError'))).get('/boom');
    expect(response.headers['www-authenticate']).toBe('Bearer realm="proyectot", error="invalid_token"');
  });

  it('quotes the configured limit in the 413 detail', async () => {
    const response = await request(appThrowing(taggedError('entity.too.large'))).get('/boom');
    expect(response.body.detail).toBe('The request body exceeds the 4194304 byte limit.');
  });
});
