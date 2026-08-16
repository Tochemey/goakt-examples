# Shared multi-stage build for every example image.
#
# EXAMPLE is the path of the example to compile (e.g. goakt-cluster/k8s).
# BINARY is the name of the binary inside the runtime image. Deployment
# manifests invoke the binary by name (e.g. `command: ["./accounts", "run"]`),
# so BINARY must match what the manifests of the example expect.
FROM golang:1.26.0-alpine AS build

ARG EXAMPLE
WORKDIR /app
COPY . .
RUN CGO_ENABLED=0 go build -mod=vendor -o /out/example ./$EXAMPLE

FROM alpine:3.21

ARG BINARY=app
WORKDIR /app
COPY --from=build /out/example ./$BINARY

# a fixed-name symlink so the ENTRYPOINT works for any BINARY value while
# compose files append arguments (e.g. `command: ["run"]`)
RUN ln -s /app/$BINARY /app/entrypoint

EXPOSE 50051 50052 3320 3322 9092

ENTRYPOINT ["./entrypoint"]
