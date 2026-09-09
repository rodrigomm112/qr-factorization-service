import { describe, expect, it } from 'vitest';
import { PROBLEM_UNAUTHORIZED, PROBLEM_VALIDATION_ERROR } from '../test/fixtures';
import {
  ApiError,
  isProblemDetails,
  isUnauthorized,
  problemMessage,
  problemSlug,
  toApiError,
} from './problem';

describe('problem', () => {
  it('maps a problem document to Spanish copy and keeps the validation detail', () => {
    expect(isProblemDetails(PROBLEM_VALIDATION_ERROR)).toBe(true);
    expect(problemSlug(PROBLEM_VALIDATION_ERROR)).toBe('validation-error');

    const error = toApiError(422, PROBLEM_VALIDATION_ERROR, 'header-id');

    expect(error).toBeInstanceOf(ApiError);
    expect(error.kind).toBe('http');
    expect(error.status).toBe(422);
    expect(error.message).toBe('La solicitud no superó la validación del servidor.');
    expect(error.issues).toEqual([
      { pointer: '/matrix/1', code: 'ragged_row', message: 'row 1 has 3 columns, expected 2' },
    ]);
    expect(error.requestId).toBe('00000000-0000-4000-8000-000000000000');
  });

  it('translates 401 into the credentials message', () => {
    expect(problemMessage(PROBLEM_UNAUTHORIZED)).toMatch(/^Credenciales inválidas/);
  });

  it('falls back to the status code when the body is not a problem document', () => {
    for (const body of ['<html>502 Bad Gateway</html>', null, undefined, { foo: 'bar' }, 42]) {
      expect(isProblemDetails(body)).toBe(false);
    }

    const proxyError = toApiError(502, '<html>502 Bad Gateway</html>', 'abc');
    expect(proxyError.problem).toBeUndefined();
    expect(proxyError.message).toBe('La API respondió con un error inesperado.');
    expect(proxyError.requestId).toBe('abc');
    expect(proxyError.issues).toEqual([]);

    expect(toApiError(401, undefined, undefined).message).toMatch(/^Credenciales inválidas/);
    expect(toApiError(404, { foo: 'bar' }, undefined).message).toBe('La API rechazó la solicitud.');
  });

  it('requires every member the contract always sends, timestamp included', () => {
    for (const member of ['type', 'title', 'status', 'instance', 'requestId', 'timestamp']) {
      const incomplete = Object.fromEntries(
        Object.entries(PROBLEM_VALIDATION_ERROR).filter(([key]) => key !== member),
      );
      expect(isProblemDetails(incomplete)).toBe(false);
    }
    // A body that only looks like a problem is rendered from its status instead.
    const undated = { ...PROBLEM_VALIDATION_ERROR, timestamp: undefined };
    expect(toApiError(422, undated, 'abc').problem).toBeUndefined();
  });

  it('keeps an unknown problem type usable by falling back to its title', () => {
    const unknown = { ...PROBLEM_VALIDATION_ERROR, type: 'urn:proyectot:problem:teapot' };
    expect(problemMessage(unknown)).toBe('The request body failed validation');
  });

  it('does not read a message off Object.prototype for a prototype-named slug', () => {
    // `slug in MESSAGES` returned a function for "constructor"/"toString"; `hasOwn` does not.
    const inherited = {
      ...PROBLEM_VALIDATION_ERROR,
      type: 'urn:proyectot:problem:constructor',
      title: 'Fabricado por un proxy',
    };
    expect(problemMessage(inherited)).toBe('Fabricado por un proxy');
  });

  it('ends the session only on 401', () => {
    expect(isUnauthorized(toApiError(401, PROBLEM_UNAUTHORIZED, 'abc'))).toBe(true);
    expect(isUnauthorized(toApiError(422, PROBLEM_VALIDATION_ERROR, 'abc'))).toBe(false);
    expect(isUnauthorized(new Error('boom'))).toBe(false);
  });
});
