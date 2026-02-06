# Low-Level Design (LLD) - Claude Terminal MID Service

## 1. Overview

**Project:** Claude Terminal MID Service for ServiceNow
**Language:** Go 1.24
**Architecture:** Two Go binaries (HTTP Service + ECC Poller) bridging Claude Code CLI to ServiceNow via MID Servers
**Database:** PostgreSQL 15 (session persistence)
**Container Runtime:** Docker + Docker Compose

---

## 2. System Architecture

```
+-------------------------------------------------------------------------+
|                        ServiceNow Instance                              |
|                                                                         |
|  Service Portal Widget (xterm.js)                                       |
|         |                                                               |
|         v                                                               |
|  Scripted REST API  -->  ECC Queue (topic: ClaudeTerminalCommand)       |
|         ^                        |                                      |
|         |                        | state=ready                          |
|  AMB Channel (real-time)         |                                      |
|  (claude.terminal.output.{id})   |                                      |
+----------------------------------+--------------------------------------+
                                   |
                                   | HTTPS (Basic Auth)
                                   v
+----------------------------------+--------------------------------------+
|                          MID Server Host                                |
|                                                                         |
|  +------------------+     HTTP (Bearer)    +------------------------+   |
|  |  ECC Poller      | ------------------> |  HTTP Service           |   |
|  |  (polls every 5s)|                      |  (Gin, port 3000)      |   |
|  +------------------+                      |                        |   |
|                                            |  Session Manager       |   |
|                                            |      |                 |   |
|                                            |      v                 |   |
|                                            |  PTY (creack/pty)      |   |
|                                            |      |                 |   |
|                                            |      v                 |   |
|                                            |  Claude Code CLI       |   |
|                                            +----------+-------------+   |
|                                                       |                 |
|                                              Async DB writes            |
|                                                       |                 |
|                                                       v                 |
|                                            +------------------------+   |
|                                            |  PostgreSQL 15         |   |
|                                            |  - sessions            |   |
|                                            |  - session_output      |   |
|                                            +------------------------+   |
+-------------------------------------------------------------------------+
```

---

## 3. Directory Structure

```
mid-llm-cli/
├── cmd/
│   ├── server/main.go              # HTTP service entry point
│   └── ecc-poller/main.go          # ECC Queue poller entry point
├── internal/
│   ├── config/config.go            # Configuration loader (env vars)
│   ├── server/server.go            # REST API handlers + auth
│   ├── session/session.go          # PTY session manager (core logic)
│   ├── store/postgres.go           # PostgreSQL persistence layer
│   ├── servicenow/client.go        # ServiceNow + Node HTTP clients
│   ├── crypto/crypto.go            # AES-256-GCM encryption
│   ├── logging/logging.go          # Centralized structured logging
│   └── middleware/ratelimit.go     # Per-IP rate limiting
├── servicenow/
│   ├── tables/                     # ServiceNow table definitions (JSON)
│   ├── rest-api/                   # Scripted REST API (XML)
│   ├── business-rules/             # Business rules (AMB notifications)
│   └── widgets/                    # Service Portal widgets (xterm.js)
├── kubernetes/                     # K8s manifests + MID server configs
├── deployment/systemd/             # systemd unit files
├── scripts/                        # Build, test, verification scripts
├── docs/                           # Documentation
├── Dockerfile                      # Multi-stage Docker build
├── docker-compose.yml              # 4-service orchestration
├── Makefile                        # Build automation
├── .env.example                    # Configuration template
└── go.mod / go.sum                 # Go module dependencies
```

---

## 4. Component Design

### 4.1 HTTP Service (`cmd/server/main.go`)

**Responsibility:** REST API server managing Claude Code CLI sessions via PTY.

**Bootstrap Sequence:**

