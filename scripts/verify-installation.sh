#!/bin/bash

# Verification script for Claude Terminal Service installation
# This script checks if all components are properly configured

set -e

echo "╔════════════════════════════════════════════════════════════╗"
echo "║   Claude Terminal Service - Installation Verification     ║"
echo "╚════════════════════════════════════════════════════════════╝"
echo ""

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Check counters
CHECKS_PASSED=0
CHECKS_FAILED=0
CHECKS_WARNING=0

check_pass() {
    echo -e "${GREEN}✓${NC} $1"
    ((CHECKS_PASSED++))
}

check_fail() {
    echo -e "${RED}✗${NC} $1"
    ((CHECKS_FAILED++))
}

check_warn() {
    echo -e "${YELLOW}⚠${NC} $1"
    ((CHECKS_WARNING++))
}

echo "=== System Requirements ==="

# Check Go version
if command -v go &> /dev/null; then
    GO_VERSION=$(go version | awk '{print $3}')
    check_pass "Go is installed ($GO_VERSION)"
else
    check_fail "Go is not installed"
fi

# Check Claude CLI
if command -v claude &> /dev/null; then
    check_pass "Claude CLI is installed"
else
    check_fail "Claude CLI is not installed"
fi

echo ""
echo "=== Project Files ==="

# Check required files
FILES_TO_CHECK=(
    "go.mod"
    "go.sum"
    ".env"
    "Makefile"
    "cmd/server/main.go"
    "cmd/ecc-poller/main.go"
    "internal/config/config.go"
    "internal/session/session.go"
    "internal/server/server.go"
    "internal/servicenow/client.go"
)

for file in "${FILES_TO_CHECK[@]}"; do
    if [ -f "$file" ]; then
        check_pass "Found: $file"
    else
        check_fail "Missing: $file"
    fi
done

echo ""
echo "=== Configuration ==="

# Check .env file
if [ -f ".env" ]; then
    check_pass ".env file exists"

    # Check required env vars
    source .env 2>/dev/null || true

    if [ -n "$SERVICENOW_INSTANCE" ]; then
        check_pass "SERVICENOW_INSTANCE is set"
    else
        check_fail "SERVICENOW_INSTANCE is not set"
    fi

    if [ -n "$SERVICENOW_API_USER" ]; then
        check_pass "SERVICENOW_API_USER is set"
    else
        check_fail "SERVICENOW_API_USER is not set"
    fi

    if [ -n "$SERVICENOW_API_PASSWORD" ]; then
        check_pass "SERVICENOW_API_PASSWORD is set"
    else
        check_fail "SERVICENOW_API_PASSWORD is not set"
    fi
else
    check_fail ".env file does not exist"
fi

echo ""
echo "=== Build Status ==="

# Check if binaries exist
if [ -f "bin/claude-terminal-service" ]; then
    check_pass "claude-terminal-service binary exists"
else
    check_warn "claude-terminal-service binary not found (run 'make build')"
fi

if [ -f "bin/ecc-poller" ]; then
    check_pass "ecc-poller binary exists"
else
    check_warn "ecc-poller binary not found (run 'make build')"
fi

echo ""
echo "=== Dependencies ==="

# Check Go dependencies
if go mod verify &> /dev/null; then
    check_pass "Go modules are verified"
else
    check_warn "Go modules need to be downloaded (run 'make deps')"
fi

echo ""
echo "=== ServiceNow Components ==="

# Check ServiceNow files
SERVICENOW_FILES=(
    "servicenow/tables/x_claude_terminal_session.json"
    "servicenow/tables/x_claude_credentials.json"
    "servicenow/rest-api/claude_terminal_api.xml"
    "servicenow/business-rules/amb_output_notification.xml"
    "servicenow/widgets/claude_terminal/widget.html"
    "servicenow/widgets/claude_terminal/client_script.js"
    "servicenow/widgets/claude_credential_setup/widget.html"
    "servicenow/widgets/claude_credential_setup/client_script.js"
)

for file in "${SERVICENOW_FILES[@]}"; do
    if [ -f "$file" ]; then
        check_pass "Found: $(basename $file)"
    else
        check_fail "Missing: $file"
    fi
done

echo ""
echo "=== Deployment Files ==="

# Check deployment files
if [ -f "deployment/systemd/claude-terminal-service.service" ]; then
    check_pass "SystemD service file exists"
else
    check_warn "SystemD service file not found"
fi

if [ -f "deployment/systemd/claude-ecc-poller.service" ]; then
    check_pass "SystemD poller service file exists"
else
    check_warn "SystemD poller service file not found"
fi

echo ""
echo "=== Runtime Checks ==="

# Check if service is running (if systemd is available)
if command -v systemctl &> /dev/null; then
    if systemctl is-active --quiet claude-terminal-service; then
        check_pass "claude-terminal-service is running"
    else
        check_warn "claude-terminal-service is not running"
    fi

    if systemctl is-active --quiet claude-ecc-poller; then
        check_pass "claude-ecc-poller is running"
    else
        check_warn "claude-ecc-poller is not running"
    fi
else
    check_warn "systemd not available (cannot check service status)"
fi

# Check if port 3000 is listening
if command -v lsof &> /dev/null; then
    if lsof -i :3000 &> /dev/null; then
        check_pass "Port 3000 is listening"
    else
        check_warn "Port 3000 is not listening"
    fi
fi

# Check if service responds to health check
if curl -sf http://localhost:3000/health &> /dev/null; then
    check_pass "HTTP service health check passed"
else
    check_warn "HTTP service is not responding (or not running)"
fi

echo ""
echo "=== Summary ==="
echo ""
echo -e "Checks passed:  ${GREEN}$CHECKS_PASSED${NC}"
echo -e "Checks failed:  ${RED}$CHECKS_FAILED${NC}"
echo -e "Warnings:       ${YELLOW}$CHECKS_WARNING${NC}"
echo ""

if [ $CHECKS_FAILED -eq 0 ]; then
    echo -e "${GREEN}✓ All critical checks passed!${NC}"
    echo ""
    echo "Next steps:"
    echo "1. Build binaries: make build"
    echo "2. Start services: sudo systemctl start claude-terminal-service claude-ecc-poller"
    echo "3. Import ServiceNow components"
    echo "4. Test in ServiceNow UI"
    exit 0
else
    echo -e "${RED}✗ Some checks failed. Please review the output above.${NC}"
    echo ""
    echo "Common fixes:"
    echo "- Install Go: https://go.dev/doc/install"
    echo "- Install Claude CLI: https://docs.anthropic.com/claude/docs/cli"
    echo "- Copy .env.example to .env and configure"
    echo "- Run 'make build' to compile binaries"
    exit 1
fi
