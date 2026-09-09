import request from 'supertest';
import { describe, expect, it } from 'vitest';
import { bearer, createTestApp, fixture } from '../support/harness.js';

interface ExpectedIssue {
  readonly pointer: string;
  readonly code: string;
}

const rows = (count: number, length = 2): number[][] =>
  Array.from({ length: count }, () => Array.from({ length }, () => 1));

const cases: ReadonlyArray<readonly [string, unknown, ExpectedIssue]> = [
  ['matrices is absent', {}, { pointer: '/matrices', code: 'missing_field' }],
  ['matrices is not an array', { matrices: 'nope' }, { pointer: '/matrices', code: 'invalid_type' }],
  ['matrices is empty', { matrices: [] }, { pointer: '/matrices', code: 'empty_matrix' }],
  [
    'there are more matrices than allowed',
    { matrices: Array.from({ length: 9 }, () => [[1]]) },
    { pointer: '/matrices', code: 'too_many_matrices' },
  ],
  [
    'an item is neither an object nor an array',
    { matrices: [42] },
    { pointer: '/matrices/0', code: 'invalid_type' },
  ],
  [
    'a canonical item has no values',
    { matrices: [{ label: 'Q' }] },
    { pointer: '/matrices/0/values', code: 'missing_field' },
  ],
  [
    'the label is not a string',
    { matrices: [{ label: 7, values: [[1]] }] },
    { pointer: '/matrices/0/label', code: 'invalid_type' },
  ],
  [
    'the label is longer than 64 characters',
    { matrices: [{ label: 'x'.repeat(65), values: [[1]] }] },
    { pointer: '/matrices/0/label', code: 'invalid_type' },
  ],
  [
    'values is not an array',
    { matrices: [{ label: 'Q', values: 5 }] },
    { pointer: '/matrices/0/values', code: 'invalid_type' },
  ],
  ['a matrix has no rows', { matrices: [[]] }, { pointer: '/matrices/0', code: 'empty_matrix' }],
  ['a row has no columns', { matrices: [[[]]] }, { pointer: '/matrices/0/0', code: 'empty_row' }],
  [
    'a row is not an array',
    { matrices: [[[1, 2], 'x']] },
    { pointer: '/matrices/0/1', code: 'invalid_type' },
  ],
  [
    'the matrix is ragged (canonical form)',
    { matrices: [{ label: 'Q', values: [[1, 2], [3]] }] },
    { pointer: '/matrices/0/values/1', code: 'ragged_row' },
  ],
  [
    'the matrix is ragged (shorthand form)',
    { matrices: [[[1, 2], [3]]] },
    { pointer: '/matrices/0/1', code: 'ragged_row' },
  ],
  [
    'a cell is null',
    { matrices: [{ label: 'Q', values: [[1, 2], [3, null]] }] },
    { pointer: '/matrices/0/values/1/1', code: 'invalid_type' },
  ],
  [
    'a cell is a string',
    { matrices: [[[1, '2']]] },
    { pointer: '/matrices/0/0/1', code: 'invalid_type' },
  ],
  [
    'a cell is a boolean',
    { matrices: [[[1, true]]] },
    { pointer: '/matrices/0/0/1', code: 'invalid_type' },
  ],
  [
    'a cell is an object',
    { matrices: [[[1, {}]]] },
    { pointer: '/matrices/0/0/1', code: 'invalid_type' },
  ],
  [
    'a cell is an array',
    { matrices: [[[1, [2]]]] },
    { pointer: '/matrices/0/0/1', code: 'invalid_type' },
  ],
  [
    'there are more rows than allowed',
    { matrices: [rows(101)] },
    { pointer: '/matrices/0', code: 'too_many_rows' },
  ],
  [
    'there are more columns than allowed',
    { matrices: [[Array.from({ length: 101 }, () => 1)]] },
    { pointer: '/matrices/0/0', code: 'too_many_cols' },
  ],
  [
    'the body is a JSON array instead of an object',
    [1, 2, 3],
    { pointer: '', code: 'invalid_type' },
  ],
];

