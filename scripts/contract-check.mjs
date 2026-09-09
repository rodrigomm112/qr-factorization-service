#!/usr/bin/env node
// contract-check.mjs — the live stack answers what the OpenAPI documents promise.
//
//   make contract-check
//   node scripts/contract-check.mjs [--base-qr URL] [--base-stats URL]
//                                   [--client-id ID] [--client-secret SECRET]
//
// Checks status, Content-Type, the echoed X-Request-ID and the WHOLE body of every response
// against `components.schemas`, where e2e-smoke.sh only asserts a few fields with jq.
// Credentials: E2E_CLIENT_ID/E2E_CLIENT_SECRET, else .env.demo-credentials. Never printed.
// Deps pinned in scripts/package.json. Exit 0 only if every check passes.
import { readFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';
import Ajv2020 from 'ajv/dist/2020.js';
import addFormats from 'ajv-formats';
import { parse as parseYaml } from 'yaml';

const ROOT_DIR = join(dirname(fileURLToPath(import.meta.url)), '..');
const EXAMPLES_DIR = join(ROOT_DIR, 'docs', 'contracts', 'examples');
const CREDENTIALS_FILE = join(ROOT_DIR, '.env.demo-credentials');

const useColor = process.stdout.isTTY && !process.env.NO_COLOR;
const C = {
  reset: useColor ? '\u001B[0m' : '',
  dim: useColor ? '\u001B[2m' : '',
  ok: useColor ? '\u001B[32m' : '',
  bad: useColor ? '\u001B[1;31m' : '',
  head: useColor ? '\u001B[1;36m' : '',
};

// ---- arguments --------------------------------------------------------------

const options = {
  baseQr: process.env.BASE_QR ?? 'http://localhost:8080',
  baseStats: process.env.BASE_STATS ?? 'http://localhost:3000',
  clientId: process.env.E2E_CLIENT_ID ?? '',
  clientSecret: process.env.E2E_CLIENT_SECRET ?? '',
};

const argv = process.argv.slice(2);
while (argv.length > 0) {
  const flag = argv.shift();
  const value = () => {
    const next = argv.shift();
    if (next === undefined) fatal(`${flag} needs a value`);
    return next;
  };
  switch (flag) {
    case '--base-qr':
      options.baseQr = value().replace(/\/+$/, '');
      break;
    case '--base-stats':
      options.baseStats = value().replace(/\/+$/, '');
      break;
    case '--client-id':
      options.clientId = value();
      break;
    case '--client-secret':
      options.clientSecret = value();
      break;
    case '-h':
    case '--help':
      printUsage();
      process.exit(0);
      break;
    default:
      fatal(`unknown argument: ${flag}`);
  }
}

function fatal(message) {
  process.stderr.write(`${C.bad}[error]${C.reset} ${message}\n`);
  process.exit(2);
}

function printUsage() {
  const source = readFileSync(fileURLToPath(import.meta.url), 'utf8');
  for (const line of source.split('\n').slice(1)) {
    if (!line.startsWith('//')) break;
    process.stdout.write(`${line.replace(/^\/\/ ?/, '')}\n`);
  }
}

// ---- credentials (never printed) -------------------------------------------

if (options.clientId === '' || options.clientSecret === '') {
  let content = '';
  try {
    content = readFileSync(CREDENTIALS_FILE, 'utf8');
  } catch {
    fatal(
      'no credentials: export E2E_CLIENT_ID/E2E_CLIENT_SECRET or run ./scripts/gen-secrets.sh',
    );
  }
  const read = (key) => {
    const prefix = `${key}=`;
    const line = content.split('\n').find((candidate) => candidate.trim().startsWith(prefix));
    return line === undefined ? '' : line.trim().slice(prefix.length);
  };
  if (options.clientId === '') options.clientId = read('AUTH_CLIENT_ID');
  if (options.clientSecret === '') options.clientSecret = read('AUTH_CLIENT_SECRET');
}
if (options.clientId === '' || options.clientSecret === '') {
  fatal(`${CREDENTIALS_FILE} has no AUTH_CLIENT_ID / AUTH_CLIENT_SECRET`);
}

// One Ajv 2020-12 schema per document, `$id` a URN so its own `$ref: '#/components/schemas/X'`
// resolves untouched. `strict: false`: OpenAPI carries annotations Ajv does not know.
function loadSchemas(relativePath, id) {
  const document = parseYaml(readFileSync(join(ROOT_DIR, relativePath), 'utf8'));
  const schemas = document?.components?.schemas;
  if (schemas === undefined) fatal(`${relativePath}: no components.schemas`);

  const ajv = new Ajv2020({ strict: false, allErrors: true });
  addFormats(ajv);
  ajv.addSchema({ $id: id, components: { schemas } }, id);

  return (name) => {
    const validate = ajv.getSchema(`${id}#/components/schemas/${name}`);
    if (validate === undefined) fatal(`${relativePath}: no schema named ${name}`);
    return validate;
  };
}

const qrSchema = loadSchemas('apps/qr-api/openapi.yaml', 'urn:proyectot:openapi:qr-api');
const statsSchema = loadSchemas('apps/stats-api/openapi.yaml', 'urn:proyectot:openapi:stats-api');

const example = (file) => JSON.parse(readFileSync(join(EXAMPLES_DIR, file), 'utf8'));

const newRequestId = () => crypto.randomUUID();

async function call({ method = 'GET', url, body, rawBody, token, contentType, headers = {} }) {
  const requestId = newRequestId();
  const requestHeaders = new Headers({ Accept: 'application/json', 'X-Request-ID': requestId });
  const payload = rawBody ?? (body === undefined ? undefined : JSON.stringify(body));
  if (payload !== undefined) requestHeaders.set('Content-Type', contentType ?? 'application/json');
  if (token !== undefined) requestHeaders.set('Authorization', `Bearer ${token}`);
  for (const [name, value] of Object.entries(headers)) requestHeaders.set(name, value);

  const response = await fetch(url, {
    method,
    headers: requestHeaders,
    ...(payload === undefined ? {} : { body: payload }),
  });
  const text = await response.text();
  let parsed;
  try {
    parsed = JSON.parse(text);
  } catch {
    parsed = undefined;
  }
  return {
    sentRequestId: requestId,
    status: response.status,
    contentType: response.headers.get('content-type') ?? '',
    requestId: response.headers.get('x-request-id'),
    body: parsed,
    text,
  };
}

// ---- assertions -------------------------------------------------------------

const formatErrors = (errors) =>
  (errors ?? [])
    .slice(0, 4)
    .map((error) => `${error.instancePath === '' ? '/' : error.instancePath} ${error.message}`)
    .join('; ');

/** Everything wrong with one response; empty means it passed. */
function verify(response, { status, contentType, schema, echoesRequestId = true }) {
  const problems = [];
  if (response.status !== status) {
    problems.push(`status ${response.status} != ${status}`);
  }
  if (!response.contentType.toLowerCase().startsWith(contentType)) {
    problems.push(`content-type "${response.contentType}" is not ${contentType}`);
  }
  if (echoesRequestId && response.requestId !== response.sentRequestId) {
    problems.push(`X-Request-ID "${response.requestId ?? '(absent)'}" was not echoed`);
  }
  if (schema !== undefined) {
    if (response.body === undefined) {
      problems.push(`body is not JSON: ${response.text.slice(0, 80)}`);
    } else if (!schema(response.body)) {
      problems.push(`schema: ${formatErrors(schema.errors)}`);
    }
  }
  return problems;
}

const JSON_TYPE = 'application/json';
const PROBLEM_TYPE = 'application/problem+json';
const HEALTH_TYPE = 'application/health+json';

const results = [];

async function check(service, name, expectation, run) {
  const row = { service, name, expectation, problems: [] };
  results.push(row);
  try {
    row.problems = (await run()) ?? [];
  } catch (error) {
    row.problems = [`threw: ${error instanceof Error ? error.message : String(error)}`];
  }
  const mark = row.problems.length === 0 ? `${C.ok}✓${C.reset}` : `${C.bad}✗${C.reset}`;
  process.stdout.write(`  ${mark} ${service} · ${name}\n`);
  for (const problem of row.problems) {
    process.stdout.write(`      ${C.bad}${problem}${C.reset}\n`);
  }
}

process.stdout.write(
  `${C.head}contract-check${C.reset} ${C.dim}qr-api ${options.baseQr} · stats-api ${options.baseStats}${C.reset}\n\n`,
);

let token = '';

await check('qr-api', 'POST /api/v1/auth/token', '200 TokenResponse', async () => {
  const response = await call({
    method: 'POST',
    url: `${options.baseQr}/api/v1/auth/token`,
    body: { clientId: options.clientId, clientSecret: options.clientSecret },
  });
  token = response.body?.accessToken ?? '';
  return verify(response, {
    status: 200,
    contentType: JSON_TYPE,
    schema: qrSchema('TokenResponse'),
  });
});

if (token === '') {
  process.stderr.write(
    `\n${C.bad}no access token: every authenticated check would fail for the same reason${C.reset}\n`,
  );
  process.exit(1);
}

await check('qr-api', 'POST /api/v1/qr (tall 3×2, full)', '200 QrResponse', async () => {
  const response = await call({
    method: 'POST',
    url: `${options.baseQr}/api/v1/qr`,
    token,
    body: { ...example('qr.request.tall-3x2.json'), mode: 'full' },
  });
  const problems = verify(response, {
    status: 200,
    contentType: JSON_TYPE,
    schema: qrSchema('QrResponse'),
  });
  if (response.body?.meta?.mode !== 'full') problems.push('meta.mode is not "full"');
  if (response.body?.requestId !== response.sentRequestId) {
    problems.push('body.requestId differs from the X-Request-ID sent');
  }
  return problems;
});

await check('qr-api', 'POST /api/v1/qr (wide 2×3, reduced)', '200 QrResponse', async () => {
  const response = await call({
    method: 'POST',
    url: `${options.baseQr}/api/v1/qr`,
    token,
    body: { ...example('qr.request.wide-2x3.json'), mode: 'reduced' },
  });
  const problems = verify(response, {
    status: 200,
    contentType: JSON_TYPE,
    schema: qrSchema('QrResponse'),
  });
  // reduced: Q is m×k, R is k×n, k = min(m, n) = 2.
  if (JSON.stringify(response.body?.meta?.qShape) !== '[2,2]') {
    problems.push(`meta.qShape ${JSON.stringify(response.body?.meta?.qShape)} != [2,2]`);
  }
  return problems;
});

await check('qr-api', 'POST /api/v1/qr (ragged)', '422 ProblemDetails', async () => {
  const response = await call({
    method: 'POST',
    url: `${options.baseQr}/api/v1/qr`,
    token,
    body: example('qr.request.invalid-ragged.json'),
  });
  const problems = verify(response, {
    status: 422,
    contentType: PROBLEM_TYPE,
    schema: qrSchema('ProblemDetails'),
  });
  const codes = (response.body?.errors ?? []).map((issue) => issue.code);
  if (!codes.includes('ragged_row')) problems.push(`errors[].code ${codes.join(',')} has no ragged_row`);
  return problems;
});

await check('qr-api', 'POST /api/v1/qr (no token)', '401 ProblemDetails', async () => {
  const response = await call({
    method: 'POST',
    url: `${options.baseQr}/api/v1/qr`,
    body: example('qr.request.tall-3x2.json'),
  });
  const problems = verify(response, {
    status: 401,
    contentType: PROBLEM_TYPE,
    schema: qrSchema('ProblemDetails'),
  });
  if (response.body?.type !== 'urn:proyectot:problem:unauthorized') {
    problems.push(`type ${response.body?.type} is not urn:proyectot:problem:unauthorized`);
  }
  return problems;
});

await check('qr-api', 'POST /api/v1/qr (text/plain)', '415 ProblemDetails', async () => {
  const response = await call({
    method: 'POST',
    url: `${options.baseQr}/api/v1/qr`,
    token,
    rawBody: JSON.stringify(example('qr.request.tall-3x2.json')),
    contentType: 'text/plain',
  });
  const problems = verify(response, {
    status: 415,
    contentType: PROBLEM_TYPE,
    schema: qrSchema('ProblemDetails'),
  });
  if (response.body?.type !== 'urn:proyectot:problem:unsupported-media-type') {
    problems.push(`type ${response.body?.type} is not urn:proyectot:problem:unsupported-media-type`);
  }
  return problems;
});

await check('qr-api', 'GET /health/live', '200 HealthResponse', async () => {
  const response = await call({ url: `${options.baseQr}/health/live` });
  return verify(response, {
    status: 200,
    contentType: HEALTH_TYPE,
    schema: qrSchema('HealthResponse'),
  });
});

await check('qr-api', 'GET /health/ready', '200 HealthResponse', async () => {
  const response = await call({ url: `${options.baseQr}/health/ready` });
  const problems = verify(response, {
    status: 200,
    contentType: HEALTH_TYPE,
    schema: qrSchema('HealthResponse'),
  });
  if (response.body?.status !== 'pass') problems.push(`status ${response.body?.status} != pass`);
  return problems;
});

await check('stats-api', 'POST /api/v1/statistics (canonical)', '200 StatisticsResponse', async () => {
  const response = await call({
    method: 'POST',
    url: `${options.baseStats}/api/v1/statistics`,
    token,
    body: example('statistics.request.identity-3x3.json'),
  });
  const problems = verify(response, {
    status: 200,
    contentType: JSON_TYPE,
    schema: statsSchema('StatisticsResponse'),
  });
  if (response.body?.summary?.anyDiagonal !== true) {
    problems.push('summary.anyDiagonal is not true for two identity matrices');
  }
  return problems;
});

await check('stats-api', 'POST /api/v1/statistics (shorthand)', '200 StatisticsResponse', async () => {
  const response = await call({
    method: 'POST',
    url: `${options.baseStats}/api/v1/statistics`,
    token,
    body: example('statistics.request.shorthand-2x2.json'),
  });
  const problems = verify(response, {
    status: 200,
    contentType: JSON_TYPE,
    schema: statsSchema('StatisticsResponse'),
  });
  const labels = (response.body?.matrices ?? []).map((entry) => entry.label);
  if (labels.join(',') !== 'matrix-0,matrix-1') problems.push(`shorthand labels ${JSON.stringify(labels)} != [matrix-0, matrix-1]`);
  return problems;
});

await check('stats-api', 'POST /api/v1/statistics (ragged)', '422 ProblemDetails', async () => {
  const response = await call({
    method: 'POST',
    url: `${options.baseStats}/api/v1/statistics`,
    token,
    body: { matrices: [{ label: 'A', values: [[1, 2], [3]] }] },
  });
  const problems = verify(response, {
    status: 422,
    contentType: PROBLEM_TYPE,
    schema: statsSchema('ProblemDetails'),
  });
  const codes = (response.body?.errors ?? []).map((issue) => issue.code);
  if (!codes.includes('ragged_row')) problems.push(`errors[].code ${codes.join(',')} has no ragged_row`);
  return problems;
});

await check('stats-api', 'POST /api/v1/statistics (no token)', '401 ProblemDetails', async () => {
  const response = await call({
    method: 'POST',
    url: `${options.baseStats}/api/v1/statistics`,
    body: example('statistics.request.shorthand-2x2.json'),
  });
  return verify(response, {
    status: 401,
    contentType: PROBLEM_TYPE,
    schema: statsSchema('ProblemDetails'),
  });
});

await check('stats-api', 'GET /health/live', '200 HealthResponse', async () => {
  const response = await call({ url: `${options.baseStats}/health/live` });
  return verify(response, {
    status: 200,
    contentType: HEALTH_TYPE,
    schema: statsSchema('HealthResponse'),
  });
});

await check('stats-api', 'GET /health/ready', '200 HealthResponse', async () => {
  const response = await call({ url: `${options.baseStats}/health/ready` });
  const problems = verify(response, {
    status: 200,
    contentType: HEALTH_TYPE,
    schema: statsSchema('HealthResponse'),
  });
  if (response.body?.status !== 'pass') problems.push(`status ${response.body?.status} != pass`);
  return problems;
});

// ---- report -----------------------------------------------------------------

const columns = [
  { head: '#', get: (row, index) => String(index + 1) },
  { head: 'service', get: (row) => row.service },
  { head: 'check', get: (row) => row.name },
  { head: 'expected', get: (row) => row.expectation },
  { head: 'result', get: (row) => (row.problems.length === 0 ? 'ok' : row.problems.join(' | ')) },
];

const widths = columns.map((column, columnIndex) =>
  Math.max(
    column.head.length,
    ...results.map((row, index) => {
      const cell = columns[columnIndex].get(row, index);
      // The result column is the only one allowed to run long; it comes last.
      return columnIndex === columns.length - 1 ? 0 : cell.length;
    }),
  ),
);

const pad = (text, width) => text + ' '.repeat(Math.max(0, width - text.length));

process.stdout.write(`\n${C.head}${columns.map((c, i) => pad(c.head, widths[i])).join('  ')}${C.reset}\n`);
process.stdout.write(`${C.dim}${widths.map((width) => '-'.repeat(width)).join('  ')}${C.reset}\n`);
for (const [index, row] of results.entries()) {
  const failed = row.problems.length > 0;
  const line = columns.map((column, columnIndex) => pad(column.get(row, index), widths[columnIndex])).join('  ');
  process.stdout.write(`${failed ? C.bad : ''}${line}${failed ? C.reset : ''}\n`);
}

const failures = results.filter((row) => row.problems.length > 0).length;
process.stdout.write(
  `\n${failures === 0 ? C.ok : C.bad}${results.length - failures}/${results.length} checks passed${C.reset}\n`,
);
process.exit(failures === 0 ? 0 : 1);
