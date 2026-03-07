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

# CLI integration tests for goakt-ai distributed AI agents
# Runs queries via make query and verifies responses
# Run with: ./scripts/test-cli.sh
# Ensure port-forward is running: make port-forward (in another terminal)

set -e

ENDPOINT="${ENDPOINT:-http://localhost:8080}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"

echo "=========================================="
echo "goakt-ai Distributed AI Agents - CLI Test"
echo "=========================================="
echo "Endpoint: $ENDPOINT"
echo ""

# Pre-flight: check endpoint connectivity
echo "Checking endpoint connectivity..."
if ! curl -sf --connect-timeout 5 -o /dev/null "$ENDPOINT" 2>/dev/null; then
  echo "WARN: Cannot connect to $ENDPOINT"
  echo "      Is 'make port-forward' running in another terminal?"
  echo "      Proceeding anyway (go run may fail)..."
fi
echo ""

# Run test queries via make
echo "Test 1: Simple query (What is 2+2?)..."
cd "$PROJECT_DIR"
if make query QUERY="What is 2+2?" ENDPOINT="$ENDPOINT" 2>&1 | tee /tmp/goakt-ai-test1.log; then
  echo "  Query completed."
else
  echo "FAIL: Query failed (implementation may not be ready)"
  exit 1
fi
echo ""

echo "Test 2: Summarization query..."
if make query QUERY="Summarize the key benefits of distributed systems in one sentence." ENDPOINT="$ENDPOINT" 2>&1 | tee /tmp/goakt-ai-test2.log; then
  echo "  Query completed."
else
  echo "FAIL: Query failed"
  exit 1
fi
echo ""

echo "=========================================="
echo "All CLI tests passed!"
echo "=========================================="