```
main()
  |-- Load .env (godotenv)
  |-- config.Load()
  |-- logging.Setup()
  |-- Validate API_AUTH_TOKEN (fatal in release mode if empty)
  |-- store.NewPostgresStore() (optional, fallback to in-memory)
  |-- session.NewManager(config, pgStore)
  |-- manager.RecoverSessions() (mark stale as terminated)
  |-- manager.StartTimeoutChecker() (background goroutine, 1min interval)
  |-- setupRouter()
  |     |-- gin.Recovery()
  |     |-- loggingMiddleware()
  |     |-- corsMiddleware()
  |     |-- RateLimiter.Middleware() (10 req/s, burst 20)
  |     |-- /health (public)
  |     |-- /api/* (authMiddleware -> handlers)
  |-- ListenAndServe / ListenAndServeTLS
  |-- Graceful shutdown (SIGINT/SIGTERM, 10s timeout)
```

**Middleware Chain:**

| Order | Middleware | Purpose |
|-------|-----------|---------|
| 1 | `gin.Recovery()` | Panic recovery |
| 2 | `loggingMiddleware()` | Structured request logging |
| 3 | `corsMiddleware()` | CORS origin allowlist |
| 4 | `RateLimiter.Middleware()` | 10 req/s per IP, burst 20 |
| 5 | `authMiddleware()` | Bearer token (on `/api/*` only) |

---

### 4.2 ECC Poller (`cmd/ecc-poller/main.go`)

**Responsibility:** Polls ServiceNow ECC Queue and forwards commands to the HTTP service.

**Polling Loop:**

```
Start(ctx)
  |-- Loop every 5 seconds:
        |-- poll(ctx)
              |-- GET /api/now/table/ecc_queue
              |     query: topic=ClaudeTerminalCommand^state=ready (limit 10)
              |-- For each item (worker pool, max 5 concurrent):
                    |-- PATCH state -> "processing"
                    |-- Parse payload JSON
                    |-- Route by action:
                    |     create_session  -> POST /api/session/create
                    |     send_command    -> POST /api/session/{id}/command
                    |     get_output      -> GET  /api/session/{id}/output
                    |     get_status      -> GET  /api/session/{id}/status
                    |     terminate       -> DELETE /api/session/{id}
                    |     resize_terminal -> POST /api/session/{id}/resize
                    |-- On success: PATCH state -> "processed"
                    |-- On failure: PATCH state -> "error"
                    |-- POST response to ECC output queue
```

**Worker Pool:**

```go
semaphore := make(chan struct{}, 5)  // max 5 concurrent
var wg sync.WaitGroup

for _, item := range items {
    wg.Add(1)
    semaphore <- struct{}{}
    go func(item) {
        defer wg.Done()
        defer func() { <-semaphore }()
        ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
        defer cancel()
        processItem(ctx, item)
    }(item)
}
wg.Wait()
```

---

### 4.3 Session Manager (`internal/session/session.go`)

**Responsibility:** PTY lifecycle, command routing, output buffering, credential encryption.

**Data Structures:**

```go
type Manager struct {
    sessions map[string]*Session   // in-memory session store
    config   *config.Config
    store    *store.PostgresStore   // optional DB persistence
    mu       sync.RWMutex
}

type Session struct {
    SessionID            string
    UserID               string
    WorkspacePath        string
    EncryptedCredentials EncryptedCredentials
    Status               string              // initializing | active | terminated
    PTY                  *os.File
    Cmd                  *exec.Cmd
    OutputBuffer         []OutputChunk
    LastActivity         time.Time
    Created              time.Time
    lastCommandTime      time.Time           // rate limiting
    mu                   sync.RWMutex
    done                 chan struct{}        // goroutine shutdown signal
    encryptionKey        string
    outputBufferSize     int
    dbStore              *store.PostgresStore
}

type OutputChunk struct {
    Timestamp string `json:"timestamp"`
    Data      string `json:"data"`
}

type Credentials struct {
    AnthropicAPIKey string
    GitHubToken     string
}

type EncryptedCredentials struct {
    AnthropicAPIKey string  // AES-256-GCM encrypted, hex-encoded
    GitHubToken     string  // AES-256-GCM encrypted, hex-encoded
}
```

**Session Lifecycle:**

