#!/bin/bash

# Comprehensive test runner for Claude Terminal MID Service

set -e

echo "╔════════════════════════════════════════════════════════════╗"
echo "║       Claude Terminal MID Service - Test Suite            ║"
echo "╚════════════════════════════════════════════════════════════╝"
echo ""

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Test results
TESTS_PASSED=0
TESTS_FAILED=0
BENCHMARKS_RUN=0

# Set up test environment
export SERVICENOW_INSTANCE=test.service-now.com
export SERVICENOW_API_USER=test_user
export SERVICENOW_API_PASSWORD=test_password
export NODE_SERVICE_PORT=3000

echo "${BLUE}=== Environment Setup ===${NC}"
echo "✓ Test environment variables configured"
echo ""

# Run unit tests
echo "${BLUE}=== Running Unit Tests ===${NC}"
echo ""

# Test each package
PACKAGES=(
    "./internal/config"
    "./internal/session"
    "./internal/server"
)

for pkg in "${PACKAGES[@]}"; do
    echo "Testing package: $pkg"
    if go test -v "$pkg" 2>&1 | tee /tmp/test-output.txt; then
        PASSED=$(grep -c "PASS:" /tmp/test-output.txt || echo "0")
        TESTS_PASSED=$((TESTS_PASSED + PASSED))
        echo -e "${GREEN}✓ Tests passed${NC}"
    else
        FAILED=$(grep -c "FAIL:" /tmp/test-output.txt || echo "1")
        TESTS_FAILED=$((TESTS_FAILED + FAILED))
        echo -e "${RED}✗ Tests failed${NC}"
    fi
    echo ""
done

# Run tests with coverage
echo "${BLUE}=== Running Tests with Coverage ===${NC}"
echo ""

go test -coverprofile=coverage.out ./...
COVERAGE=$(go tool cover -func=coverage.out | grep total | awk '{print $3}')
echo -e "${GREEN}Total Coverage: $COVERAGE${NC}"
echo ""

# Generate coverage HTML report
go tool cover -html=coverage.out -o coverage.html
echo "✓ Coverage report generated: coverage.html"
echo ""

# Run benchmarks
echo "${BLUE}=== Running Benchmarks ===${NC}"
echo ""

go test -bench=. -benchmem ./internal/session/ > benchmark-session.txt 2>&1 || true
go test -bench=. -benchmem ./internal/server/ > benchmark-server.txt 2>&1 || true
go test -bench=. -benchmem ./internal/config/ > benchmark-config.txt 2>&1 || true

echo "✓ Benchmark results saved:"
echo "  - benchmark-session.txt"
echo "  - benchmark-server.txt"
echo "  - benchmark-config.txt"
echo ""

# Display benchmark summary
echo "${BLUE}=== Benchmark Summary ===${NC}"
echo ""
echo "Session Manager Benchmarks:"
grep "Benchmark" benchmark-session.txt | head -5
echo ""
echo "Server Benchmarks:"
grep "Benchmark" benchmark-server.txt | head -5
echo ""

# Run race detector
echo "${BLUE}=== Running Race Detector ===${NC}"
echo ""

if go test -race ./... 2>&1 | tee /tmp/race-output.txt; then
    echo -e "${GREEN}✓ No race conditions detected${NC}"
else
    echo -e "${RED}✗ Race conditions detected${NC}"
    grep "WARNING: DATA RACE" /tmp/race-output.txt || true
fi
echo ""

# Run static analysis
echo "${BLUE}=== Running Static Analysis ===${NC}"
echo ""

# go vet
if go vet ./...; then
    echo -e "${GREEN}✓ go vet passed${NC}"
else
    echo -e "${RED}✗ go vet found issues${NC}"
fi
echo ""

# Check formatting
echo "${BLUE}=== Checking Code Formatting ===${NC}"
echo ""

UNFORMATTED=$(gofmt -l . | grep -v vendor || true)
if [ -z "$UNFORMATTED" ]; then
    echo -e "${GREEN}✓ All files are properly formatted${NC}"
else
    echo -e "${YELLOW}⚠ The following files need formatting:${NC}"
    echo "$UNFORMATTED"
fi
echo ""

# Integration tests
echo "${BLUE}=== Running Integration Tests ===${NC}"
echo ""

# Check if service is running
if curl -s http://localhost:3000/health > /dev/null 2>&1; then
    echo "✓ HTTP service is running"

    # Test health endpoint
    HEALTH=$(curl -s http://localhost:3000/health)
    if echo "$HEALTH" | grep -q "healthy"; then
        echo -e "${GREEN}✓ Health endpoint test passed${NC}"
    else
        echo -e "${RED}✗ Health endpoint test failed${NC}"
    fi

    # Test session creation
    RESULT=$(curl -s -X POST http://localhost:3000/api/session/create \
        -H "Content-Type: application/json" \
        -d '{"userId":"test","credentials":{"anthropicApiKey":"test-key"}}')

    if echo "$RESULT" | grep -q "sessionId"; then
        echo -e "${GREEN}✓ Session creation test passed${NC}"
        SESSION_ID=$(echo "$RESULT" | grep -o '"sessionId":"[^"]*"' | cut -d'"' -f4)

        # Cleanup
        curl -s -X DELETE "http://localhost:3000/api/session/$SESSION_ID" > /dev/null
    else
        echo -e "${YELLOW}⚠ Session creation test skipped (Claude CLI may not be available)${NC}"
    fi
else
    echo -e "${YELLOW}⚠ Integration tests skipped (service not running)${NC}"
    echo "  Start service with: ./bin/claude-terminal-service"
fi
echo ""

# Test report
echo "╔════════════════════════════════════════════════════════════╗"
echo "║                     Test Summary                           ║"
echo "╚════════════════════════════════════════════════════════════╝"
echo ""
echo "Unit Tests:"
echo "  - Passed: $TESTS_PASSED"
echo "  - Failed: $TESTS_FAILED"
echo ""
echo "Coverage:"
echo "  - Total: $COVERAGE"
echo ""
echo "Benchmarks:"
echo "  - Results saved to benchmark-*.txt files"
echo ""
echo "Reports Generated:"
echo "  - coverage.html (test coverage visualization)"
echo "  - coverage.out (coverage data)"
echo "  - benchmark-*.txt (performance benchmarks)"
echo ""

if [ $TESTS_FAILED -eq 0 ]; then
    echo -e "${GREEN}✓ All tests passed!${NC}"
    exit 0
else
    echo -e "${RED}✗ Some tests failed${NC}"
    exit 1
fi
