#!/usr/bin/env bash
# Setup two separate Kind clusters with a shared Docker network and standalone NATS.
# This simulates real network isolation between datacenters.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"
NETWORK_NAME="goakt-multi-dc-net"
NATS_CONTAINER="nats-shared"
DC1_CLUSTER="dc1"
DC2_CLUSTER="dc2"

echo "=========================================="
echo "Multi-DC Isolated Setup"
echo "=========================================="

# 1. Create shared Docker network
echo "Creating shared Docker network '${NETWORK_NAME}'..."
docker network create "${NETWORK_NAME}" 2>/dev/null || echo "  Network already exists"

# 2. Start standalone NATS container with JetStream
echo "Starting NATS container..."
if docker inspect "${NATS_CONTAINER}" &>/dev/null; then
  echo "  NATS container already exists, restarting..."
  docker rm -f "${NATS_CONTAINER}" >/dev/null
fi
docker run -d --name "${NATS_CONTAINER}" \
  --network "${NETWORK_NAME}" \
  nats:2.11-alpine \
  --jetstream --store_dir=/data --http_port=8222
echo "  NATS started"

# 3. Create Kind cluster for DC-1
echo "Creating Kind cluster '${DC1_CLUSTER}'..."
kind create cluster --name "${DC1_CLUSTER}" \
  --config "${PROJECT_DIR}/deploy/dc1/kind-config.yaml" \
  --wait 2m

# 4. Create Kind cluster for DC-2
echo "Creating Kind cluster '${DC2_CLUSTER}'..."
kind create cluster --name "${DC2_CLUSTER}" \
  --config "${PROJECT_DIR}/deploy/dc2/kind-config.yaml" \
  --wait 2m

# 5. Attach Kind nodes to the shared Docker network
echo "Connecting Kind nodes to shared network..."
docker network connect "${NETWORK_NAME}" "${DC1_CLUSTER}-control-plane" 2>/dev/null || echo "  DC-1 already connected"
docker network connect "${NETWORK_NAME}" "${DC2_CLUSTER}-control-plane" 2>/dev/null || echo "  DC-2 already connected"

# 6. Install metrics-server in both clusters
echo "Installing metrics-server..."
for ctx in "kind-${DC1_CLUSTER}" "kind-${DC2_CLUSTER}"; do
  kubectl apply -f https://github.com/kubernetes-sigs/metrics-server/releases/latest/download/components.yaml --context "${ctx}" >/dev/null 2>&1
  kubectl patch deployment metrics-server -n kube-system --context "${ctx}" --type='json' \
    -p='[{"op":"add","path":"/spec/template/spec/containers/0/args/-","value":"--kubelet-insecure-tls"}]' >/dev/null 2>&1
done

# 7. Discover IPs on the shared Docker network
NATS_IP=$(docker inspect "${NATS_CONTAINER}" -f "{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}" | head -1)
# Get the IP on the shared network specifically
NATS_IP=$(docker inspect -f "{{(index .NetworkSettings.Networks \"${NETWORK_NAME}\").IPAddress}}" "${NATS_CONTAINER}")
DC1_IP=$(docker inspect -f "{{(index .NetworkSettings.Networks \"${NETWORK_NAME}\").IPAddress}}" "${DC1_CLUSTER}-control-plane")
DC2_IP=$(docker inspect -f "{{(index .NetworkSettings.Networks \"${NETWORK_NAME}\").IPAddress}}" "${DC2_CLUSTER}-control-plane")

# Save IPs for deploy and test scripts
cat > "${PROJECT_DIR}/.env.ips" <<EOF
NATS_IP=${NATS_IP}
DC1_IP=${DC1_IP}
DC2_IP=${DC2_IP}
EOF

echo ""
echo "=========================================="
echo "Setup Complete"
echo "=========================================="
echo "  NATS IP:  ${NATS_IP}"
echo "  DC-1 IP:  ${DC1_IP}"
echo "  DC-2 IP:  ${DC2_IP}"
echo ""
echo "  DC-1 context: kind-${DC1_CLUSTER}"
echo "  DC-2 context: kind-${DC2_CLUSTER}"
echo ""
echo "Run 'make deploy' to build and deploy to both DCs."
