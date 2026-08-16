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

# API integration test for the blockchain cluster.
# Submits transactions, mines a block, and verifies the chain.
# Ensure port-forward is running: make port-forward (in another terminal)

set -e

BASE_URL="${BASE_URL:-http://localhost:8080}"
MINE_TIMEOUT="${MINE_TIMEOUT:-30}"

echo "=== Blockchain API test against $BASE_URL ==="

echo "--- Baseline chain length"
baseline=$(curl -sf "$BASE_URL/status" | jq 'length')
echo "chain length: $baseline"

echo "--- Submitting two transactions"
curl -sf -X POST "$BASE_URL/transactions" -d '{"sender":"alice","recipient":"bob","value":40}' | jq -r '.message'
curl -sf -X POST "$BASE_URL/transactions" -d '{"sender":"bob","recipient":"carol","value":15}' | jq -r '.message'

pending=$(curl -sf "$BASE_URL/transactions" | jq 'length')
if [ "$pending" -lt 2 ]; then
  echo "FAIL: expected at least 2 pending transactions, got $pending"
  exit 1
fi
echo "pending transactions: $pending"

echo "--- Mining a block"
curl -sf "$BASE_URL/mine" | jq -r '.message'

expected=$((baseline + 1))
for i in $(seq 1 "$MINE_TIMEOUT"); do
  length=$(curl -sf "$BASE_URL/status" | jq 'length')
  if [ "$length" -ge "$expected" ]; then
    break
  fi
  sleep 1
done
if [ "$length" -lt "$expected" ]; then
  echo "FAIL: chain did not grow to $expected blocks within ${MINE_TIMEOUT}s"
  exit 1
fi
echo "chain length: $length"

echo "--- Verifying the mined block"
status=$(curl -sf "$BASE_URL/status")
head_transactions=$(echo "$status" | jq '.[0].transactions | length')
coinbase=$(echo "$status" | jq -r '.[0].transactions[] | select(.sender=="coinbase") | .recipient')
links=$(echo "$status" | jq '.[0].previousHash == .[1].hash')

if [ "$head_transactions" -lt 3 ]; then
  echo "FAIL: expected at least 3 transactions in the mined block, got $head_transactions"
  exit 1
fi
if [ "$coinbase" != "node0" ]; then
  echo "FAIL: expected a coinbase reward for node0, got '$coinbase'"
  exit 1
fi
if [ "$links" != "true" ]; then
  echo "FAIL: the mined block does not link to the previous block"
  exit 1
fi
echo "mined block has $head_transactions transaction(s) including the coinbase reward, and links to the previous block"

echo "--- Pending transactions are cleared"
pending=$(curl -sf "$BASE_URL/transactions" | jq 'length')
if [ "$pending" -ne 0 ]; then
  echo "FAIL: expected 0 pending transactions after mining, got $pending"
  exit 1
fi
echo "pending transactions: $pending"

echo "=== PASS ==="
