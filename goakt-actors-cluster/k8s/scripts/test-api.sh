#!/usr/bin/env bash
# gRPC/Connect API integration tests for k8s account service
# Run with: sh scripts/test-api.sh
# Ensure port-forward is running: make port-forward (in another terminal)
# Requires: grpcurl (go install github.com/fullstorydev/grpcurl/cmd/grpcurl@latest)

set -e

BASE_URL="${BASE_URL:-localhost:8080}"
REPO_ROOT="${REPO_ROOT:-../..}"
PROTO_PATH="$REPO_ROOT/protos"
NUM_ACCOUNTS="${NUM_ACCOUNTS:-100}"
VERIFY_SAMPLE="${VERIFY_SAMPLE:-10}"
# Unique prefix per run so re-runs never collide with long-lived actors from
# previous test runs.
RUN_ID="${RUN_ID:-$(date +%s)}"

grpc_call() {
  grpcurl -plaintext -import-path "$PROTO_PATH" -proto sample/service.proto "$@"
}

echo "=========================================="
echo "k8s Account Service - gRPC Load Test"
echo "=========================================="
echo "API: $BASE_URL"
echo "Run ID: $RUN_ID"
echo "Accounts: $NUM_ACCOUNTS (create + credit)"
echo "Verification sample: $VERIFY_SAMPLE accounts"
echo ""

# Pre-flight: verify API is reachable
echo "Checking API connectivity..."
if ! grpc_call -d '{"create_account":{"account_id":"_ping","account_balance":0}}' "$BASE_URL" samplepb.AccountService/CreateAccount >/dev/null 2>&1; then
  echo ""
  echo "ERROR: Cannot connect to gRPC API at $BASE_URL"
  echo "       Is 'make port-forward' running in another terminal?"
  echo "       Is grpcurl installed? (go install github.com/fullstorydev/grpcurl/cmd/grpcurl@latest)"
  echo ""
  exit 1
fi
echo "API reachable."
echo ""

START_TIME=$(date +%s)

# Phase 1: Create accounts
echo "Phase 1: Creating $NUM_ACCOUNTS accounts (initial balance: 100)..."
CREATE_FAIL=0
for i in $(seq 1 "$NUM_ACCOUNTS"); do
  acc_id=$(printf "%s-acc-%04d" "$RUN_ID" "$i")
  if ! grpc_call -d "{\"create_account\":{\"account_id\":\"$acc_id\",\"account_balance\":100}}" \
    "$BASE_URL" samplepb.AccountService/CreateAccount 2>/dev/null | grep -q account_balance; then
    echo "  FAIL: $acc_id"
    ((CREATE_FAIL++)) || true
  fi
  if [ $((i % 20)) -eq 0 ]; then
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
echo "Phase 2: Crediting $NUM_ACCOUNTS accounts (+50 each)..."
CREDIT_FAIL=0
for i in $(seq 1 "$NUM_ACCOUNTS"); do
  acc_id=$(printf "%s-acc-%04d" "$RUN_ID" "$i")
  if ! grpc_call -d "{\"credit_account\":{\"account_id\":\"$acc_id\",\"balance\":50}}" \
    "$BASE_URL" samplepb.AccountService/CreditAccount 2>/dev/null | grep -q account_balance; then
    echo "  FAIL: $acc_id"
    ((CREDIT_FAIL++)) || true
  fi
  if [ $((i % 20)) -eq 0 ]; then
    echo "  Progress: $i/$NUM_ACCOUNTS"
  fi
done

if [ "$CREDIT_FAIL" -gt 0 ]; then
  echo "  Credit phase: $CREDIT_FAIL failures"
  exit 1
fi
echo "  Done: $NUM_ACCOUNTS accounts credited"
echo ""

# Phase 3: Verify sample via GetAccount
echo "Phase 3: Verifying $VERIFY_SAMPLE accounts (expected balance: 150)..."
VERIFY_FAIL=0
VERIFY_PASS=0
step=$((NUM_ACCOUNTS / VERIFY_SAMPLE))
[ "$step" -lt 1 ] && step=1

for i in $(seq 1 "$step" "$NUM_ACCOUNTS" | head -n "$VERIFY_SAMPLE"); do
  acc_id=$(printf "%s-acc-%04d" "$RUN_ID" "$i")
  resp=$(grpc_call -d "{\"account_id\":\"$acc_id\"}" "$BASE_URL" samplepb.AccountService/GetAccount 2>/dev/null || true)
  if echo "$resp" | grep -q '"account_balance":150'; then
    ((VERIFY_PASS++)) || true
  else
    echo "  FAIL: $acc_id - expected balance 150"
    ((VERIFY_FAIL++)) || true
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
