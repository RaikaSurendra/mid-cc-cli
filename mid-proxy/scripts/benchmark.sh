#!/bin/bash
#
# Benchmark: ECC Poller (original) vs MID Server Proxy
#
# Measures end-to-end latency for each operation through both paths.
# Run both setups simultaneously on different ports to compare.
#
# Original setup: docker compose -f docker-compose.yml up
#   -> HTTP Service on port 3000
#
# MID Proxy setup: docker compose -f mid-proxy/docker-compose.yml up
#   -> HTTP Service on port 3001
#
# Usage:
#   chmod +x mid-proxy/scripts/benchmark.sh
#   ./mid-proxy/scripts/benchmark.sh

set -euo pipefail

# Configuration
ORIGINAL_URL="http://localhost:3000"
MIDPROXY_URL="http://localhost:3001"
AUTH_TOKEN="mid-llm-cli-dev-token-2026"
ITERATIONS=10
RESULTS_DIR="mid-proxy/benchmark-results"

mkdir -p "$RESULTS_DIR"
TIMESTAMP=$(date +%Y%m%d_%H%M%S)
RESULTS_FILE="$RESULTS_DIR/benchmark_${TIMESTAMP}.txt"

# Colors
GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
NC='\033[0m'

echo "=============================================" | tee "$RESULTS_FILE"
echo " Claude Terminal - Performance Benchmark"      | tee -a "$RESULTS_FILE"
echo " $(date)"                                      | tee -a "$RESULTS_FILE"
echo "=============================================" | tee -a "$RESULTS_FILE"
echo ""                                               | tee -a "$RESULTS_FILE"

# Helper: measure HTTP request time in milliseconds
measure_request() {
    local method=$1
    local url=$2
    local data=$3
    local extra_headers=$4

    local start=$(python3 -c 'import time; print(int(time.time() * 1000))')

    if [ "$method" = "GET" ]; then
        curl -s -o /dev/null -w "%{http_code}" \
            -H "Authorization: Bearer $AUTH_TOKEN" \
            -H "Content-Type: application/json" \
            $extra_headers \
            "$url" 2>/dev/null
    elif [ "$method" = "POST" ]; then
        curl -s -o /dev/null -w "%{http_code}" \
            -X POST \
            -H "Authorization: Bearer $AUTH_TOKEN" \
            -H "Content-Type: application/json" \
            $extra_headers \
            -d "$data" \
            "$url" 2>/dev/null
    elif [ "$method" = "DELETE" ]; then
        curl -s -o /dev/null -w "%{http_code}" \
            -X DELETE \
            -H "Authorization: Bearer $AUTH_TOKEN" \
            -H "Content-Type: application/json" \
            $extra_headers \
            "$url" 2>/dev/null
    fi

    local end=$(python3 -c 'import time; print(int(time.time() * 1000))')
    echo $((end - start))
}

# Helper: measure and store results
benchmark_operation() {
    local name=$1
    local method=$2
    local path=$3
    local data=$4
    local extra_headers=$5
    local base_url=$6
    local label=$7

    local total=0
    local min=999999
    local max=0
    local times=()

    for i in $(seq 1 $ITERATIONS); do
        local ms=$(measure_request "$method" "${base_url}${path}" "$data" "$extra_headers")
        times+=($ms)
        total=$((total + ms))
        if [ $ms -lt $min ]; then min=$ms; fi
        if [ $ms -gt $max ]; then max=$ms; fi
    done

    local avg=$((total / ITERATIONS))

    printf "  %-25s avg=%4dms  min=%4dms  max=%4dms\n" "$label" "$avg" "$min" "$max" | tee -a "$RESULTS_FILE"
}

# ==============================
# Test 1: Health Check Latency
# ==============================
echo -e "${GREEN}Test 1: Health Check Latency ($ITERATIONS iterations)${NC}" | tee -a "$RESULTS_FILE"

benchmark_operation "health" "GET" "/health" "" "" "$ORIGINAL_URL" "Original (direct):"
benchmark_operation "health" "GET" "/health" "" "" "$MIDPROXY_URL" "MID Proxy:"
echo "" | tee -a "$RESULTS_FILE"

# ==============================
# Test 2: Session Create Latency
# ==============================
echo -e "${GREEN}Test 2: Session Create + Terminate ($ITERATIONS iterations)${NC}" | tee -a "$RESULTS_FILE"

# Original
create_total_orig=0
for i in $(seq 1 $ITERATIONS); do
    start=$(python3 -c 'import time; print(int(time.time() * 1000))')

    session_id=$(curl -s -X POST \
        -H "Authorization: Bearer $AUTH_TOKEN" \
        -H "Content-Type: application/json" \
        -d '{"userId":"bench_user","credentials":{"anthropicApiKey":"sk-test-key"},"workspaceType":"isolated"}' \
        "${ORIGINAL_URL}/api/session/create" 2>/dev/null | python3 -c "import sys,json; print(json.load(sys.stdin).get('sessionId',''))" 2>/dev/null || echo "")

    end=$(python3 -c 'import time; print(int(time.time() * 1000))')
    ms=$((end - start))
    create_total_orig=$((create_total_orig + ms))

    # Cleanup
    if [ -n "$session_id" ]; then
        curl -s -X DELETE \
            -H "Authorization: Bearer $AUTH_TOKEN" \
            -H "X-User-ID: bench_user" \
            "${ORIGINAL_URL}/api/session/${session_id}" >/dev/null 2>&1 || true
    fi
