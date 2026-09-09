# qr-api

Public entry point of Proyecto T. Factors a rectangular matrix as `A = Q·R` with
Householder reflections (implemented from scratch, no linear-algebra dependency), then
calls `stats-api` with `[Q, R]` and returns both in one document.

Go 1.27 · Fiber v3 · hexagonal · RFC 9457 errors · OpenAPI 3.1 at `/openapi.yaml` and `/docs`.

## Layout

```
cmd/qr-api/                 composition root + the "healthcheck" subcommand (distroless has no curl)
internal/domain/matrix/     Matrix value type (flat []float64) and the validation rules
internal/domain/qr/         Householder decomposition, LAPACK-style scaled 2-norm  <- zero dependencies
internal/application/       use cases (DecomposeAndAnalyze, IssueToken), ports, error taxonomy
internal/adapters/inbound/http/     Fiber app, handlers, dto, problem+json, middleware, embedded Swagger UI
internal/adapters/outbound/statsclient/   net/http client to stats-api (timeout, retry, header propagation)
internal/platform/          config (fail-fast), logging (slog JSON), security (HS256, SHA-256 secrets)
spec.go                     //go:embed openapi.yaml
```

Dependencies point inwards only: `domain` knows nothing, `application` knows the domain and its
own ports, the adapters know everything. `New(cfg, deps) *fiber.App` never listens, so the whole
API is exercised in-process by `app.Test`.

That rule is enforced by **`depguard`** in `.golangci.yml`, not by review: `internal/domain` may
import only the standard library (plus itself), `internal/application` the standard library plus
`internal/domain`, and neither may import `github.com/gofiber/...`, `github.com/golang-jwt/...`,
`net/http`, `internal/adapters/...` or `internal/platform/...`. A wrong import fails
`golangci-lint run ./...` in CI, with the reason spelled out.

## Run

```bash
export JWT_SECRET=$(head -c 48 /dev/urandom | base64)      # >= 32 bytes
export AUTH_CLIENT_SECRET_SHA256=$(printf 's3cret' | sha256sum | cut -c1-64)
export STATS_API_BASE_URL=http://localhost:3000
go run ./cmd/qr-api                                        # http://localhost:8080

docker build -t proyectot/qr-api:local --build-arg VERSION=1.0.0 .
```

```bash
TOKEN=$(curl -s localhost:8080/api/v1/auth/token -H 'Content-Type: application/json' \
  -d '{"clientId":"demo-client","clientSecret":"s3cret"}' | jq -r .accessToken)
curl -s localhost:8080/api/v1/qr -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' -d '{"matrix":[[1,2],[3,4],[5,6]]}' | jq
```

## Test

```bash
go test -race -cover ./...      # unit + HTTP integration through app.Test
golangci-lint run ./...         # v2 schema, see .golangci.yml
govulncheck ./...
go test -run=xxx -fuzz=FuzzDecompose -fuzztime=60s ./internal/domain/qr
go test -run '^$' -bench . -benchmem ./internal/domain/qr
FIXTURE_OUT=1 go test -run TestGenerateFixtures ./internal/domain/qr
```

`go test` runs each test binary with its **package directory** as the working directory, so the
regeneration path is relative to `internal/domain/qr`, not to `apps/qr-api`:
`FIXTURE_OUT=1` expands to `../../../../../contracts/examples` (five levels up to the
repository root). The two files it rewrites — `qr.output.tall-3x2.json` and
`statistics.request.qr-tall-3x2.json` — are asserted against the live HTTP response by
`internal/adapters/inbound/http`, so a silent drift fails a test.

## Environment

Read verbatim from the "Shared" and "qr-api" sections of the repository's `.env.example`;
everything except the three required values has a working default.

| Required | |
|---|---|
| `JWT_SECRET` | HS256 key, ≥ 32 bytes |
| `AUTH_CLIENT_SECRET_SHA256` | 64 lowercase hex characters — the SHA-256 of the demo client secret |
| `STATS_API_BASE_URL` | absolute URL of stats-api |

Optional, with their defaults: `APP_ENV=development` (`production` turns HSTS on and panic
stack traces off), `LOG_LEVEL=info`, `PORT=8080`, `TRUST_PROXY=false`, `JWT_ISSUER=qr-api`,
`JWT_AUDIENCE=qr-api,stats-api`, `JWT_TTL=1h`, `AUTH_CLIENT_ID=demo-client`,
`STATS_API_TIMEOUT=5s` and `STATS_API_RETRIES=1` (per attempt; retried on 5xx, timeouts and
connection errors only — never on 4xx), `REQUEST_BUDGET=12s` (see below), `MAX_MATRIX_ROWS=100`, `MAX_MATRIX_COLS=100`,
`MAX_BODY_BYTES=1048576`, `RATE_LIMIT_MAX=60` and `RATE_LIMIT_WINDOW=1m` and
`AUTH_RATE_LIMIT_MAX=10` (per IP), `CORS_ALLOWED_ORIGINS` (allow-list, no wildcard),
`SHUTDOWN_TIMEOUT=10s`.

An invalid environment reports **every** problem at once and refuses to start.

### `REQUEST_BUDGET`

The total deadline of one `POST /api/v1/qr`: the decomposition **and** the call to stats-api,
its retry included, spend the same budget, so a slow factorization shortens what is left for the
dependency instead of adding to it. It is validated at boot against two hard bounds and the
service refuses to start outside them:

- `>= STATS_API_TIMEOUT x (1 + STATS_API_RETRIES)` (10 s with the defaults), or the last attempt
  could never finish;
- `<= 15s`, the server's write timeout (`config.WriteTimeout`), or the connection is closed
  before the budget expires and the caller never sees the `504`.

The outbound client also skips a retry when what is left of the budget no longer covers a whole
attempt: starting one that the deadline will cut short only delays an answer that is already an
error.

### Limits worth knowing

| | |
|---|---|
| `MAX_BODY_BYTES` (1 MiB) | Answered with `413 payload-too-large` by the middleware, with a problem document. |
| fasthttp backstop (2 x `MAX_BODY_BYTES`) | The framework buffers the body before any middleware sees it, so a *much* larger body is dropped at the transport level instead — no problem document, the connection is closed. |
| 16 KiB read buffer | The request line plus headers. Anything above it (a multi-KB `Authorization` header) is answered with `431 request-header-fields-too-large`. The 4 KiB fasthttp default would have rejected a legitimate long bearer token. |
| `errors[]` cap (200) | A 1 MiB body of `null` cells fails 200 000 rules; `detail` states the real total. |
