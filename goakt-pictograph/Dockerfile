# syntax=docker/dockerfile:1.6
#
# Three-stage build (mirrors goakt-game):
#   1. web-builder  — Node compiles web/main.ts → web/main.js
#   2. go-builder   — Go compiles the binary, embedding the freshly-built JS
#   3. runtime      — minimal alpine that just runs the binary
#
# Build context must be the parent directory (the goakt-examples module
# root) so go.mod / go.sum / vendor are visible. docker-compose.yaml in
# this folder already sets `context: ..`.

# ─── Stage 1: TypeScript ────────────────────────────────────────────────
FROM node:22-alpine AS web-builder

WORKDIR /src
COPY goakt-pictograph/tsconfig.json ./tsconfig.json
COPY goakt-pictograph/web ./web

RUN npx --package=typescript@5.6 -y -- tsc -p .

# ─── Stage 2: Go ────────────────────────────────────────────────────────
FROM golang:1.26-alpine AS go-builder

RUN apk --no-cache add git ca-certificates

WORKDIR /src

COPY go.mod go.sum ./
COPY vendor ./vendor

COPY goakt-pictograph ./goakt-pictograph

COPY --from=web-builder /src/web/main.js ./goakt-pictograph/web/main.js

ENV CGO_ENABLED=0
RUN go build -mod=vendor -trimpath -ldflags="-s -w" -o /out/goakt-pictograph ./goakt-pictograph

# ─── Stage 3: Runtime ───────────────────────────────────────────────────
FROM alpine:3.20

RUN apk --no-cache add ca-certificates

COPY --from=go-builder /out/goakt-pictograph /usr/local/bin/goakt-pictograph

# 8080 = HTTP/WS gateway, 9000 = remote actor gRPC, 9001 = cluster gossip,
# 9002 = peer state sync. docker-compose.yaml maps 8080 to a host port and
# leaves the rest internal to the docker network.
EXPOSE 8080 9000 9001 9002

ENTRYPOINT ["/usr/local/bin/goakt-pictograph"]