```
CreateSession(userID, credentials, workspaceType)
  |-- Validate userID (regex: ^[a-zA-Z0-9_-]+$)
  |-- Check max sessions per user (default: 3)
  |-- Generate UUID (session ID)
  |-- Encrypt credentials (AES-256-GCM)
  |-- Create workspace: {basePath}/{userID}/{sessionID}/
  |-- Validate path (must resolve under basePath)
  |-- session.Initialize(credentials)
  |     |-- Decrypt API key
  |     |-- Build command: claude --no-browser
  |     |-- Set env: ANTHROPIC_API_KEY, HOME, PATH
  |     |-- pty.Start(cmd)  -> allocate PTY
  |     |-- go readOutput() -> start output reader goroutine
  |     |-- Status = "active"
  |-- Async: saveSessionToDB()
  |-- Return session
```

**Command Flow:**

```
SendCommand(command)
  |-- Check status == "active"
  |-- Rate limit (100ms between commands)
  |-- Sanitize command:
  |     |-- Remove control chars (0x00-0x1F except \n \r \t)
  |     |-- Truncate to 16384 bytes
  |-- Write to PTY
  |-- Update LastActivity
  |-- Async: UpdateLastActivity in DB
```

**Output Flow:**

```
readOutput() [goroutine]
  |-- Loop:
        |-- Read from PTY (4096 byte buffer)
        |-- On EOF or done channel -> return
        |-- handleOutput(data)
              |-- Lock session mutex
              |-- Append OutputChunk with timestamp
              |-- Trim buffer to outputBufferSize (FIFO)
              |-- Update LastActivity
              |-- Async: SaveOutputChunk to DB
```

**Timeout Checker:**

```
StartTimeoutChecker(ctx) [goroutine, 1min interval]
  |-- For each session:
        |-- If time.Since(LastActivity) > TimeoutMinutes:
              |-- session.Cleanup()
              |-- Remove from map
              |-- Update DB status = "terminated"
```

---

### 4.4 PostgreSQL Store (`internal/store/postgres.go`)

**Responsibility:** Persistent session storage with automatic schema migration.

**Connection Pool:**

```go
poolConfig.MaxConns = 10
poolConfig.MinConns = 2
poolConfig.MaxConnLifetime = 30 * time.Minute
poolConfig.MaxConnIdleTime = 5 * time.Minute
```

**Schema (auto-created on startup):**

```sql
CREATE TABLE IF NOT EXISTS sessions (
    session_id          VARCHAR(36) PRIMARY KEY,
    user_id             VARCHAR(255) NOT NULL,
    workspace_path      TEXT NOT NULL,
    status              VARCHAR(50) NOT NULL DEFAULT 'initializing',
    encrypted_credentials JSONB,
    last_activity       TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    created_at          TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_sessions_user_id ON sessions(user_id);
CREATE INDEX IF NOT EXISTS idx_sessions_status  ON sessions(status);

CREATE TABLE IF NOT EXISTS session_output (
    id          BIGSERIAL PRIMARY KEY,
    session_id  VARCHAR(36) NOT NULL
                REFERENCES sessions(session_id) ON DELETE CASCADE,
    timestamp   TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    data        TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_session_output_session_id
    ON session_output(session_id);
```

**Operations:**

| Method | SQL | Notes |
|--------|-----|-------|
| `SaveSession` | INSERT ... ON CONFLICT UPDATE | Upsert pattern |
| `GetSession` | SELECT WHERE session_id=$1 | Single row |
| `GetSessionsForUser` | SELECT WHERE user_id=$1 | Multi-row |
| `UpdateSessionStatus` | UPDATE SET status=$2 | + updated_at |
| `UpdateLastActivity` | UPDATE SET last_activity=$2 | + updated_at |
| `SaveOutputChunk` | INSERT INTO session_output | Append-only |
| `GetOutputChunks` | SELECT ORDER BY id DESC LIMIT | Paginated |
| `DeleteSession` | DELETE WHERE session_id=$1 | Cascades output |
| `GetActiveSessions` | SELECT WHERE status IN (...) | Recovery |
| `MarkStaleSessionsTerminated` | UPDATE SET status='terminated' | Startup cleanup |

**Write Strategy:** All DB writes from the session manager are async (fire-and-forget goroutines with context timeouts). DB failures never block HTTP responses.

