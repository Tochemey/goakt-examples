# syntax=docker/dockerfile:1.6
#
# Three-stage build:
#   1. web-builder  — Node compiles web/main.ts → web/main.js
#   2. go-builder   — Go compiles the binary, embedding the freshly-built JS
#   3. runtime      — minimal alpine that just runs the binary
#
# The web/main.js artifact is intentionally NOT committed (see .gitignore);
# it's produced fresh inside this image so the docker build is hermetic.
#
# Build from the *parent* directory (the goakt-examples module root) so
# that go.mod / go.sum / vendor are visible to the COPY commands below.
# docker-compose.yaml in this folder already sets `context: ..`.

# ─── Stage 1: TypeScript ────────────────────────────────────────────────
FROM node:22-alpine AS web-builder

WORKDIR /src
COPY goakt-tetris/tsconfig.json ./tsconfig.json
COPY goakt-tetris/web ./web

# `tsc` reads tsconfig.json (rootDir=web, outDir=web) and writes
# /src/web/main.js next to the source.
RUN npx --package=typescript@5.6 -y -- tsc -p .

# ─── Stage 2: Go ────────────────────────────────────────────────────────
FROM golang:1.26-alpine AS go-builder

RUN apk --no-cache add git ca-certificates

WORKDIR /src

# Module + vendor first so changes to source code alone don't bust the
# dependency cache.
COPY go.mod go.sum ./
COPY vendor ./vendor

# Source for just this example.
COPY goakt-tetris ./goakt-tetris

# Drop in the freshly-built JS. Overwrites any stale file that might
# have been carried in by the COPY above (there shouldn't be one — it's
# gitignored — but being explicit keeps the layer hermetic).
COPY --from=web-builder /src/web/main.js ./goakt-tetris/web/main.js

ENV CGO_ENABLED=0
RUN go build -mod=vendor -trimpath -ldflags="-s -w" -o /out/goakt-tetris ./goakt-tetris

# ─── Stage 3: Runtime ───────────────────────────────────────────────────
FROM alpine:3.20

RUN apk --no-cache add ca-certificates

COPY --from=go-builder /out/goakt-tetris /usr/local/bin/goakt-tetris

# 8080 = HTTP/WS gateway, 9000 = remote actor gRPC, 9001 = cluster gossip,
# 9002 = peer state sync. docker-compose.yaml maps 8080 to a host port and
# leaves the rest internal to the docker network.
EXPOSE 8080 9000 9001 9002

ENTRYPOINT ["/usr/local/bin/goakt-tetris"]
