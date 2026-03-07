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

# Resilience test for k8s account service (gRPC)
#
# Flow:
#   Phase 1 – Create NUM_ACCOUNTS accounts
#   Phase 2 – Credit each account
#   Phase 3 – Verify a sample via GetAccount (baseline)
#   Phase 4 – Randomly pick one accounts pod and delete it gracefully
#   Phase 5 – Wait WAIT_AFTER_KILL seconds, then wait for all pods to be Ready again
#   Phase 6 – Re-verify the same sample to confirm data survived the node loss
#
# Run with:   ./scripts/test-resilience.sh
# Requires:   make port-forward (in another terminal)
#             grpcurl: go install github.com/fullstorydev/grpcurl/cmd/grpcurl@latest

set -e

BASE_URL="${BASE_URL:-localhost:8080}"
REPO_ROOT="${REPO_ROOT:-../..}"
PROTO_PATH="$REPO_ROOT/protos"
NUM_ACCOUNTS="${NUM_ACCOUNTS:-100}"
VERIFY_SAMPLE="${VERIFY_SAMPLE:-10}"
WAIT_AFTER_KILL="${WAIT_AFTER_KILL:-30}"
# Unique prefix per run so re-runs never collide with long-lived actors from
# previous test runs.
RUN_ID="${RUN_ID:-$(date +%s)}"

EXPECTED_BALANCE=150  # 100 initial + 50 credit

grpc_call() {
  grpcurl -plaintext -import-path "$PROTO_PATH" -proto sample/service.proto "$@"
}

echo "=========================================="
echo "k8s Account Service - gRPC Resilience Test"
echo "=========================================="
echo "API:              $BASE_URL"
echo "Run ID:           $RUN_ID"
echo "Accounts:         $NUM_ACCOUNTS"
echo "Verify sample:    $VERIFY_SAMPLE accounts"
echo "Wait after kill:  ${WAIT_AFTER_KILL}s"
echo ""

# Pre-flight: verify API is reachable
echo "Checking API connectivity..."
if ! grpc_call -d '{"create_account":{"account_id":"_ping","account_balance":0}}' \
    "$BASE_URL" samplepb.AccountService/CreateAccount >/dev/null 2>&1; then
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

# ---------------------------------------------------------------------------
# Phase 1: Create accounts
# ---------------------------------------------------------------------------
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

# ---------------------------------------------------------------------------
# Phase 2: Credit accounts
# ---------------------------------------------------------------------------
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

# ---------------------------------------------------------------------------
# Phase 3: Baseline verification (before any node shutdown)
# ---------------------------------------------------------------------------
echo "Phase 3: Verifying $VERIFY_SAMPLE accounts before node shutdown (expected balance: $EXPECTED_BALANCE)..."
VERIFY_FAIL=0
VERIFY_PASS=0

step=$((NUM_ACCOUNTS / VERIFY_SAMPLE))
[ "$step" -lt 1 ] && step=1

SAMPLED_IDS=()
for i in $(seq 1 "$step" "$NUM_ACCOUNTS" | head -n "$VERIFY_SAMPLE"); do
  SAMPLED_IDS+=("$i")
  acc_id=$(printf "%s-acc-%04d" "$RUN_ID" "$i")
  resp=$(grpc_call -d "{\"account_id\":\"$acc_id\"}" "$BASE_URL" samplepb.AccountService/GetAccount 2>/dev/null || true)
  if echo "$resp" | grep -q "\"account_balance\":$EXPECTED_BALANCE"; then
    ((VERIFY_PASS++)) || true
  else
    echo "  FAIL: $acc_id - expected balance $EXPECTED_BALANCE (got: $resp)"
    ((VERIFY_FAIL++)) || true
  fi
done

if [ "$VERIFY_FAIL" -gt 0 ]; then
  echo "  Baseline verification: $VERIFY_FAIL failure(s) — aborting before node kill"
  exit 1
fi
echo "  Baseline passed: $VERIFY_PASS/$VERIFY_SAMPLE accounts verified"
echo ""

# ---------------------------------------------------------------------------
# Phase 4: Randomly select and gracefully terminate one accounts pod
# ---------------------------------------------------------------------------
echo "Phase 4: Randomly selecting an accounts pod to terminate gracefully..."
PODS=(accounts-0 accounts-1 accounts-2)
RANDOM_IDX=$(( RANDOM % 3 ))
TARGET_POD="${PODS[$RANDOM_IDX]}"

echo "  Selected pod: $TARGET_POD"
if ! kubectl get pod "$TARGET_POD" &>/dev/null; then
  echo "  ERROR: Pod $TARGET_POD not found — is the cluster running?"
  exit 1
fi

kubectl delete pod "$TARGET_POD"
echo "  Pod $TARGET_POD deletion requested (graceful termination)."
echo ""

# ---------------------------------------------------------------------------
# Phase 5: Wait, then confirm the cluster is healthy again
# ---------------------------------------------------------------------------
echo "Phase 5: Waiting ${WAIT_AFTER_KILL}s for the cluster to stabilize..."
sleep "$WAIT_AFTER_KILL"

echo "  Waiting for all accounts pods to be Ready (timeout 120s)..."
kubectl wait --for=condition=ready --timeout=120s pod -l app.kubernetes.io/name=accounts
echo "  All accounts pods are Ready."
echo ""

# ---------------------------------------------------------------------------
# Phase 6: Re-verify the same sampled accounts after recovery
# ---------------------------------------------------------------------------
echo "Phase 6: Re-verifying the same $VERIFY_SAMPLE accounts after node recovery (expected balance: $EXPECTED_BALANCE)..."
REVERIFY_FAIL=0
REVERIFY_PASS=0

for i in "${SAMPLED_IDS[@]}"; do
  acc_id=$(printf "%s-acc-%04d" "$RUN_ID" "$i")
  resp=$(grpc_call -d "{\"account_id\":\"$acc_id\"}" "$BASE_URL" samplepb.AccountService/GetAccount 2>/dev/null || true)
  if echo "$resp" | grep -q "\"account_balance\":$EXPECTED_BALANCE"; then
    ((REVERIFY_PASS++)) || true
  else
    echo "  FAIL: $acc_id - expected balance $EXPECTED_BALANCE (got: $resp)"
    ((REVERIFY_FAIL++)) || true
  fi
done

END_TIME=$(date +%s)
DURATION=$((END_TIME - START_TIME))

echo ""
echo "=========================================="
echo "Results"
echo "=========================================="
echo "Created:         $NUM_ACCOUNTS accounts"
echo "Credited:        $NUM_ACCOUNTS accounts"
echo "Pod terminated:  $TARGET_POD (graceful, ${WAIT_AFTER_KILL}s wait)"
echo "Re-verified:     $REVERIFY_PASS passed, $REVERIFY_FAIL failed (sample of $VERIFY_SAMPLE)"
echo "Duration:        ${DURATION}s"
echo ""

if [ "$REVERIFY_FAIL" -gt 0 ]; then
  echo "FAIL: $REVERIFY_FAIL re-verification(s) failed after node recovery"
  exit 1
fi

echo "All resilience tests passed!"