---

### 4.5 ServiceNow Client (`internal/servicenow/client.go`)

**Two HTTP Clients:**

**1. ServiceNow API Client (`Client`)**

```go
type Client struct {
    config     *config.Config
    httpClient *http.Client       // 30s timeout
    baseURL    string             // https://{instance}
    auth       string             // Base64(user:password)
}
```

| Method | HTTP | Endpoint |
|--------|------|----------|
| `GetECCQueueItems` | GET | `/api/now/table/ecc_queue?sysparm_query=topic=ClaudeTerminalCommand^state=ready&sysparm_limit=10` |
| `UpdateECCQueueItem` | PATCH | `/api/now/table/ecc_queue/{sys_id}` |
| `CreateECCQueueResponse` | POST | `/api/now/table/ecc_queue` |

**2. Node Service Client (`NodeServiceClient`)**

```go
type NodeServiceClient struct {
    config     *config.Config
    httpClient *http.Client       // 30s timeout
    baseURL    string             // http(s)://{host}:{port}
}
```

Forwards ECC commands to the local HTTP service. Adds `Authorization: Bearer {token}` if configured.

---

### 4.6 Crypto Module (`internal/crypto/crypto.go`)

**Algorithm:** AES-256-GCM (Authenticated Encryption with Associated Data)

```
Encrypt(plaintext, hexKey):
  key = hex.Decode(hexKey)         # 32 bytes
  block = aes.NewCipher(key)
  aead = cipher.NewGCM(block)
  nonce = crypto/rand(aead.NonceSize())
  ciphertext = aead.Seal(nonce, nonce, plaintext, nil)
  return hex.Encode(ciphertext)

Decrypt(hexCiphertext, hexKey):
  data = hex.Decode(hexCiphertext)
  key = hex.Decode(hexKey)
  block = aes.NewCipher(key)
  aead = cipher.NewGCM(block)
  nonce = data[:aead.NonceSize()]
  plaintext = aead.Open(nil, nonce, data[NonceSize:], nil)
  return plaintext
```

**Storage Format:** `hex(nonce || ciphertext || GCM_tag)`

---

### 4.7 Rate Limiter (`internal/middleware/ratelimit.go`)

**Algorithm:** Token bucket (via `golang.org/x/time/rate`)

```go
type RateLimiter struct {
    limiters map[string]*limiterEntry  // key: client IP
    mu       sync.Mutex
    rate     float64                    // 10 tokens/sec
    burst    int                        // 20 tokens
}
```

**Cleanup:** Background goroutine runs every 60 seconds, evicts IPs idle for 10+ minutes.

**Response on limit exceeded:**

```json
HTTP 429 Too Many Requests
{"error": "rate limit exceeded"}
```

---

## 5. API Reference

### 5.1 Endpoints

| Method | Path | Auth | User-ID | Description |
|--------|------|------|---------|-------------|
| `GET` | `/health` | No | No | Health check + diagnostics |
| `POST` | `/api/session/create` | Bearer | No | Create Claude session |
| `POST` | `/api/session/:id/command` | Bearer | Yes | Send command to PTY |
| `GET` | `/api/session/:id/output` | Bearer | Yes | Get buffered output |
| `GET` | `/api/session/:id/status` | Bearer | Yes | Get session status |
| `POST` | `/api/session/:id/resize` | Bearer | Yes | Resize terminal |
| `DELETE` | `/api/session/:id` | Bearer | Yes | Terminate session |
| `GET` | `/api/sessions` | Bearer | Yes | List user's sessions |

### 5.2 Request/Response Models

**POST /api/session/create**

```json
// Request
{
  "userId": "john.doe",
  "credentials": {
    "anthropicApiKey": "sk-ant-...",
    "githubToken": "ghp_..."
  },
  "workspaceType": "isolated"
}

// Response 200
{
  "sessionId": "a1b2c3d4-...",
  "status": "active",
  "workspacePath": "/tmp/claude-sessions/john.doe/a1b2c3d4-..."
}
```

**POST /api/session/:id/command**

```json
// Request
{ "command": "help\n" }

// Response 200
{ "success": true }
```