done
avg_orig=$((create_total_orig / ITERATIONS))
printf "  %-25s avg=%4dms\n" "Original (direct):" "$avg_orig" | tee -a "$RESULTS_FILE"

# MID Proxy
create_total_mid=0
for i in $(seq 1 $ITERATIONS); do
    start=$(python3 -c 'import time; print(int(time.time() * 1000))')

    session_id=$(curl -s -X POST \
        -H "Authorization: Bearer $AUTH_TOKEN" \
        -H "Content-Type: application/json" \
        -d '{"userId":"bench_user","credentials":{"anthropicApiKey":"sk-test-key"},"workspaceType":"isolated"}' \
        "${MIDPROXY_URL}/api/session/create" 2>/dev/null | python3 -c "import sys,json; print(json.load(sys.stdin).get('sessionId',''))" 2>/dev/null || echo "")

    end=$(python3 -c 'import time; print(int(time.time() * 1000))')
    ms=$((end - start))
    create_total_mid=$((create_total_mid + ms))

    # Cleanup
    if [ -n "$session_id" ]; then
        curl -s -X DELETE \
            -H "Authorization: Bearer $AUTH_TOKEN" \
            -H "X-User-ID: bench_user" \
            "${MIDPROXY_URL}/api/session/${session_id}" >/dev/null 2>&1 || true
    fi
done
avg_mid=$((create_total_mid / ITERATIONS))
printf "  %-25s avg=%4dms\n" "MID Proxy:" "$avg_mid" | tee -a "$RESULTS_FILE"
echo "" | tee -a "$RESULTS_FILE"

# ==============================
# Test 3: Command Send Latency
# ==============================
echo -e "${GREEN}Test 3: Send Command Latency ($ITERATIONS iterations)${NC}" | tee -a "$RESULTS_FILE"

# Create sessions for testing
orig_session=$(curl -s -X POST \
    -H "Authorization: Bearer $AUTH_TOKEN" \
    -H "Content-Type: application/json" \
    -d '{"userId":"bench_cmd","credentials":{"anthropicApiKey":"sk-test-key"},"workspaceType":"isolated"}' \
    "${ORIGINAL_URL}/api/session/create" 2>/dev/null | python3 -c "import sys,json; print(json.load(sys.stdin).get('sessionId',''))" 2>/dev/null || echo "")

mid_session=$(curl -s -X POST \
    -H "Authorization: Bearer $AUTH_TOKEN" \
    -H "Content-Type: application/json" \
    -d '{"userId":"bench_cmd","credentials":{"anthropicApiKey":"sk-test-key"},"workspaceType":"isolated"}' \
    "${MIDPROXY_URL}/api/session/create" 2>/dev/null | python3 -c "import sys,json; print(json.load(sys.stdin).get('sessionId',''))" 2>/dev/null || echo "")

if [ -n "$orig_session" ]; then
    benchmark_operation "cmd" "POST" "/api/session/${orig_session}/command" \
        '{"command":"echo test\n"}' "-H 'X-User-ID: bench_cmd'" "$ORIGINAL_URL" "Original (direct):"
fi

if [ -n "$mid_session" ]; then
    benchmark_operation "cmd" "POST" "/api/session/${mid_session}/command" \
        '{"command":"echo test\n"}' "-H 'X-User-ID: bench_cmd'" "$MIDPROXY_URL" "MID Proxy:"
fi

# Cleanup
[ -n "$orig_session" ] && curl -s -X DELETE -H "Authorization: Bearer $AUTH_TOKEN" -H "X-User-ID: bench_cmd" "${ORIGINAL_URL}/api/session/${orig_session}" >/dev/null 2>&1 || true
[ -n "$mid_session" ] && curl -s -X DELETE -H "Authorization: Bearer $AUTH_TOKEN" -H "X-User-ID: bench_cmd" "${MIDPROXY_URL}/api/session/${mid_session}" >/dev/null 2>&1 || true

echo "" | tee -a "$RESULTS_FILE"

# ==============================
# Summary
# ==============================
echo "=============================================" | tee -a "$RESULTS_FILE"
echo " Summary" | tee -a "$RESULTS_FILE"
echo "=============================================" | tee -a "$RESULTS_FILE"
echo "" | tee -a "$RESULTS_FILE"
echo "Original setup:  ECC Poller -> direct HTTP to Go service" | tee -a "$RESULTS_FILE"
echo "MID Proxy setup: MID Server -> JavascriptProbe -> HTTP to Go service" | tee -a "$RESULTS_FILE"
echo "" | tee -a "$RESULTS_FILE"
echo "Note: MID Proxy latency includes:" | tee -a "$RESULTS_FILE"
echo "  - ECC Queue write (~1-2s)" | tee -a "$RESULTS_FILE"
echo "  - MID Server pickup (~1-3s)" | tee -a "$RESULTS_FILE"
echo "  - Probe execution (~50-200ms)" | tee -a "$RESULTS_FILE"
echo "  - ECC Queue response write (~1-2s)" | tee -a "$RESULTS_FILE"
echo "" | tee -a "$RESULTS_FILE"
echo "Direct HTTP tests above measure only the Go service layer." | tee -a "$RESULTS_FILE"
echo "For true E2E comparison, measure from the ServiceNow widget." | tee -a "$RESULTS_FILE"
echo "" | tee -a "$RESULTS_FILE"
echo "Results saved to: $RESULTS_FILE" | tee -a "$RESULTS_FILE"
