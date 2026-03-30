#!/usr/bin/env bash
# Multi-DC Isolated — Cross-DC integration tests
# Tests cross-datacenter actor placement and messaging across two separate Kind clusters.
set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"

if [ ! -f "${PROJECT_DIR}/.env.ips" ]; then
  echo "ERROR: .env.ips not found. Run 'make setup && make deploy' first."
  exit 1
fi

source "${PROJECT_DIR}/.env.ips"

# Docker network IPs are only reachable from within Docker.
# All curl calls run via docker exec on the respective Kind node.
BASE_URL="http://127.0.0.1:8080"
DC1_URL="${BASE_URL}"
DC2_URL="${BASE_URL}"

# Wrappers to run curl from within each DC's Kind node
dc1_curl() { docker exec dc1-control-plane curl "$@"; }
dc2_curl() { docker exec dc2-control-plane curl "$@"; }
NUM_ACCOUNTS="${NUM_ACCOUNTS:-50}"
INITIAL_BALANCE="${INITIAL_BALANCE:-100}"
CREDIT_AMOUNT="${CREDIT_AMOUNT:-50}"
RUN_ID="${RUN_ID:-$(date +%s)}"

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
echo "Multi-DC Isolated - Integration Test"
echo "=========================================="
echo "DC-1 API: $DC1_URL (Kind cluster: dc1)"
echo "DC-2 API: $DC2_URL (Kind cluster: dc2)"
echo "Run ID: $RUN_ID"
echo ""

# Pre-flight: verify both DCs are reachable
echo "Phase 0: Checking API connectivity..."
if ! dc1_curl -sf --connect-timeout 5 -o /dev/null "$DC1_URL/openapi.yaml"; then
  echo "ERROR: Cannot connect to DC-1"
  exit 1
fi
echo "  DC-1 reachable"

if ! dc2_curl -sf --connect-timeout 5 -o /dev/null "$DC2_URL/openapi.yaml"; then
  echo "ERROR: Cannot connect to DC-2"
  exit 1
fi
echo "  DC-2 reachable"
echo ""

# Wait for DC readiness
echo "Phase 0.5: Waiting for datacenter readiness..."
for dc_label in DC-1 DC-2; do
  if [ "$dc_label" = "DC-1" ]; then dc_fn=dc1_curl; else dc_fn=dc2_curl; fi
  elapsed=0
  while true; do
    ready=$($dc_fn -s "$BASE_URL/dc/status" | grep -o '"ready":true' || true)
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
dc1_status=$(dc1_curl -s "$DC1_URL/dc/status")
dc2_status=$(dc2_curl -s "$DC2_URL/dc/status")
echo "  DC-1 status: $dc1_status"
echo "  DC-2 status: $dc2_status"
echo ""

START_TIME=$(date +%s)

# Phase 1: Create accounts in DC-1
echo "Phase 1: Creating $NUM_ACCOUNTS accounts in DC-1 (initial balance: $INITIAL_BALANCE)..."
CREATE_FAIL=0
for i in $(seq 1 "$NUM_ACCOUNTS"); do
  acc_id=$(printf "%s-acc-%04d" "$RUN_ID" "$i")
  resp=$(dc1_curl -s -w "\n%{http_code}" --connect-timeout 5 -m 10 -X POST "$DC1_URL/accounts" \
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

# Phase 2: Query accounts from DC-2 (cross-DC lookup across separate Kind clusters)
echo "Phase 2: Querying $NUM_ACCOUNTS accounts from DC-2 (cross-DC lookup)..."
QUERY_FAIL=0
QUERY_PASS=0
for i in $(seq 1 "$NUM_ACCOUNTS"); do
  acc_id=$(printf "%s-acc-%04d" "$RUN_ID" "$i")
  resp=$(dc2_curl -s -w "\n%{http_code}" --connect-timeout 5 -m 10 "$DC2_URL/accounts/$acc_id")
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
  resp=$(dc2_curl -s -w "\n%{http_code}" --connect-timeout 5 -m 10 -X POST "$DC2_URL/accounts/$acc_id/credit" \
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
  resp=$(dc1_curl -s -w "\n%{http_code}" --connect-timeout 5 -m 10 "$DC1_URL/accounts/$acc_id")
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

END_TIME=$(date +%s)
DURATION=$((END_TIME - START_TIME))

echo "=========================================="
echo "Results"
echo "=========================================="
echo "Created (DC-1):     $NUM_ACCOUNTS accounts"
echo "Cross-DC query:     $QUERY_PASS passed, $QUERY_FAIL failed"
echo "Credited (DC-2):    $NUM_ACCOUNTS accounts"
echo "Final verify (DC-1): $VERIFY_PASS passed, $VERIFY_FAIL failed"
echo "Duration:           ${DURATION}s"
echo ""

TOTAL_FAIL=$((QUERY_FAIL + VERIFY_FAIL))
if [ "$TOTAL_FAIL" -gt 0 ]; then
  echo "FAIL: $TOTAL_FAIL verification(s) failed"
  exit 1
fi

echo "All tests passed!"