**GET /api/session/:id/output?clear=true**

```json
// Response 200
{
  "sessionId": "a1b2c3d4-...",
  "output": [
    { "timestamp": "2026-02-06T10:30:00Z", "data": "Welcome to Claude..." }
  ],
  "status": "active"
}
```

**GET /api/session/:id/status**

```json
// Response 200
{
  "sessionId": "a1b2c3d4-...",
  "userId": "john.doe",
  "status": "active",
  "workspacePath": "/tmp/claude-sessions/john.doe/a1b2c3d4-...",
  "created": "2026-02-06T10:00:00Z",
  "lastActivity": "2026-02-06T10:30:00Z"
}
```

**POST /api/session/:id/resize**

```json
// Request
{ "cols": 120, "rows": 40 }

// Response 200
{ "success": true }
```

**GET /health**

```json
// Response 200
{
  "status": "healthy",
  "timestamp": "2026-02-06T10:30:00Z",
  "active_sessions": 3,
  "memory_alloc_mb": 64
}
```

---

## 6. Database Design

### 6.1 Entity Relationship

```
+-------------------+          +---------------------+
|     sessions      |          |   session_output     |
+-------------------+          +---------------------+
| session_id (PK)   |<----+   | id (PK, BIGSERIAL)  |
| user_id            |     +---| session_id (FK)      |
| workspace_path     |         | timestamp            |
| status             |         | data                 |
| encrypted_creds    |         +---------------------+
| last_activity      |         ON DELETE CASCADE
| created_at         |
| updated_at         |
+-------------------+
```

### 6.2 ServiceNow Tables

```
+-------------------------------+         +----------------------------+
| x_claude_terminal_session     |         | x_claude_credentials       |
+-------------------------------+         +----------------------------+
| session_id (PK, display)      |         | user (PK, ref: sys_user)   |
| user (ref: sys_user)          |         | anthropic_api_key (pwd2)   |
| status (choice)               |         | github_token (pwd2)        |
| command_queue (JSON, 4000)    |         | last_used (datetime)       |
| output_buffer (JSON, 65000)   |         | last_validated (datetime)  |
| workspace_path (string, 255)  |         | validation_status (choice) |
| workspace_type (choice)       |         +----------------------------+
| last_activity (datetime)      |
| mid_server (string, 100)      |
| error_message (string, 1000)  |
+-------------------------------+
```

---

## 7. Security Design

### 7.1 Defense Layers

| Layer | Mechanism | Implementation |
|-------|-----------|----------------|
| Transport | TLS 1.2+ | Optional `TLS_CERT_PATH` / `TLS_KEY_PATH` |
| Authentication | Bearer token | `crypto/subtle.ConstantTimeCompare` |
| Authorization | User ownership | Mandatory `X-User-ID` header on session endpoints |
| Rate limiting | Token bucket | 10 req/s per IP, burst 20 |
| Input validation | Regex + sanitization | UserID regex, control char filter, 16KB limit |
| Path traversal | Prefix check | `filepath.Abs` + `strings.HasPrefix(basePath)` |
| Encryption at rest | AES-256-GCM | Credentials encrypted before storage |
| Container isolation | Non-root user | `appuser:appgroup` in Docker |

### 7.2 Credential Flow

```
User submits API key via ServiceNow widget
  |-- ServiceNow stores in x_claude_credentials (password2 field)
  |-- ECC Queue payload includes API key (HTTPS transport)
  |
  v
ECC Poller receives payload
  |-- Forwards to HTTP Service (internal network)
  |
  v
HTTP Service (CreateSession)
  |-- crypto.Encrypt(apiKey, ENCRYPTION_KEY)
  |-- Store EncryptedCredentials in Session struct
  |-- Async: Save encrypted JSON to PostgreSQL (JSONB)
  |
  v
Session.Initialize()
  |-- crypto.Decrypt(encryptedKey, ENCRYPTION_KEY)
  |-- Set ANTHROPIC_API_KEY env var for Claude CLI process
  |-- Plaintext exists only in process memory
```

### 7.3 PTY Input Sanitization

