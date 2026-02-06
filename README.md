# Claude Code Integration for ServiceNow MID Server

A terminal-based integration that brings Claude Code CLI capabilities into your ServiceNow environment. Supports two deployment modes: **ECC Poller** (custom Go binary) and **MID Server Proxy** (native ServiceNow pattern).

## Overview

This solution provides an interactive terminal UI in ServiceNow that connects to Claude Code CLI running alongside a MID Server, enabling users to leverage Claude's AI-powered coding capabilities within their ServiceNow environment.

### Key Features

- Interactive xterm.js terminal in ServiceNow Service Portal
- Real-time communication via AMB notifications + adaptive polling
- Per-user session isolation with encrypted credentials (AES-256-GCM)
- PostgreSQL session persistence with async writes
- Bearer token auth with constant-time comparison
- Per-IP rate limiting (10 req/s, token bucket)
- PTY input sanitization and path traversal prevention
- Two deployment modes for performance comparison

## Architecture

This project supports two modes of bridging ServiceNow to the Claude Terminal HTTP Service:

### Mode 1: ECC Poller (Default)

A custom Go binary polls the ECC Queue and forwards commands to the HTTP service.

```
ServiceNow Instance                         MID Server Host
+-------------------------+                +---------------------------+
|                         |                |                           |
|  Service Portal Widget  |                |  ECC Poller (Go)          |
|  (xterm.js)             |                |  polls every 5s           |
|        |                |                |        |                  |
|        v                |                |        v                  |
|  Scripted REST API      |   ECC Queue    |  HTTP Service (Go/Gin)   |
|  + Session Table        |<-------------->|  :3000                    |
|        |                |                |        |                  |
|        v                |                |        v                  |
|  AMB Notifications      |                |  Session Manager + PTY   |
|                         |                |        |                  |
+-------------------------+                |        v                  |
                                           |  Claude Code CLI          |
                                           |                           |
                                           |  PostgreSQL (persistence) |
                                           +---------------------------+
```

### Mode 2: MID Server Proxy

The MID Server natively polls the ECC Queue and executes a JavascriptProbe that forwards HTTP requests. No custom poller needed.

```
ServiceNow Instance                         MID Server Host
+-------------------------+                +---------------------------+
|                         |                |                           |
|  Service Portal Widget  |                |  MID Server (native)      |
|  (xterm.js)             |                |  polls every ~2s          |
|        |                |                |        |                  |
|        v                |                |        v                  |
|  Scripted REST API      |   ECC Queue    |  ClaudeTerminalProbe.js  |
|  (JavascriptProbe)      |<-------------->|  (MID Script Include)     |
|        |                |                |        |                  |
|        v                |                |        v (HTTP)           |
|  ClaudeTerminalAPI      |                |  HTTP Service (Go/Gin)   |
|  (Script Include)       |                |  :3000                    |
|                         |                |        |                  |
+-------------------------+                |        v                  |
                                           |  Session Manager + PTY   |
                                           |        |                  |
                                           |        v                  |
                                           |  Claude Code CLI          |
                                           |                           |
                                           |  PostgreSQL (persistence) |
                                           +---------------------------+
```

### Mode Comparison

| Aspect | ECC Poller | MID Server Proxy |
|--------|-----------|-----------------|
| Docker services | 4 (postgres, http, poller, mid) | 3 (postgres, http, mid) |
| ECC poll interval | ~5s (configurable) | ~1-3s (native) |
| E2E command latency | ~5-10s | ~3-7s |
| Code to maintain | ~300 lines Go | ~200 lines JS (ServiceNow) |
| ServiceNow credentials | Needed by poller + MID | Only MID Server |
| Monitoring | Custom logging | ServiceNow MID dashboard |
| Failover | Manual | MID Server cluster support |
| Memory overhead | ~20MB (Go poller) | MID Server JVM (1-4GB) |

## Project Structure

