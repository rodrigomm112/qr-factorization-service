# syntax=docker/dockerfile:1

# ---- Build: type-check, then bundle with Vite ----
FROM node:26.8-alpine AS build

WORKDIR /app
ENV CI=true

# Dependencies first: the layer is reused while only sources change.
COPY package.json package-lock.json ./
RUN npm ci --no-audit --no-fund

COPY tsconfig.json vite.config.ts index.html ./
COPY public ./public
COPY src ./src

# `tsc --noEmit && vite build`: no image out of code that does not type-check.
RUN npm run build

# ---- Runtime: static files behind nginx, unprivileged and non-root ----
FROM nginxinc/nginx-unprivileged:1.29-alpine AS runtime

USER root

COPY templates/default.conf.template /etc/nginx/templates/default.conf.template
COPY docker-entrypoint.d/10-app-config.sh /docker-entrypoint.d/10-app-config.sh
COPY --from=build /app/dist /usr/share/nginx/html

RUN chmod 0755 /docker-entrypoint.d/10-app-config.sh \
    && chown -R 101:101 /usr/share/nginx/html

# Browser-facing defaults; docker-compose and Cloud Run override them.
ENV QR_API_BASE_URL=http://localhost:8080 \
    STATS_API_BASE_URL=http://localhost:3000

USER 101
EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget -qO- http://127.0.0.1:8080/ >/dev/null || exit 1