```go
func sanitizeCommand(command string) string {
    // Truncate to 16384 bytes
    // Remove bytes 0x00-0x1F EXCEPT:
    //   0x0A (\n) - newline
    //   0x0D (\r) - carriage return
    //   0x09 (\t) - tab
    // Keep all printable ASCII and UTF-8
}
```

---

## 8. Concurrency Model

### 8.1 Goroutine Map

| Goroutine | Lifetime | Purpose | Shutdown |
|-----------|----------|---------|----------|
| HTTP Server | App lifetime | Serve requests | `srv.Shutdown(ctx)` |
| Timeout Checker | App lifetime | Expire idle sessions | Context cancellation |
| ECC Poll Loop | App lifetime | Poll ServiceNow queue | Context cancellation |
| Worker (per item) | 30s max | Process single ECC item | Context timeout |
| Output Reader (per session) | Session lifetime | Read PTY output | `done` channel + PTY close |
| DB Writer (per operation) | 3-5s max | Async persistence | Context timeout |

### 8.2 Locking Strategy

| Resource | Lock Type | Granularity |
|----------|-----------|-------------|
| Session map (`Manager.sessions`) | `sync.RWMutex` | Per manager operation |
| Individual session | `sync.RWMutex` | Per session field access |
| Rate limiter map | `sync.Mutex` | Per IP lookup |

### 8.3 Graceful Shutdown

```
SIGINT/SIGTERM received
  |-- Cancel root context
  |-- HTTP server graceful shutdown (10s timeout)
  |-- manager.CleanupAll()
  |     |-- For each session:
  |           |-- Kill Claude CLI process
  |           |-- Close PTY file descriptor
  |           |-- Remove workspace directory
  |           |-- Update DB status = "terminated"
  |-- pgStore.Close() (drain connection pool)
  |-- Exit
```

---

## 9. Data Flow Diagrams

### 9.1 Session Creation

```
ServiceNow Widget                ECC Queue       ECC Poller        HTTP Service        PTY/CLI
      |                              |               |                  |                 |
      |-- Create Session Request --> |               |                  |                 |
      |                              |-- ready ----> |                  |                 |
      |                              |               |-- POST /create ->|                 |
      |                              |               |                  |-- Validate ---  |
      |                              |               |                  |-- Encrypt creds |
      |                              |               |                  |-- mkdir workspace|
      |                              |               |                  |-- pty.Start() ->|
      |                              |               |                  |                 |-- claude --no-browser
      |                              |               |                  |-- go readOutput()|
      |                              |               |                  |-- Save to DB    |
      |                              |               |<- 200 {session} -|                 |
      |                              |<- processed --|                  |                 |
      |<-- AMB: session created -----|               |                  |                 |
```

### 9.2 Command Execution

```
ServiceNow Widget                ECC Queue       ECC Poller        HTTP Service      Session/PTY
      |                              |               |                  |                 |
      |-- Send Command ------------> |               |                  |                 |
      |                              |-- ready ----> |                  |                 |
      |                              |               |-- POST /command->|                 |
      |                              |               |                  |-- Auth check    |
      |                              |               |                  |-- User ownership|
      |                              |               |                  |-- Rate limit    |
      |                              |               |                  |-- Sanitize cmd  |
      |                              |               |                  |-- Write to PTY->|
      |                              |               |                  |                 |-- Execute
      |                              |               |                  |                 |-- Output
      |                              |               |                  |<- readOutput() -|
      |                              |               |                  |-- Buffer output |
      |                              |               |                  |-- Async DB save |
      |                              |               |<- 200 {success} -|                 |
      |                              |<- processed --|                  |                 |
      |<-- AMB: output available ----|               |                  |                 |
      |                              |               |                  |                 |
      |-- Get Output --------------> |               |                  |                 |
      |                              |-- ready ----> |                  |                 |
      |                              |               |-- GET /output -->|                 |
      |                              |               |                  |-- Return buffer |
      |                              |               |<- 200 {output} --|                 |
      |<-- Display in xterm.js ------|<- processed --|                  |                 |
```

---

## 10. Deployment Architecture

### 10.1 Docker Compose (Development/Testing)

