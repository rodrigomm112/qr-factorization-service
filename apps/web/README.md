# apps/web — Proyecto T client

React 19 + Vite 8 + TypeScript 6 SPA that drives **both** APIs from the browser:
`qr-api` (demo token, QR factorization) and `stats-api` (direct recomputation of
the statistics, health badge). No router, no state library, no UI/CSS framework.

One centred column, `max-width: 72rem`, in this order:

1. **Header** — title, subtitle, the two health dots (`operativo` / `sin
   respuesta`; the version is in the badge tooltip) and the session chip
   `Sesión de demo · JWT · 59:58`.
2. **Matriz** — the editor, always enabled: nothing to log into, so nothing to
   wait for. Steppers, presets, mode, the grid, and `Calcular QR` on the right of
   its footer.
3. **Resultado · Q, R y estadísticas** — errors first, then A / Q / R (each with a
   `Copiar` button that puts the raw JSON on the clipboard), the summary tiles,
   the per-matrix table, and `Recalcular en stats-api` at the foot.
4. **Detalles técnicos** — a closed `<details>` with the audit trail: both base
   URLs and versions, the token endpoint used, the token preview, `sub`, scope,
   expiry, the 100×100 server limit, the in-memory-token note, the reduced-mode
   explanation, the response metadata, and a copyable `curl` for the
   client-credentials grant.

```bash
npm ci && npm run dev            # http://localhost:5173
npm run typecheck && npm run lint && npm run test && npm run build
```

`npm test` mocks `fetch` with the golden fixtures of `contracts/examples`, so
the suite needs no API running. Against real APIs, `docker compose up qr-api
stats-api`: the session appears on its own, no secret to type.

`src/test/fixtures.ts` **reads** those example files at test time (`node:fs`, path
resolved from `import.meta.url`): they are not copied here, so a drift in the
shared contract fails a test instead of going unnoticed. Only the test files import
it — `vite build` walks the graph from `index.html` and never reaches it — which is
why the Docker context (`.dockerignore`, no `contracts/`) still builds: the image runs
`tsc --noEmit && vite build`, never `vitest`.

Browser journeys against the real containers live in [`../../e2e`](../../e2e)

## Runtime configuration

`index.html` loads `/config.js` **before** the module bundle; it sets
`window.__APP_CONFIG__ = { qrApiBaseUrl, statsApiBaseUrl }`. `src/config.ts` reads
it, then `VITE_QR_API_BASE_URL` / `VITE_STATS_API_BASE_URL`, then
`http://localhost:8080` / `http://localhost:3000`, stripping trailing slashes. In
the container, `docker-entrypoint.d/10-app-config.sh` regenerates `config.js` from
`QR_API_BASE_URL` / `STATS_API_BASE_URL` at startup, so **one image** serves
localhost and Cloud Run. The *browser* resolves these URLs: never `http://qr-api:8080`.

> **Precedencia (decidida: gana el runtime).** `VITE_QR_API_BASE_URL` /
> `VITE_STATS_API_BASE_URL` **solo se aplican si `public/config.js` no define esa
> clave**. El `config.js` del repositorio define las dos, así que en `npm run dev`
> las variables `VITE_*` no tienen efecto mientras ese archivo esté completo: para
> apuntar `vite dev` a otro backend, edita `public/config.js` (o borra la clave que
> quieras dejar en manos de `VITE_*`). Se eligió así porque es la única precedencia
> que sobrevive al build: la imagen se construye una vez y el entrypoint reescribe
> `config.js` en cada arranque, mientras que un `VITE_*` quedaría *horneado* en el
> bundle. `src/config.ts` implementa exactamente ese orden y `src/config.test.ts`
> lo fija.

```bash
docker build -t proyectot/web:local .
docker run --rm -p 8081:8080 -e QR_API_BASE_URL=http://localhost:8080 \
  -e STATS_API_BASE_URL=http://localhost:3000 proyectot/web:local
```

nginx (unprivileged, non-root, port 8080) serves the SPA with `try_files $uri
/index.html`, gzip, `no-store` on `config.js`, immutable caching for hashed assets
(but `no-store` on their 404s, so a stale hash is never negatively cached), HSTS,
and a strict CSP whose `connect-src` is `'self'` plus both API origins. The build
emits no inline script and no inline style, so no `'unsafe-inline'` is needed.

## Authentication

There is **no login form**. On load `useAuth` posts to
`{qrApiBaseUrl}/api/v1/auth/demo-token` — no body, no credentials — and gets the
same `TokenResponse` the client-credentials grant returns, with `sub = "demo"`.
The header chip counts the token down, a silent refresh fires ~60 s before it
expires, and a `401` from either API drops the session, issues one new demo token
and replays the failed request **once**; a second failure is rendered in the
results area. If the instance runs with `DEMO_TOKEN_ENABLED=false` the endpoint
answers `404` and the header says «Esta instancia no emite tokens de demo», with a
link to the technical details.

That is a deliberate trade: a public SPA cannot keep a client secret, so shipping
a form for it would only teach the wrong habit. The **confidential** grant
(`POST /api/v1/auth/token` with `clientId` / `clientSecret`) is still implemented,
still documented in `openapi.yaml`, still exercised by the API suites — and the
technical details panel shows a ready-to-copy `curl` for it:

```bash
curl -sS -X POST http://localhost:8080/api/v1/auth/token \
  -H 'Content-Type: application/json' \
  -d '{"clientId":"<clientId>","clientSecret":"<clientSecret>"}'
```

(the real values come from `scripts/gen-secrets.sh` / `.env.demo-credentials`).

## Security

The access token lives **only** in React state — no `localStorage`, no
`sessionStorage`, no cookie — is dropped when `expiresIn` elapses, and the header
shows the countdown. Errors are RFC 9457 documents rendered with `title`, `detail`,
`errors[]` (pointer + code) and `requestId`.

## Visual identity

The palette follows Interseguro's public site: blue `#0855C4` for headings, links and every
button (white text; the site's fuchsia is not used), text `#454A6C` /
`#74799A`, section background `#F5F6FC`, borders `#C8D1F1`, and product cards with two opposite
rounded corners (`25px 0 25px 0`). Only colours and shapes are borrowed; no logo or name.
`public/fonts/sofiapro-light.otf` is the single weight available (Fontspring desktop EULA:
check its web-embedding terms before any public distribution); headings use the system
semibold and Arial is the fallback. Health, warning and error colours stay semantic.
