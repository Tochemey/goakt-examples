#!/usr/bin/env bash
# Deploy to a specific DC's Kind cluster.
# Usage: ./scripts/deploy-dc.sh dc1|dc2
set -euo pipefail

DC="${1:?Usage: deploy-dc.sh dc1|dc2}"
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"
DEPLOY_DIR="${PROJECT_DIR}/deploy/${DC}"
CONTEXT="kind-${DC}"
IMAGE_NAME="accounts:dev-multi-dc-isolated"

if [ ! -f "${PROJECT_DIR}/.env.ips" ]; then
  echo "ERROR: .env.ips not found. Run 'make setup' first."
  exit 1
fi

source "${PROJECT_DIR}/.env.ips"

echo "Deploying to ${DC} (context: ${CONTEXT})..."
echo "  NATS IP: ${NATS_IP}"

# Load image into this DC's Kind cluster
echo "Loading image into Kind cluster '${DC}'..."
kind load docker-image "${IMAGE_NAME}" --name "${DC}"

# Deploy shared resources
echo "Deploying PostgreSQL..."
kubectl apply -f "${PROJECT_DIR}/deploy/postgres-secret.yaml" --context "${CONTEXT}"
kubectl apply -f "${PROJECT_DIR}/deploy/postgres-configmap.yaml" --context "${CONTEXT}"
kubectl apply -f "${DEPLOY_DIR}/postgres.yaml" --context "${CONTEXT}"

PG_NAME="postgres-${DC}"
echo "Waiting for PostgreSQL (${PG_NAME})..."
kubectl wait --for=condition=available --timeout=120s "deployment/${PG_NAME}" --context "${CONTEXT}"

# Determine this DC's IP on the shared Docker network
if [ "${DC}" = "dc1" ]; then
  DC_IP="${DC1_IP}"
else
  DC_IP="${DC2_IP}"
fi

# Deploy accounts StatefulSet with NATS IP and DC IP injected
echo "Deploying accounts StatefulSet (DC IP: ${DC_IP})..."
sed -e "s/NATS_IP_PLACEHOLDER/${NATS_IP}/g" \
    -e "s/DC_IP_PLACEHOLDER/${DC_IP}/g" \
    "${DEPLOY_DIR}/statefulset.yaml" | \
  kubectl apply -f - --context "${CONTEXT}"

# Wait for all 3 pods
echo "Waiting for account pods..."
for i in 0 1 2; do
  POD="accounts-${DC}-${i}"
  echo "  Waiting for ${POD}..."
  elapsed=0
  until [ "$(kubectl get pod "${POD}" -o jsonpath='{.status.phase}' --context "${CONTEXT}" 2>/dev/null)" = "Running" ]; do
    sleep 5
    elapsed=$((elapsed + 5))
    if [ $elapsed -ge 180 ]; then
      echo "  Timeout waiting for ${POD}"
      kubectl describe pod "${POD}" --context "${CONTEXT}" 2>/dev/null | tail -15
      exit 1
    fi
  done
  echo "  ${POD} is running"
done

# Deploy nginx
echo "Deploying nginx..."
kubectl apply -f "${DEPLOY_DIR}/nginx.yaml" --context "${CONTEXT}"

echo "${DC} deployment complete."
