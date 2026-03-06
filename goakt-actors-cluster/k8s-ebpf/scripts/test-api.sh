#!/usr/bin/env bash
# API integration tests for k8s-v2 account service
# Creates 1000 accounts, credits each, and verifies balances across the cluster
# Run with: ./scripts/test-api.sh
# Ensure port-forward is running: make port-forward (in another terminal)

set -e

BASE_URL="${BASE_URL:-http://localhost:8080}"
NUM_ACCOUNTS="${NUM_ACCOUNTS:-1000}"
INITIAL_BALANCE="${INITIAL_BALANCE:-100}"
CREDIT_AMOUNT="${CREDIT_AMOUNT:-50}"
VERIFY_SAMPLE="${VERIFY_SAMPLE:-100}"
# Unique prefix per run so re-runs never collide with long-lived actors from
# previous test runs.
RUN_ID="${RUN_ID:-$(date +%s)}"

# Extract balance from JSON response (handles "100", "100.00", 100, etc.)
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
echo "k8s-v2 Account Service - Load Test"
echo "=========================================="
echo "API: $BASE_URL"
echo "Run ID: $RUN_ID"
echo "Accounts: $NUM_ACCOUNTS (create + credit)"
echo "Verification sample: $VERIFY_SAMPLE accounts"
echo ""

# Pre-flight: verify API is reachable
echo "Checking API connectivity..."
if ! curl -sf --connect-timeout 5 -o /dev/null "$BASE_URL/openapi.yaml"; then
  echo ""
  echo "ERROR: Cannot connect to API at $BASE_URL"
  echo "       Is 'make port-forward' running in another terminal?"
  echo ""
  exit 1
fi
echo "API reachable."
echo ""

START_TIME=$(date +%s)

# Phase 1: Create accounts
echo "Phase 1: Creating $NUM_ACCOUNTS accounts (initial balance: $INITIAL_BALANCE)..."
CREATE_FAIL=0
for i in $(seq 1 "$NUM_ACCOUNTS"); do
  acc_id=$(printf "%s-acc-%04d" "$RUN_ID" "$i")
  resp=$(curl -s -w "\n%{http_code}" --connect-timeout 5 -m 10 -X POST "$BASE_URL/accounts" \
    -H "Content-Type: application/json" \
    -d "{\"create_account\":{\"account_id\":\"$acc_id\",\"account_balance\":$INITIAL_BALANCE}}")
  http_code=$(echo "$resp" | tail -n1)
  body=$(echo "$resp" | sed '$d')
  balance=$(get_balance "$body")

  if [ "$http_code" != "200" ] || [ -z "$balance" ]; then
    echo "  FAIL: $acc_id (HTTP $http_code)"
    ((CREATE_FAIL++)) || true
  fi

  if [ $((i % 100)) -eq 0 ]; then
    echo "  Progress: $i/$NUM_ACCOUNTS"
  fi
done

if [ "$CREATE_FAIL" -gt 0 ]; then
  echo "  Create phase: $CREATE_FAIL failures"
  exit 1
fi
echo "  Done: $NUM_ACCOUNTS accounts created"
echo ""

# Phase 2: Credit accounts
echo "Phase 2: Crediting $NUM_ACCOUNTS accounts (+$CREDIT_AMOUNT each)..."
CREDIT_FAIL=0
for i in $(seq 1 "$NUM_ACCOUNTS"); do
  acc_id=$(printf "%s-acc-%04d" "$RUN_ID" "$i")
  resp=$(curl -s -w "\n%{http_code}" --connect-timeout 5 -m 10 -X POST "$BASE_URL/accounts/$acc_id/credit" \
    -H "Content-Type: application/json" \
    -d "{\"balance\":$CREDIT_AMOUNT}")
  http_code=$(echo "$resp" | tail -n1)
  body=$(echo "$resp" | sed '$d')
  balance=$(get_balance "$body")

  expected=$((INITIAL_BALANCE + CREDIT_AMOUNT))
  if [ "$http_code" != "200" ] || [ -z "$balance" ]; then
    echo "  FAIL: $acc_id (HTTP $http_code)"
    ((CREDIT_FAIL++)) || true
  fi

  if [ $((i % 100)) -eq 0 ]; then
    echo "  Progress: $i/$NUM_ACCOUNTS"
  fi
done

if [ "$CREDIT_FAIL" -gt 0 ]; then
  echo "  Credit phase: $CREDIT_FAIL failures"
  exit 1
fi
echo "  Done: $NUM_ACCOUNTS accounts credited"
echo ""

# Phase 3: Verify sample of accounts via GET
EXPECTED_BALANCE=$((INITIAL_BALANCE + CREDIT_AMOUNT))
echo "Phase 3: Verifying $VERIFY_SAMPLE accounts (expected balance: $EXPECTED_BALANCE)..."

# Sample evenly across the range
VERIFY_FAIL=0
VERIFY_PASS=0
step=$((NUM_ACCOUNTS / VERIFY_SAMPLE))
[ "$step" -lt 1 ] && step=1

for i in $(seq 1 "$step" "$NUM_ACCOUNTS" | head -n "$VERIFY_SAMPLE"); do
  acc_id=$(printf "%s-acc-%04d" "$RUN_ID" "$i")
  resp=$(curl -s -w "\n%{http_code}" --connect-timeout 5 -m 10 "$BASE_URL/accounts/$acc_id")
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
    # Compare as numbers (handle 150 vs 150.00)
    balance_int=$(echo "$balance" | cut -d. -f1)
    if [ "$balance_int" != "$EXPECTED_BALANCE" ]; then
      echo "  FAIL: $acc_id - expected $EXPECTED_BALANCE, got $balance"
      ((VERIFY_FAIL++)) || true
    else
      ((VERIFY_PASS++)) || true
    fi
  fi
done

END_TIME=$(date +%s)
DURATION=$((END_TIME - START_TIME))

echo ""
echo "=========================================="
echo "Results"
echo "=========================================="
echo "Created:  $NUM_ACCOUNTS accounts"
echo "Credited: $NUM_ACCOUNTS accounts"
echo "Verified: $VERIFY_PASS passed, $VERIFY_FAIL failed (sample of $VERIFY_SAMPLE)"
echo "Duration: ${DURATION}s"
echo ""

if [ "$VERIFY_FAIL" -gt 0 ]; then
  echo "FAIL: $VERIFY_FAIL verification(s) failed"
  exit 1
fi

echo "All tests passed!"