```
+-----------------------------------------------------------------+
|  mid-network (bridge)                                            |
|                                                                  |
|  +----------------+    +-------------------------+               |
|  | claude-postgres |    | claude-terminal-service |               |
|  | :5433 -> :5432  |<---| :3000                   |               |
|  | postgres:15     |    | golang:1.24-alpine      |               |
|  +----------------+    | + Claude Code CLI        |               |
|                         +------------+------------+               |
|                                      ^                            |
|  +----------------+                  |                            |
|  | ecc-poller     |----- HTTP -------+                            |
|  | ./ecc-poller   |                                              |
|  +----------------+                                              |
|                                                                  |
|  +-----------------------+                                       |
|  | servicenow-mid-server |                                       |
|  | zurich-patch4-hotfix3 |                                       |
|  | 0.5-2 CPU, 1-4GB RAM |                                       |
|  +-----------------------+                                       |
+-----------------------------------------------------------------+

Named Volumes:
  postgres-data     -> /var/lib/postgresql/data
  claude-sessions   -> /tmp/claude-sessions
  claude-logs       -> /var/log
  mid-server-work   -> /opt/snc_mid_server/agent/work
  mid-server-logs   -> /opt/snc_mid_server/agent/logs
```

### 10.2 Kubernetes (Production)

```
Namespace: claude-terminal
  |
  |-- Deployment: claude-terminal (replicas: 2)
  |     |-- Container: claude-terminal-service
  |     |-- Container: ecc-poller (sidecar)
  |     |-- PVC: claude-sessions (ReadWriteMany)
  |
  |-- Deployment: servicenow-mid (replicas: 1)
  |     |-- Container: mid-server
  |     |-- PVC: mid-server-work
  |
  |-- Service: claude-terminal-service (ClusterIP, port 3000)
  |
  |-- Secret: claude-terminal-secrets
  |     |-- SERVICENOW_API_PASSWORD
  |     |-- ENCRYPTION_KEY
  |     |-- API_AUTH_TOKEN
  |     |-- DB credentials
  |
  |-- ConfigMap: claude-terminal-config
        |-- SERVICENOW_INSTANCE
        |-- MID_SERVER_NAME
        |-- SESSION_TIMEOUT_MINUTES
        |-- etc.
```

---

## 11. Configuration Reference

| Variable | Default | Required | Description |
|----------|---------|----------|-------------|
| `SERVICENOW_INSTANCE` | - | Yes | ServiceNow instance hostname |
| `SERVICENOW_API_USER` | - | Yes | ServiceNow API username |
| `SERVICENOW_API_PASSWORD` | - | Yes | ServiceNow API password |
| `MID_SERVER_NAME` | - | No | MID server identifier |
| `NODE_SERVICE_HOST` | localhost | No | HTTP service bind address |
| `NODE_SERVICE_PORT` | 3000 | No | HTTP service port |
| `SESSION_TIMEOUT_MINUTES` | 30 | No | Idle session timeout |
| `MAX_SESSIONS_PER_USER` | 3 | No | Max concurrent sessions per user |
| `OUTPUT_BUFFER_SIZE` | 100 | No | Output chunks kept in memory |
| `WORKSPACE_BASE_PATH` | /tmp/claude-sessions | No | Session workspace root |
| `WORKSPACE_TYPE` | isolated | No | Workspace isolation mode |
| `LOG_LEVEL` | info | No | Log level (debug/info/warn/error) |
| `LOG_FILE` | stdout | No | Log file path |
| `ENCRYPTION_KEY` | - | Yes* | 32-byte hex key for AES-256-GCM |
| `API_AUTH_TOKEN` | - | Yes** | Bearer token for API auth |
| `CORS_ALLOWED_ORIGINS` | http://localhost | No | Comma-separated allowed origins |
| `TLS_CERT_PATH` | - | No | TLS certificate file path |
| `TLS_KEY_PATH` | - | No | TLS private key file path |
| `DB_HOST` | - | No | PostgreSQL host (empty = in-memory only) |
| `DB_PORT` | 5432 | No | PostgreSQL port |
| `DB_USER` | postgres | No | PostgreSQL username |
| `DB_PASSWORD` | - | No | PostgreSQL password |
| `DB_NAME` | claude_terminal | No | PostgreSQL database name |
| `DB_SSLMODE` | disable | No | PostgreSQL SSL mode |
| `GIN_MODE` | debug | No | Gin framework mode (debug/release) |

