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

# Multi-DC API integration tests
# Tests cross-datacenter actor placement and messaging
# Run with: ./scripts/test-api.sh
# Ensure port-forward is running: make port-forward (in another terminal)

set -e

BASE_URL="${BASE_URL:-http://localhost:8080}"
DC1_URL="${BASE_URL}/dc1"
DC2_URL="${BASE_URL}/dc2"
NUM_ACCOUNTS="${NUM_ACCOUNTS:-50}"
INITIAL_BALANCE="${INITIAL_BALANCE:-100}"
CREDIT_AMOUNT="${CREDIT_AMOUNT:-50}"
RUN_ID="${RUN_ID:-$(date +%s)}"

# Extract balance from JSON response
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
echo "Multi-DC Account Service - Integration Test"
echo "=========================================="
echo "DC-1 API: $DC1_URL"
echo "DC-2 API: $DC2_URL"
echo "Run ID: $RUN_ID"
echo ""

# Pre-flight: verify both DCs are reachable
echo "Phase 0: Checking API connectivity..."
if ! curl -sf --connect-timeout 5 -o /dev/null "$DC1_URL/openapi.yaml"; then
  echo "ERROR: Cannot connect to DC-1 at $DC1_URL"
  echo "       Is 'make port-forward' running in another terminal?"
  exit 1
fi
echo "  DC-1 reachable"

if ! curl -sf --connect-timeout 5 -o /dev/null "$DC2_URL/openapi.yaml"; then
  echo "ERROR: Cannot connect to DC-2 at $DC2_URL"
  exit 1
fi
echo "  DC-2 reachable"
echo ""

# Check DC status and wait for both DCs to be ready
echo "Phase 0.5: Waiting for datacenter readiness..."
for dc_label in DC-1 DC-2; do
  if [ "$dc_label" = "DC-1" ]; then dc_url="$DC1_URL"; else dc_url="$DC2_URL"; fi
  elapsed=0
  while true; do
    ready=$(curl -s "$dc_url/dc/status" | grep -o '"ready":true' || true)
    if [ -n "$ready" ]; then
      echo "  $dc_label is ready"
      break
    fi
    if [ $elapsed -ge 60 ]; then
      echo "  WARN: $dc_label not ready after 60s, proceeding anyway"
      break
    fi
    sleep 3
    elapsed=$((elapsed + 3))
  done
done
dc1_status=$(curl -s "$DC1_URL/dc/status")
dc2_status=$(curl -s "$DC2_URL/dc/status")
echo "  DC-1 status: $dc1_status"
echo "  DC-2 status: $dc2_status"
echo ""

START_TIME=$(date +%s)

# Phase 1: Create accounts in DC-1
echo "Phase 1: Creating $NUM_ACCOUNTS accounts in DC-1 (initial balance: $INITIAL_BALANCE)..."
CREATE_FAIL=0
for i in $(seq 1 "$NUM_ACCOUNTS"); do
  acc_id=$(printf "%s-acc-%04d" "$RUN_ID" "$i")
  resp=$(curl -s -w "\n%{http_code}" --connect-timeout 5 -m 10 -X POST "$DC1_URL/accounts" \
    -H "Content-Type: application/json" \
    -d "{\"create_account\":{\"account_id\":\"$acc_id\",\"account_balance\":$INITIAL_BALANCE}}")
  http_code=$(echo "$resp" | tail -n1)
  body=$(echo "$resp" | sed '$d')
  balance=$(get_balance "$body")

  if [ "$http_code" != "200" ] || [ -z "$balance" ]; then
    echo "  FAIL: $acc_id (HTTP $http_code)"
    ((CREATE_FAIL++)) || true
  fi

  if [ $((i % 10)) -eq 0 ]; then
    echo "  Progress: $i/$NUM_ACCOUNTS"
  fi
done

if [ "$CREATE_FAIL" -gt 0 ]; then
  echo "  Create phase: $CREATE_FAIL failures"
  exit 1
fi
echo "  Done: $NUM_ACCOUNTS accounts created in DC-1"
echo ""

# Phase 2: Query accounts from DC-2 (cross-DC lookup)
echo "Phase 2: Querying $NUM_ACCOUNTS accounts from DC-2 (cross-DC lookup)..."
QUERY_FAIL=0
QUERY_PASS=0
for i in $(seq 1 "$NUM_ACCOUNTS"); do
  acc_id=$(printf "%s-acc-%04d" "$RUN_ID" "$i")
  resp=$(curl -s -w "\n%{http_code}" --connect-timeout 5 -m 10 "$DC2_URL/accounts/$acc_id")
  http_code=$(echo "$resp" | tail -n1)
  body=$(echo "$resp" | sed '$d')
  balance=$(get_balance "$body")

  if [ "$http_code" != "200" ]; then
    echo "  FAIL: $acc_id - HTTP $http_code"
    ((QUERY_FAIL++)) || true
  elif [ -z "$balance" ]; then
    echo "  FAIL: $acc_id - no balance in response"
    ((QUERY_FAIL++)) || true
  else
    balance_int=$(echo "$balance" | cut -d. -f1)
    if [ "$balance_int" != "$INITIAL_BALANCE" ]; then
      echo "  FAIL: $acc_id - expected $INITIAL_BALANCE, got $balance"
      ((QUERY_FAIL++)) || true
    else
      ((QUERY_PASS++)) || true
    fi
  fi

  if [ $((i % 10)) -eq 0 ]; then
    echo "  Progress: $i/$NUM_ACCOUNTS"
  fi
