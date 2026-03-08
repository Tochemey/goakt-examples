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

# API integration tests for goakt-2pc money transfer service
# Creates accounts, performs transfers, and verifies balances
# Run with: ./scripts/test-api.sh
# Ensure port-forward is running: make port-forward (in another terminal)

set -e

BASE_URL="${BASE_URL:-http://localhost:8080}"

get_balance() {
  local json="$1"
  [ -z "$json" ] && return 0
  if command -v jq &>/dev/null; then
    echo "$json" | jq -r '.account.account_balance // empty' 2>/dev/null || true
  else
    echo "$json" | grep -o '"account_balance":[0-9.]*' | cut -d':' -f2
  fi
}

echo "=========================================="
echo "goakt-2pc Money Transfer - Integration Test"
echo "=========================================="
echo "API: $BASE_URL"
echo ""

# Pre-flight
echo "Checking API connectivity..."
if ! curl -sf --connect-timeout 5 -o /dev/null "$BASE_URL/openapi.yaml"; then
  echo "ERROR: Cannot connect to API at $BASE_URL"
  echo "       Is 'make port-forward' running in another terminal?"
  exit 1
fi
echo "API reachable."
echo ""

# Create two accounts
echo "Creating account alice with balance 100..."
alice_resp=$(curl -s -X POST "$BASE_URL/accounts" \
  -H "Content-Type: application/json" \
  -d '{"create_account":{"account_id":"alice","account_balance":100}}')
alice_bal=$(get_balance "$alice_resp")
if [ -z "$alice_bal" ]; then
  echo "FAIL: Could not create alice"
  exit 1
fi
echo "  alice balance: $alice_bal"

echo "Creating account bob with balance 50..."
bob_resp=$(curl -s -X POST "$BASE_URL/accounts" \
  -H "Content-Type: application/json" \
  -d '{"create_account":{"account_id":"bob","account_balance":50}}')
bob_bal=$(get_balance "$bob_resp")
if [ -z "$bob_bal" ]; then
  echo "FAIL: Could not create bob"
  exit 1
fi
echo "  bob balance: $bob_bal"
echo ""

# Transfer 30 from alice to bob
echo "Transferring 30 from alice to bob..."
transfer_resp=$(curl -s -X POST "$BASE_URL/transfers" \
  -H "Content-Type: application/json" \
  -d '{"transfer":{"from_account_id":"alice","to_account_id":"bob","amount":30}}')
status=$(echo "$transfer_resp" | jq -r '.transfer.status // empty' 2>/dev/null || echo "$transfer_resp" | grep -o '"status":"[^"]*"' | cut -d'"' -f4)
if [ "$status" != "completed" ]; then
  echo "FAIL: Transfer failed. Response: $transfer_resp"
  exit 1
fi
transfer_id=$(echo "$transfer_resp" | jq -r '.transfer.transfer_id // empty' 2>/dev/null)
echo "  Transfer completed. ID: $transfer_id"
echo ""

# Verify balances
echo "Verifying balances..."
alice_resp=$(curl -s "$BASE_URL/accounts/alice")
bob_resp=$(curl -s "$BASE_URL/accounts/bob")
alice_bal=$(get_balance "$alice_resp")
bob_bal=$(get_balance "$bob_resp")
echo "  alice balance: $alice_bal (expected 70)"
echo "  bob balance: $bob_bal (expected 80)"

alice_expected=70
bob_expected=80
alice_int=$(echo "$alice_bal" | cut -d. -f1)
bob_int=$(echo "$bob_bal" | cut -d. -f1)

if [ "$alice_int" != "$alice_expected" ]; then
  echo "FAIL: alice expected $alice_expected, got $alice_bal"
  exit 1
fi
if [ "$bob_int" != "$bob_expected" ]; then
  echo "FAIL: bob expected $bob_expected, got $bob_bal"
  exit 1
fi
echo ""

# Test insufficient funds
echo "Testing insufficient funds (transfer 200 from bob to alice)..."
fail_resp=$(curl -s -X POST "$BASE_URL/transfers" \
  -H "Content-Type: application/json" \
  -d '{"transfer":{"from_account_id":"bob","to_account_id":"alice","amount":200}}')
fail_status=$(echo "$fail_resp" | jq -r '.transfer.status // empty' 2>/dev/null || echo "$fail_resp" | grep -o '"status":"[^"]*"' | cut -d'"' -f4)
if [ "$fail_status" != "failed" ]; then
  echo "FAIL: Expected transfer to fail with insufficient funds. Response: $fail_resp"
  exit 1
fi
echo "  Correctly rejected (insufficient funds)"
echo ""

# Verify balances unchanged after failed transfer
alice_resp=$(curl -s "$BASE_URL/accounts/alice")
bob_resp=$(curl -s "$BASE_URL/accounts/bob")
alice_bal=$(get_balance "$alice_resp")
bob_bal=$(get_balance "$bob_resp")
if [ "$(echo "$alice_bal" | cut -d. -f1)" != "70" ] || [ "$(echo "$bob_bal" | cut -d. -f1)" != "80" ]; then
  echo "FAIL: Balances changed after failed transfer (2pc compensation should have rolled back)"
  exit 1
fi
echo "  Balances unchanged after failed transfer (2pc compensation verified)"
echo ""

echo "=========================================="
echo "All tests passed!"
echo "=========================================="