\* Required when credential encryption is used
\** Required in release mode (`GIN_MODE=release`)

---

## 12. External Dependencies

| Package | Version | Purpose |
|---------|---------|---------|
| `github.com/gin-gonic/gin` | v1.11.0 | HTTP web framework |
| `github.com/creack/pty` | v1.1.24 | PTY allocation for Claude CLI |
| `github.com/jackc/pgx/v5` | v5.8.0 | PostgreSQL driver + connection pool |
| `github.com/google/uuid` | v1.6.0 | Session ID generation |
| `github.com/joho/godotenv` | v1.5.1 | .env file loading |
| `github.com/sirupsen/logrus` | v1.9.4 | Structured JSON logging |
| `golang.org/x/time` | v0.14.0 | Rate limiting (token bucket) |
| `golang.org/x/crypto` | v0.47.0 | Cryptographic primitives (indirect) |

---

## 13. Error Handling Strategy

| Scenario | Behavior |
|----------|----------|
| PostgreSQL unavailable at startup | Warning logged, fallback to in-memory sessions |
| PostgreSQL write fails at runtime | Error logged, session continues (async write) |
| Claude CLI fails to start | HTTP 500 returned, session cleaned up |
| PTY read returns EOF | Output reader exits, session status unchanged |
| ECC item processing fails | Item state set to "error", poller continues |
| Auth token missing in release mode | `log.Fatal` - server refuses to start |
| Invalid user ID format | HTTP 400 "invalid user ID" |
| Session not found | HTTP 404 "session not found" |
| User doesn't own session | HTTP 403 "access denied" |
| Rate limit exceeded | HTTP 429 "rate limit exceeded" |
| Command too long (>16KB) | Silently truncated to 16384 bytes |
| Control chars in command | Silently stripped (except \n, \r, \t) |

---

## 14. Testing

**Unit Tests:** Located alongside source files (`*_test.go`)

| Package | Test File | Coverage Focus |
|---------|-----------|----------------|
| `config` | `config_test.go` | Env var loading, defaults, validation |
| `server` | `server_test.go` | Route handlers, auth, CORS, IDOR |
| `session` | `session_test.go` | Lifecycle, sanitization, encryption, path traversal |

**Integration Tests:** Run via `scripts/run-tests.sh`

| Test | Validates |
|------|-----------|
| Health endpoint | Service availability |
| Auth rejection | Missing/invalid bearer token |
| Session create | Full lifecycle with PTY spawn |
| Command send | PTY write + output capture |
| Output retrieval | Buffer read with clear flag |
| IDOR protection | X-User-ID enforcement |
| Session terminate | Cleanup + DB update |
| Rate limiting | 429 response on burst |
| CORS headers | Origin allowlist enforcement |
| Path traversal | Malicious userID rejection |
| Session listing | Per-user filtering |

**Run tests:**

```bash
make test           # Unit tests
make test-race      # With race detector
make test-coverage  # With HTML coverage report
./scripts/run-tests.sh  # Full integration suite
```

---

## 15. Known Limitations

| Limitation | Impact | Mitigation Path |
|------------|--------|-----------------|
| ECC Queue 5s polling interval | High latency for interactive terminal | Replace with WebSocket for real-time I/O |
| In-memory session state is primary | Sessions lost on pod restart | PostgreSQL stores metadata but not PTY state |
| Single ECC Poller instance | Single point of failure | Add leader election or multiple pollers |
| No horizontal scaling for PTY | Sessions pinned to single node | Sticky sessions or session migration |
| Output buffer capped in memory | Old output lost | PostgreSQL stores full history |
| No WebSocket support | Polling-only real-time updates | Add WebSocket endpoint for output streaming |
| Claude CLI must be installed | Container image dependency | Pre-baked in Dockerfile |