describe('422 validation-error', () => {
  const { app } = createTestApp();

  it.each(cases)('reports %s', async (_name, body, expected) => {
    const response = await request(app)
      .post('/api/v1/statistics')
      .set('Authorization', bearer())
      .send(body as object);

    expect(response.status).toBe(422);
    expect(response.headers['content-type']).toBe('application/problem+json');
    expect(response.body.type).toBe('urn:proyectot:problem:validation-error');
    expect(response.body.title).toBe('The request body failed validation');
    expect(response.body.errors).toHaveLength(1);
    expect(response.body.errors[0]).toMatchObject(expected);
    expect(typeof response.body.errors[0].message).toBe('string');
  });

  it('produces the envelope of the shared golden fixture', async () => {
    const response = await request(app)
      .post('/api/v1/statistics')
      .set('Authorization', bearer())
      .send({ matrices: [{ label: 'Q', values: [[1, 2], [3]] }] });

    const golden = fixture('problem.validation-error.json');
    expect(Object.keys(response.body).sort()).toEqual(Object.keys(golden).sort());
    expect(response.body.status).toBe(422);
    expect(response.body.instance).toBe('/api/v1/statistics');
    // The wording of the OpenAPI example, singular "column" included.
    expect(response.body.detail).toBe(
      'matrices[0].values must be rectangular: row 1 has 1 column, expected 2',
    );
    expect(response.body.errors[0]).toEqual({
      pointer: '/matrices/0/values/1',
      code: 'ragged_row',
      message: 'row 1 has 1 column, expected 2',
    });
  });

  it('pluralizes the ragged row message', async () => {
    const response = await request(app)
      .post('/api/v1/statistics')
      .set('Authorization', bearer())
      .send({ matrices: [[[1, 2], [3, 4, 5]]] });

    expect(response.body.errors[0].message).toBe('row 1 has 3 columns, expected 2');
  });

  it('reports every failure, ordered by pointer', async () => {
    const response = await request(app)
      .post('/api/v1/statistics')
      .set('Authorization', bearer())
      .send({
        matrices: [
          [[1, null], [2, 3]],
          [[1, 2], [3]],
          { values: [['x', 2]] },
        ],
      });

    expect(response.status).toBe(422);
    expect(response.body.errors.map((issue: ExpectedIssue) => issue.pointer)).toEqual([
      '/matrices/0/0/1',
      '/matrices/1/1',
      '/matrices/2/values/0/0',
    ]);
    expect(response.body.detail).toBe('The request body contains 3 validation errors.');
  });

  it('orders numeric pointer segments numerically, not lexicographically', async () => {
    const values = rows(12);
    values[2] = [1];
    values[10] = [1];
    const response = await request(app)
      .post('/api/v1/statistics')
      .set('Authorization', bearer())
      .send({ matrices: [values] });

    expect(response.body.errors.map((issue: ExpectedIssue) => issue.pointer)).toEqual([
      '/matrices/0/2',
      '/matrices/0/10',
    ]);
  });

  it('rejects a value that parses to Infinity with non_finite_value', async () => {
    const response = await request(app)
      .post('/api/v1/statistics')
      .set('Authorization', bearer())
      .type('application/json')
      .send('{"matrices":[[[1e999,0],[0,1]]]}');

    expect(response.status).toBe(422);
    expect(response.body.errors).toEqual([
      {
        pointer: '/matrices/0/0/0',
        code: 'non_finite_value',
        message: 'row 0, column 0 must be a finite number',
      },
    ]);
  });

  it('rejects the Infinity literal as malformed JSON, not as a validation error', async () => {
    const response = await request(app)
      .post('/api/v1/statistics')
      .set('Authorization', bearer())
      .type('application/json')
      .send('{"matrices":[[[Infinity,0],[0,1]]]}');

    expect(response.status).toBe(400);
    expect(response.headers['content-type']).toBe('application/problem+json');
    expect(response.body).toMatchObject({
      type: 'urn:proyectot:problem:malformed-json',
      title: 'Malformed JSON body',
      status: 400,
      detail: 'The request body is not valid JSON.',
      instance: '/api/v1/statistics',
    });
    expect(response.body).not.toHaveProperty('errors');
  });

  it('rejects the NaN literal as malformed JSON', async () => {
    const response = await request(app)
      .post('/api/v1/statistics')
      .set('Authorization', bearer())
      .type('application/json')
      .send('{"matrices":[[[NaN]]]}');

    expect(response.status).toBe(400);
  });

  it('rejects more elements than the configured budget', async () => {
    // 8 x 100 x 100 = 80 000 is under the production budget of 100 000; a lower one, same path.
    const { app: bounded } = createTestApp({ MAX_TOTAL_ELEMENTS: '50' });
    const response = await request(bounded)
      .post('/api/v1/statistics')
      .set('Authorization', bearer())
      .send({ matrices: [rows(6, 6), rows(6, 6)] });

    expect(response.status).toBe(422);
    expect(response.body.errors).toEqual([
      {
        pointer: '/matrices',
        code: 'too_many_elements',
        message: 'matrices contain 72 elements in total, at most 50 are allowed',
      },
    ]);
  });

  describe('numerical_overflow', () => {
    const overflowCases: ReadonlyArray<readonly [string, unknown]> = [
      ['two entries near MAX_VALUE in one matrix', { matrices: [[[1e308, 0], [0, 1e308]]] }],
      ['an exactly-zero total that still saturates the accumulator', { matrices: [[[1e308, 1e308], [-1e308, -1e308]]] }],
      [
        'the Q and R of an ill-scaled decomposition',
        {
          matrices: [
            { label: 'Q', values: [[1e308, 0], [0, 1e308]] },
            { label: 'R', values: [[1, 0], [0, 1]] },
          ],
        },
      ],
    ];

    it.each(overflowCases)('rejects %s with 422, never a null sum', async (_name, body) => {
      const response = await request(app)
        .post('/api/v1/statistics')
        .set('Authorization', bearer())
        .send(body as object);

      expect(response.status).toBe(422);
      expect(response.headers['content-type']).toBe('application/problem+json');
      expect(response.body.type).toBe('urn:proyectot:problem:validation-error');
      expect(response.body.errors).toEqual([
        {
          pointer: '/matrices',
          code: 'numerical_overflow',
          message: 'the sum of the values overflows the double range',
        },
      ]);
      expect(JSON.stringify(response.body)).not.toContain('null');
    });

    it('still answers 200 for a total that fits', async () => {
      const response = await request(app)
        .post('/api/v1/statistics')
        .set('Authorization', bearer())
        .send({ matrices: [[[1e308, 0], [0, -1e308]]] });

      expect(response.status).toBe(200);
      expect(response.body.summary.sum).toBe(0);
    });
  });

  it('caps the number of reported issues', async () => {
    const values = Array.from({ length: 50 }, () => Array.from({ length: 50 }, () => null));
    const response = await request(app)
      .post('/api/v1/statistics')
      .set('Authorization', bearer())
      .send({ matrices: [values] });

    expect(response.status).toBe(422);
    expect(response.body.errors).toHaveLength(200);
    expect(response.body.detail).toBe(
      'The request body contains 2500 validation errors; the first 200 are listed.',
    );
  });
});
