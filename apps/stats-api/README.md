# stats-api

Descriptive statistics over one or more rectangular matrices — Node 24, Express 5, TypeScript 6, ESM.
It knows nothing about QR: `qr-api` is just one of its clients (it posts `[Q, R]`).

## Endpoints

| Method | Path | Auth | Notes |
|---|---|---|---|
| `POST` | `/api/v1/statistics` | Bearer JWT | 1..8 matrices, `{label, values}` or bare `number[][]` |
| `GET` | `/health/live`, `/health/ready` | — | `application/health+json`, outside the rate limiter |
| `GET` | `/openapi.yaml`, `/docs` | — | The contract and Swagger UI (bundled, no CDN) |

Errors follow RFC 9457 (`application/problem+json`); the registry is shared with qr-api (same slugs and titles).
Requests and responses are frozen in `contracts/examples/` and asserted by the test suite.

## Run

```bash
npm ci
JWT_SECRET=$(openssl rand -base64 48) npm run dev   # tsx watch, http://localhost:3000
docker build -t proyectot/stats-api:local .         # non-root, HEALTHCHECK included
```

## Configuration

Plain environment names only — `PORT`, `APP_ENV`, `LOG_LEVEL`, `JWT_SECRET` (>= 32 chars, the only
value with no default), `JWT_ISSUER`, `JWT_AUDIENCE`, `CORS_ALLOWED_ORIGINS`, `MAX_BODY_BYTES`,
`RATE_LIMIT_MAX`, `RATE_LIMIT_WINDOW_MS`, `DIAGONAL_ABS_TOLERANCE`, `DIAGONAL_REL_TOLERANCE`,
`MAX_MATRICES`, `MAX_TOTAL_ELEMENTS`, `MAX_MATRIX_ROWS`, `MAX_MATRIX_COLS`, `TRUST_PROXY`,
`SHUTDOWN_TIMEOUT_MS`. Unknown keys are ignored on purpose, so one `.env` can feed both services
(`docker-compose.yml` maps `STATS_*` onto the names above). Invalid values fail fast at startup.

`SHUTDOWN_TIMEOUT_MS` (default `10000`) is the grace period given to in-flight requests after
`SIGTERM` / `SIGINT`: the server stops accepting connections, drains, and hard-exits once the
deadline passes. It is the Node counterpart of qr-api's `SHUTDOWN_TIMEOUT` and is read under this
plain name, never prefixed with `STATS_`.

## Errors, auth and hardening

- Every failure is an RFC 9457 document, including the ones Express never sees: a header block
  above Node's `maxHeaderSize` (16 KiB) is answered `431 request-header-fields-too-large`, written
  straight onto the socket by the `clientError` hook installed in `src/main.ts`.
- A sum that saturates the double range comes from the caller's values, so it is a
  `422 validation-error` with `errors[0].code = numerical_overflow` — never a `500`.
- The bearer JWT must carry `exp`: `jsonwebtoken` only checks the claim when it is present, so a
  token minted without one is rejected here too (qr-api requires it as well). `alg` is pinned to
  HS256, `iss` and `aud` are verified, with 30 s of clock leeway.
- CSP is `default-src 'none'` on the API surface. `/docs` gets its own policy in which only
  `style-src` allows `'unsafe-inline'`; Swagger UI's bootstrap is an external `swagger-ui-init.js`,
  so `script-src` stays `'self'`. Answers on `/api/v1` carry the draft-8 `RateLimit` /
  `RateLimit-Policy` pair in qr-api's exact spelling (`"default";q=120;w=60`), with no
  partition key.

## Numerics

- `sum` uses **Neumaier** compensated summation: `[1e16, 1, -1e16]` returns `1`, the naive loop `0`.
  Per-matrix accumulators are merged unevaluated, so the global total loses nothing.
- `isDiagonal` is the general `m x n` definition with a hybrid tolerance
  `max(absTol, relTol * max|a_ij|)`: scale invariant against the `O(||A||·u)` residue of an upstream
  QR, while still meaningful for the zero matrix.

## Layout and tests

`domain` (pure) <- `application` (use case) <- `adapters/inbound/http` + `platform`, composed in
`src/main.ts`. `buildApp()` returns an app without listening, which is what the integration suite
drives through supertest: `npm run typecheck && npm run lint && npm run test:coverage`.

## Version and preflight notes

- `/health/live` reports `APP_VERSION` when the image was built with
  `--build-arg VERSION=<git sha>` (the deploy script and compose do this), so both
  APIs expose the same deployed commit; otherwise the `package.json` version.
- A CORS preflight from an origin outside the allow-list is answered with an empty
  `204` and no CORS headers, before the rate limiter and the router, exactly like
  qr-api; it never consumes rate-limit quota.
