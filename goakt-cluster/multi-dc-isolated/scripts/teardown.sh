#!/usr/bin/env bash
# Tear down the two Kind clusters, NATS container, and shared Docker network.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"

echo "Tearing down multi-dc-isolated environment..."

echo "Deleting Kind clusters..."
kind delete cluster --name dc1 2>/dev/null || true
kind delete cluster --name dc2 2>/dev/null || true

echo "Removing NATS container..."
docker rm -f nats-shared 2>/dev/null || true

echo "Removing shared Docker network..."
docker network rm goakt-multi-dc-net 2>/dev/null || true

rm -f "${PROJECT_DIR}/.env.ips"

echo "Teardown complete."