```
mid-llm-cli/
├── cmd/
│   ├── server/main.go                 # HTTP service entry point
│   └── ecc-poller/main.go             # ECC Queue poller entry point
├── internal/
│   ├── config/config.go               # Configuration (env vars)
│   ├── server/server.go               # REST API handlers + auth
│   ├── session/session.go             # PTY session manager
│   ├── store/postgres.go              # PostgreSQL persistence
│   ├── servicenow/client.go           # ServiceNow + HTTP clients
│   ├── crypto/crypto.go               # AES-256-GCM encryption
│   ├── logging/logging.go             # Structured logging
│   └── middleware/ratelimit.go        # Per-IP rate limiting
├── mid-proxy/                          # MID Server Proxy (alternative to ECC Poller)
│   ├── docker-compose.yml             # 3-service deployment
│   ├── docs/                          # Architecture + setup guide
│   ├── scripts/benchmark.sh           # Side-by-side perf comparison
│   └── servicenow/
│       ├── script-includes/           # ClaudeTerminalAPI + ClaudeTerminalProbe
│       ├── scripted-rest/             # MID Proxy REST endpoints
│       ├── business-rules/            # AMB output notification
│       ├── widgets/claude_terminal_mid/ # Terminal widget (MID variant)
│       └── fix-scripts/               # Table + property + ACL creation scripts
├── servicenow/                         # ServiceNow components (ECC Poller mode)
│   ├── tables/                        # Table definitions (JSON)
│   ├── rest-api/                      # Scripted REST API (XML)
│   ├── business-rules/                # AMB notifications
│   └── widgets/                       # Service Portal widgets
├── kubernetes/                         # K8s deployment manifests
├── deployment/systemd/                 # systemd unit files
├── scripts/                            # Build/test/verify scripts
├── docs/                               # Low-level design document
├── Dockerfile                          # Multi-stage Docker build
├── docker-compose.yml                  # 4-service orchestration (Mode 1)
├── Jenkinsfile                         # CI/CD pipeline (K8s agents)
├── Makefile                            # Build automation
└── .env.example                        # Configuration template
```

> **Note:** Jenkins infrastructure (controller, K8s manifests, plugins, docs) lives in a
> separate shared project at [`jenkins-k8s/`](../jenkins-k8s/) so it can serve all projects.

## Prerequisites

- Go 1.24+
- Docker and Docker Compose
- ServiceNow instance (Tokyo or later)
- Claude Code CLI (`npm install -g @anthropic-ai/claude-code`)
- PostgreSQL 15 (included in Docker Compose)

## Quick Start (Docker)

### Mode 1: ECC Poller (Default)

```bash
# Configure environment
cp .env.example .env
# Edit .env with your ServiceNow instance details, encryption key, and auth token

# Build and start all 4 services
docker compose up --build -d

# Verify
curl http://localhost:3000/health
docker compose ps
```

### Mode 2: MID Server Proxy

```bash
# Build and start 3 services (no ECC Poller)
docker compose -f mid-proxy/docker-compose.yml up --build -d

# Verify
curl http://localhost:3001/health
docker compose -f mid-proxy/docker-compose.yml ps

# Then configure ServiceNow (see mid-proxy/docs/SETUP_GUIDE.md)
```

### Run Both for Comparison

```bash
# Start Mode 1 on port 3000
docker compose up --build -d

# Start Mode 2 on port 3001
docker compose -f mid-proxy/docker-compose.yml up --build -d

# Run benchmark
./mid-proxy/scripts/benchmark.sh
```

## Installation (Manual)

### 1. Build

```bash
# Download dependencies and build binaries
make build

# Outputs:
#   bin/claude-terminal-service
#   bin/ecc-poller
```

### 2. Configure

```bash
cp .env.example .env
```

Edit `.env` with your configuration:

```bash
# ServiceNow
SERVICENOW_INSTANCE=your-instance.service-now.com
SERVICENOW_API_USER=integration_user
SERVICENOW_API_PASSWORD=your_password
MID_SERVER_NAME=your_mid_server_name

# Server
NODE_SERVICE_PORT=3000
NODE_SERVICE_HOST=localhost
GIN_MODE=release

# Security (required in release mode)
API_AUTH_TOKEN=your-secure-token
ENCRYPTION_KEY=your-64-char-hex-key  # generate: openssl rand -hex 32

# Database (optional, omit DB_HOST for in-memory only)
DB_HOST=localhost
DB_PORT=5432
DB_USER=postgres
DB_PASSWORD=postgres
DB_NAME=claude_terminal
DB_SSLMODE=disable
```

See `.env.example` for all available options.

### 3. ServiceNow Setup

#### Option A: Fix Scripts (Recommended)

Run the fix scripts in order from **System Definition → Fix Scripts**:

```
mid-proxy/servicenow/fix-scripts/
  01_create_terminal_session_table.js   # Tables + columns + indexes
  02_create_credentials_table.js        # Credentials with password2 encryption
  03_create_system_properties.js        # MID proxy config properties
  04_create_acls.js                     # Row-level access controls
```

See `mid-proxy/servicenow/fix-scripts/README.md` for details.

#### Option B: Manual Import

1. **Tables:** Import from `servicenow/tables/` (`x_claude_terminal_session.json`, `x_claude_credentials.json`)
2. **REST API:** Import `servicenow/rest-api/claude_terminal_api.xml`
3. **Business Rules:** Import `servicenow/business-rules/amb_output_notification.xml`
4. **Widgets:** Create from `servicenow/widgets/claude_terminal/` and `servicenow/widgets/claude_credential_setup/`

#### For MID Server Proxy mode, additionally follow: `mid-proxy/docs/SETUP_GUIDE.md`

### 4. Start Services

```bash
# Option A: Direct
./bin/claude-terminal-service &
./bin/ecc-poller &

# Option B: systemd
sudo cp deployment/systemd/*.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now claude-terminal-service
sudo systemctl enable --now claude-ecc-poller

# Option C: Docker (recommended)
docker compose up --build -d
```

## HTTP API Reference

All `/api/*` endpoints require `Authorization: Bearer <token>`. Session-specific endpoints require `X-User-ID` header.

| Method | Path | Auth | User-ID | Description |
|--------|------|------|---------|-------------|
| `GET` | `/health` | No | No | Health check + diagnostics |
| `POST` | `/api/session/create` | Yes | No | Create Claude session |
| `POST` | `/api/session/:id/command` | Yes | Yes | Send command to PTY |
| `GET` | `/api/session/:id/output` | Yes | Yes | Get buffered output |
| `GET` | `/api/session/:id/status` | Yes | Yes | Get session status |
| `POST` | `/api/session/:id/resize` | Yes | Yes | Resize terminal |
| `DELETE` | `/api/session/:id` | Yes | Yes | Terminate session |
| `GET` | `/api/sessions` | Yes | Yes | List user's sessions |

### Examples

```bash
TOKEN="your-auth-token"

# Health check
curl http://localhost:3000/health

# Create session
curl -X POST http://localhost:3000/api/session/create \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"userId":"john.doe","credentials":{"anthropicApiKey":"sk-ant-..."},"workspaceType":"isolated"}'

# Send command
curl -X POST http://localhost:3000/api/session/{sessionId}/command \
  -H "Authorization: Bearer $TOKEN" \
  -H "X-User-ID: john.doe" \
  -H "Content-Type: application/json" \
  -d '{"command":"help\n"}'

# Get output
curl http://localhost:3000/api/session/{sessionId}/output?clear=true \
  -H "Authorization: Bearer $TOKEN" \
  -H "X-User-ID: john.doe"

# Terminate
curl -X DELETE http://localhost:3000/api/session/{sessionId} \
  -H "Authorization: Bearer $TOKEN" \
  -H "X-User-ID: john.doe"
```

