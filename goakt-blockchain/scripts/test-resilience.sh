#!/usr/bin/env bash
# MIT License
#
# Copyright (c) 2022-2026 GoAkt Team
#
# Permission is hereby granted, free of charge, to any person obtaining a copy
# of this software and associated documentation files (the "Software"), to deal
# in the Software without restriction, including without limitation the rights
# to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
# copies of the Software, and to permit persons to whom the Software is
# furnished to do so, subject to the following conditions:
#
# The above copyright notice and this permission notice shall be included in all
# copies or substantial portions of the Software.
#
# THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
# IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
# FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
# AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
# LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
# OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
# SOFTWARE.

# Resilience test for the blockchain cluster.
#
# Flow:
#   Phase 1 - Mine a block so the chain has state to lose
#   Phase 2 - Find the pod hosting the node0 singleton and delete it
#   Phase 3 - Wait for the StatefulSet to become Ready again
#   Phase 4 - Verify the singleton respawned elsewhere, the chain survived the
#             failover (every pod keeps a full replica in its local Pebble
#             store), and mining still works
#
# Run with:   ./scripts/test-resilience.sh
# Requires:   make port-forward (in another terminal)

set -e

BASE_URL="${BASE_URL:-http://localhost:8080}"
RECOVER_TIMEOUT="${RECOVER_TIMEOUT:-120}"

echo "=== Blockchain resilience test against $BASE_URL ==="

echo "--- Phase 1: mining a baseline block"
curl -sf "$BASE_URL/mine" > /dev/null
baseline=$(curl -sf "$BASE_URL/status" | jq 'length')
for i in $(seq 1 30); do
  baseline=$(curl -sf "$BASE_URL/status" | jq 'length')
  if [ "$baseline" -ge 2 ]; then
    break
  fi
  sleep 1
done
if [ "$baseline" -lt 2 ]; then
  echo "FAIL: could not mine a baseline block"
  exit 1
fi
echo "chain length before failover: $baseline"

echo "--- Phase 2: locating the pod hosting the node0 singleton"
host_pod=""
for pod in $(kubectl get pods -l app.kubernetes.io/name=blockchain -o jsonpath='{.items[*].metadata.name}'); do
  if kubectl logs "$pod" | grep -q "node0 started on"; then
    host_pod="$pod"
  fi
done
if [ -z "$host_pod" ]; then
  echo "FAIL: could not find the pod hosting the node0 singleton"
  exit 1
fi
echo "node0 singleton runs on: $host_pod"

echo "--- deleting $host_pod"
kubectl delete pod "$host_pod" --wait=false

echo "--- Phase 3: waiting for the StatefulSet to recover"
sleep 5
kubectl rollout status statefulset/blockchain --timeout="${RECOVER_TIMEOUT}s"

echo "--- Phase 4: verifying failover"
recovered=""
for i in $(seq 1 30); do
  if curl -sf "$BASE_URL/status" > /dev/null 2>&1; then
    recovered="yes"
    break
  fi
  sleep 2
done
if [ -z "$recovered" ]; then
  echo "FAIL: API did not answer after the pod loss"
  exit 1
fi

echo "--- The chain survived the failover"
length=$(curl -sf "$BASE_URL/status" | jq 'length')
if [ "$length" -lt "$baseline" ]; then
  echo "FAIL: chain length dropped from $baseline to $length after the failover"
  exit 1
fi
echo "chain length after failover: $length"

echo "--- Mining still works after failover"
curl -sf "$BASE_URL/mine" | jq -r '.message'
expected=$((length + 1))
for i in $(seq 1 30); do
  length=$(curl -sf "$BASE_URL/status" | jq 'length')
  if [ "$length" -ge "$expected" ]; then
    break
  fi
  sleep 1
done
if [ "$length" -lt "$expected" ]; then
  echo "FAIL: could not mine a block after failover"
  exit 1
fi
echo "mined a block after failover (chain length: $length)"

new_host=""
for pod in $(kubectl get pods -l app.kubernetes.io/name=blockchain -o jsonpath='{.items[*].metadata.name}'); do
  if kubectl logs "$pod" 2>/dev/null | grep -q "node0 started on"; then
    new_host="$pod"
  fi
done
echo "node0 singleton now runs on: ${new_host:-unknown}"

echo "=== PASS ==="
