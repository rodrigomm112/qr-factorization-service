# syntax=docker/dockerfile:1

ARG NODE_IMAGE=node:24.20.0-alpine

# Full dependency tree, needed to compile.
FROM ${NODE_IMAGE} AS deps
WORKDIR /app
COPY package.json package-lock.json ./
RUN npm ci --no-audit --no-fund

# tsc -> dist/
FROM ${NODE_IMAGE} AS build
WORKDIR /app
COPY --from=deps /app/node_modules ./node_modules
COPY package.json package-lock.json tsconfig.json tsconfig.build.json ./
COPY src ./src
RUN npm run build

# Runtime dependency tree only.
FROM ${NODE_IMAGE} AS prod-deps
WORKDIR /app
COPY package.json package-lock.json ./
RUN npm ci --omit=dev --no-audit --no-fund && npm cache clean --force

# Distributable image: non-root, no toolchain, no sources.
FROM ${NODE_IMAGE} AS runtime
# Stamped by the deploy script with the short git SHA; surfaced at /health/live.
ARG VERSION=dev
LABEL org.opencontainers.image.title="proyectot/stats-api" \
      org.opencontainers.image.description="Descriptive statistics over one or more rectangular matrices" \
      org.opencontainers.image.licenses="MIT"

ENV NODE_ENV=production \
    APP_ENV=production \
    APP_VERSION=${VERSION} \
    PORT=3000

WORKDIR /app
COPY --from=prod-deps /app/node_modules ./node_modules
COPY --from=build /app/dist ./dist
# `openapi.yaml` is served by /openapi.yaml and /docs; `package.json` carries the version.
COPY package.json openapi.yaml ./

USER node
EXPOSE 3000

# `fetch` is built into Node 24, so the probe needs no shell and no extra package.
HEALTHCHECK --interval=30s --timeout=3s --start-period=10s --retries=3 \
  CMD node -e "fetch('http://127.0.0.1:'+(process.env.PORT||3000)+'/health/ready').then(r=>process.exit(r.ok?0:1)).catch(()=>process.exit(1))"

ENTRYPOINT ["node", "dist/main.js"]