## Security

### Authentication & Authorization

- **API Auth:** Bearer token with `crypto/subtle.ConstantTimeCompare`
- **User Ownership:** Mandatory `X-User-ID` header; sessions bound to creator
- **Rate Limiting:** 10 req/s per IP with token bucket algorithm
- **CORS:** Configurable origin allowlist (no wildcard)
- **TLS:** Optional HTTPS with minimum TLS 1.2

### Credential Protection

- API keys encrypted at rest with AES-256-GCM
- 32-byte hex encryption key (generated via `openssl rand -hex 32`)
- Credentials decrypted only in server memory for PTY env vars
- ServiceNow stores keys in `password2` field type

### Input Validation

- User IDs: regex `^[a-zA-Z0-9_-]+$` (no path traversal)
- Workspace paths: `filepath.Abs` + prefix check under base path
- PTY commands: control char sanitization (allows only `\n`, `\r`, `\t`)
- Command size: max 16,384 bytes
- Command rate: 100ms minimum interval per session

## Development

```bash
# Build
make build

# Run tests
make test

# Run with race detector
make test-race

# Coverage report
make test-coverage

# Run benchmarks
make bench

# Format code
make fmt

# Lint
make lint

# Full integration test suite
./scripts/run-tests.sh
```

## Documentation

| Document | Description |
|----------|-------------|
| `docs/LOW_LEVEL_DESIGN.md` | Detailed LLD with component design, data flows, DB schema |
| `mid-proxy/docs/ARCHITECTURE.md` | MID Proxy architecture and comparison |
| `mid-proxy/docs/SETUP_GUIDE.md` | MID Proxy deployment instructions |
| `mid-proxy/servicenow/fix-scripts/README.md` | Fix script execution order for table/ACL setup |
| `DEPLOYMENT.md` | Production deployment guide |
| `TESTING_GUIDE.md` | Test strategy and execution |
| `kubernetes/KUBERNETES_DEPLOYMENT.md` | K8s deployment instructions |
| `servicenow/INSTALLATION_GUIDE.md` | ServiceNow component setup |

## Troubleshooting

### Service won't start

- **"API_AUTH_TOKEN must be configured":** Set `API_AUTH_TOKEN` in `.env` (required in release mode)
- **Log file permission denied:** Change `LOG_FILE` to a writable path (e.g., `./service.log`)
- **Go version mismatch:** Ensure Go 1.24+ (check `go.mod`)

### Session creation fails

- **"executable file not found":** Install Claude CLI (`npm install -g @anthropic-ai/claude-code`)
- **"invalid user ID":** User ID must match `^[a-zA-Z0-9_-]+$`
- **"max sessions exceeded":** Increase `MAX_SESSIONS_PER_USER` or terminate existing sessions

### No output in terminal

- Check ECC Poller is running: `docker logs ecc-poller`
- Verify AMB notifications are enabled in ServiceNow
- Check browser console for widget errors
- Try direct API call: `curl .../api/session/{id}/output`

### Database connection issues

- PostgreSQL is optional; service falls back to in-memory if `DB_HOST` is empty
- Check connectivity: `docker exec claude-postgres pg_isready`
- Verify credentials match between `.env` and `docker-compose.yml`

## Roadmap

- [ ] WebSocket endpoint for real-time output streaming
- [ ] Multi-MID Server load balancing
- [ ] Session sharing and collaboration
- [ ] Enhanced audit dashboard in ServiceNow
- [ ] Integration with ServiceNow ITSM workflows
- [ ] Custom Claude prompts/templates

## License

MIT License with Additional Disclaimer - see LICENSE file for details.

**USE AT YOUR OWN RISK.** The authors assume no responsibility for any damage, data loss, security incidents, or API costs resulting from the use of this software.
