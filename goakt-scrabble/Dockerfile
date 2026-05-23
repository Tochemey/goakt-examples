# syntax=docker/dockerfile:1.6
#
# Three-stage build:
#   1. web-builder  — Node compiles web/main.ts → web/main.js
#   2. go-builder   — Go statically compiles the binary, embedding the JS + ENABLE dict
#   3. runtime      — distroless/static (no shell, no package manager, ~3 MB base)
#
# Build context must be the parent directory (the goakt-examples module
# root) so go.mod / go.sum / vendor are visible. The `make image-build`
# target sets the context for you.

# ─── Stage 1: TypeScript ────────────────────────────────────────────────
FROM node:22-alpine AS web-builder

WORKDIR /src
COPY goakt-scrabble/tsconfig.json ./tsconfig.json
COPY goakt-scrabble/web ./web

RUN npx --package=typescript@5.6 -y -- tsc -p .

# ─── Stage 2: Go ────────────────────────────────────────────────────────
FROM golang:1.26-alpine AS go-builder

RUN apk --no-cache add ca-certificates

WORKDIR /src

COPY go.mod go.sum ./
COPY vendor ./vendor

COPY goakt-scrabble ./goakt-scrabble

COPY --from=web-builder /src/web/main.js ./goakt-scrabble/web/main.js

ENV CGO_ENABLED=0
RUN go build -mod=vendor -trimpath -ldflags="-s -w" -o /out/goakt-scrabble ./goakt-scrabble

# ─── Stage 3: Runtime ───────────────────────────────────────────────────
FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=go-builder /out/goakt-scrabble /usr/local/bin/goakt-scrabble

# 8080 = HTTP/WS gateway, 9000 = remote actor gRPC,
# 9001 = cluster gossip, 9002 = peer state sync.
EXPOSE 8080 9000 9001 9002

ENTRYPOINT ["/usr/local/bin/goakt-scrabble"]
