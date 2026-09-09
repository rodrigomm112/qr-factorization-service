# syntax=docker/dockerfile:1
# qr-api. Runtime is distroless/static: no shell, no package manager, no libc, non-root,
# so the binary implements its own `healthcheck` subcommand instead of shelling out to curl.
#   docker build -t proyectot/qr-api:local --build-arg VERSION=1.0.0 .

FROM golang:1.27.1-alpine AS build
WORKDIR /src

# Dependencies first: cached until go.mod/go.sum change.
COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG VERSION=dev
ARG TARGETOS=linux
ARG TARGETARCH=amd64

# CGO off: a static binary for distroless/static. -trimpath strips local paths, -s -w drop
# the symbol table and DWARF.
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -trimpath -ldflags="-s -w -X main.version=${VERSION}" \
      -o /out/qr-api ./cmd/qr-api

# Fail the build here rather than ship a binary that cannot run its own probe.
RUN /out/qr-api healthcheck 2>/dev/null; [ $? -eq 1 ]

FROM gcr.io/distroless/static-debian12:nonroot AS runtime

WORKDIR /app
COPY --from=build /out/qr-api /app/qr-api

ENV PORT=8080
EXPOSE 8080
USER nonroot:nonroot

HEALTHCHECK --interval=10s --timeout=3s --start-period=5s --retries=5 \
  CMD ["/app/qr-api", "healthcheck"]

ENTRYPOINT ["/app/qr-api"]
