#!/usr/bin/env bash
# =============================================================================
# MID Proxy vs ECC Poller — Latency Benchmark
# =============================================================================
# Compares roundtrip latency between the two approaches by sending
# test commands through each path and measuring response time.
#
# Prerequisites:
#   - Both stacks running (docker-compose.yml for ECC poller + mid-proxy)
#   - curl, jq, bc installed
#   - ServiceNow instance accessible
#
# Usage:
#   ./mid-proxy/scripts/benchmark.sh
# =============================================================================

set -euo pipefail

# Configuration — override via environment
SN_INSTANCE="${SN_INSTANCE_URL:-https://your-instance.service-now.com}"
SN_AUTH="${SN_AUTH:-admin:password}"  # user:pass for Basic auth
ITERATIONS="${BENCH_ITERATIONS:-10}"
API_KEY="${ANTHROPIC_API_KEY:-test-key}"

GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
NC='\033[0m'

info()  { echo -e "${GREEN}[INFO]${NC}  $*"; }
warn()  { echo -e "${YELLOW}[WARN]${NC}  $*"; }
header(){ echo -e "\n${CYAN}═══════════════════════════════════════════════════${NC}"; echo -e "${CYAN}  $*${NC}"; echo -e "${CYAN}═══════════════════════════════════════════════════${NC}\n"; }

AUTH_HEADER="Authorization: Basic $(echo -n "$SN_AUTH" | base64)"

# ── Benchmark function ───────────────────────────────────────────────────────

benchmark_endpoint() {
    local label="$1"
    local url="$2"
    local method="$3"
    local data="$4"
    local total_ms=0
    local successes=0

    header "$label — $ITERATIONS iterations"

    for i in $(seq 1 "$ITERATIONS"); do
        start_ms=$(date +%s%3N)

        http_code=$(curl -s -o /dev/null -w "%{http_code}" \
            -X "$method" \
            -H "Content-Type: application/json" \
            -H "$AUTH_HEADER" \
            -d "$data" \
            "$url" 2>/dev/null || echo "000")

        end_ms=$(date +%s%3N)
        elapsed=$((end_ms - start_ms))

        if [ "$http_code" -ge 200 ] && [ "$http_code" -lt 300 ]; then
            successes=$((successes + 1))
            total_ms=$((total_ms + elapsed))
            echo "  [$i/$ITERATIONS] ${elapsed}ms (HTTP $http_code)"
        else
            echo "  [$i/$ITERATIONS] FAILED (HTTP $http_code) — ${elapsed}ms"
        fi

        sleep 1  # Don't hammer the instance
    done

    echo ""
    if [ "$successes" -gt 0 ]; then
        avg=$(echo "scale=1; $total_ms / $successes" | bc)
        info "$label: avg=${avg}ms over $successes successful requests"
    else
        warn "$label: No successful requests"
    fi

    echo "$successes $total_ms"
}

# ── Main ─────────────────────────────────────────────────────────────────────

echo ""
header "Claude Terminal — Latency Benchmark"
echo "Instance:   $SN_INSTANCE"
echo "Iterations: $ITERATIONS"
echo ""

# Test 1: ECC Poller path (original REST API)
ECC_URL="${SN_INSTANCE}/api/x_claude/terminal/session"
ECC_DATA='{"credentials":{"anthropicApiKey":"'"$API_KEY"'"},"workspaceType":"temp"}'
ECC_RESULT=$(benchmark_endpoint "ECC Poller Path" "$ECC_URL" "POST" "$ECC_DATA")

echo ""

# Test 2: MID Proxy path (new REST API)
MID_URL="${SN_INSTANCE}/api/x_claude/terminal_mid/session"
MID_DATA='{"credentials":{"anthropicApiKey":"'"$API_KEY"'"},"workspaceType":"temp"}'
MID_RESULT=$(benchmark_endpoint "MID Proxy Path" "$MID_URL" "POST" "$MID_DATA")

# ── Summary ──────────────────────────────────────────────────────────────────

echo ""
header "Summary"

ECC_SUCCESSES=$(echo "$ECC_RESULT" | tail -1 | awk '{print $1}')
ECC_TOTAL=$(echo "$ECC_RESULT" | tail -1 | awk '{print $2}')
MID_SUCCESSES=$(echo "$MID_RESULT" | tail -1 | awk '{print $1}')
MID_TOTAL=$(echo "$MID_RESULT" | tail -1 | awk '{print $2}')

if [ "$ECC_SUCCESSES" -gt 0 ] && [ "$MID_SUCCESSES" -gt 0 ]; then
    ECC_AVG=$(echo "scale=1; $ECC_TOTAL / $ECC_SUCCESSES" | bc)
    MID_AVG=$(echo "scale=1; $MID_TOTAL / $MID_SUCCESSES" | bc)
    DIFF=$(echo "scale=1; $ECC_AVG - $MID_AVG" | bc)

    echo "  ECC Poller:  avg ${ECC_AVG}ms ($ECC_SUCCESSES/$ITERATIONS successful)"
    echo "  MID Proxy:   avg ${MID_AVG}ms ($MID_SUCCESSES/$ITERATIONS successful)"
    echo ""

    if [ "$(echo "$DIFF > 0" | bc)" -eq 1 ]; then
        info "MID Proxy is ${DIFF}ms faster per request"
    else
        DIFF_ABS=$(echo "$DIFF * -1" | bc)
        warn "ECC Poller is ${DIFF_ABS}ms faster per request"
    fi
else
    warn "Insufficient successful requests for comparison"
fi

echo ""