done
echo "  Cross-DC query: $QUERY_PASS passed, $QUERY_FAIL failed"
echo ""

# Phase 3: Credit accounts via DC-2
echo "Phase 3: Crediting $NUM_ACCOUNTS accounts via DC-2 (+$CREDIT_AMOUNT each)..."
CREDIT_FAIL=0
for i in $(seq 1 "$NUM_ACCOUNTS"); do
  acc_id=$(printf "%s-acc-%04d" "$RUN_ID" "$i")
  resp=$(curl -s -w "\n%{http_code}" --connect-timeout 5 -m 10 -X POST "$DC2_URL/accounts/$acc_id/credit" \
    -H "Content-Type: application/json" \
    -d "{\"balance\":$CREDIT_AMOUNT}")
  http_code=$(echo "$resp" | tail -n1)

  if [ "$http_code" != "200" ]; then
    echo "  FAIL: $acc_id (HTTP $http_code)"
    ((CREDIT_FAIL++)) || true
  fi

  if [ $((i % 10)) -eq 0 ]; then
    echo "  Progress: $i/$NUM_ACCOUNTS"
  fi
done

if [ "$CREDIT_FAIL" -gt 0 ]; then
  echo "  Credit phase: $CREDIT_FAIL failures"
  exit 1
fi
echo "  Done: $NUM_ACCOUNTS accounts credited via DC-2"
echo ""

# Phase 4: Verify final balances from DC-1
EXPECTED_BALANCE=$((INITIAL_BALANCE + CREDIT_AMOUNT))
echo "Phase 4: Verifying final balances from DC-1 (expected: $EXPECTED_BALANCE)..."
VERIFY_FAIL=0
VERIFY_PASS=0
for i in $(seq 1 "$NUM_ACCOUNTS"); do
  acc_id=$(printf "%s-acc-%04d" "$RUN_ID" "$i")
  resp=$(curl -s -w "\n%{http_code}" --connect-timeout 5 -m 10 "$DC1_URL/accounts/$acc_id")
  http_code=$(echo "$resp" | tail -n1)
  body=$(echo "$resp" | sed '$d')
  balance=$(get_balance "$body")

  if [ "$http_code" != "200" ]; then
    echo "  FAIL: $acc_id - HTTP $http_code"
    ((VERIFY_FAIL++)) || true
  elif [ -z "$balance" ]; then
    echo "  FAIL: $acc_id - no balance in response"
    ((VERIFY_FAIL++)) || true
  else
    balance_int=$(echo "$balance" | cut -d. -f1)
    if [ "$balance_int" != "$EXPECTED_BALANCE" ]; then
      echo "  FAIL: $acc_id - expected $EXPECTED_BALANCE, got $balance"
      ((VERIFY_FAIL++)) || true
    else
      ((VERIFY_PASS++)) || true
    fi
  fi

  if [ $((i % 10)) -eq 0 ]; then
    echo "  Progress: $i/$NUM_ACCOUNTS"
  fi
done
echo ""

# Phase 5: Test remote spawn (spawn actor in DC-2 from DC-1)
echo "Phase 5: Testing remote spawn (DC-1 -> DC-2)..."
REMOTE_ACC_ID="${RUN_ID}-remote-001"
remote_resp=$(curl -s -w "\n%{http_code}" --connect-timeout 5 -m 10 -X POST "$DC1_URL/accounts/spawn-remote" \
  -H "Content-Type: application/json" \
  -d "{\"account_id\":\"$REMOTE_ACC_ID\",\"account_balance\":500.00,\"target_dc\":\"dc-2\"}")
remote_http=$(echo "$remote_resp" | tail -n1)
remote_body=$(echo "$remote_resp" | sed '$d')

if [ "$remote_http" = "200" ]; then
  echo "  Remote spawn OK: $remote_body"
  # Verify from DC-2
  verify_resp=$(curl -s "$DC2_URL/accounts/$REMOTE_ACC_ID")
  verify_balance=$(get_balance "$verify_resp")
  if [ -n "$verify_balance" ]; then
    echo "  Remote account verified from DC-2: balance=$verify_balance"
  else
    echo "  WARN: Could not verify remote account from DC-2"
  fi
else
  echo "  WARN: Remote spawn returned HTTP $remote_http (may not be ready yet)"
  echo "  Response: $remote_body"
fi
echo ""

END_TIME=$(date +%s)
DURATION=$((END_TIME - START_TIME))

echo "=========================================="
echo "Results"
echo "=========================================="
echo "Created (DC-1):     $NUM_ACCOUNTS accounts"
echo "Cross-DC query:     $QUERY_PASS passed, $QUERY_FAIL failed"
echo "Credited (DC-2):    $NUM_ACCOUNTS accounts"
echo "Final verify (DC-1): $VERIFY_PASS passed, $VERIFY_FAIL failed"
echo "Remote spawn:       $([ "$remote_http" = "200" ] && echo "OK" || echo "WARN")"
echo "Duration:           ${DURATION}s"
echo ""

TOTAL_FAIL=$((QUERY_FAIL + VERIFY_FAIL))
if [ "$TOTAL_FAIL" -gt 0 ]; then
  echo "FAIL: $TOTAL_FAIL verification(s) failed"
  exit 1
fi

echo "All tests passed!"
