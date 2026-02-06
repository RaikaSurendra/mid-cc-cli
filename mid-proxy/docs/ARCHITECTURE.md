# MID Server Proxy - Architecture

## Data Flow

### Original Setup (ECC Poller)

```
User types command in xterm.js widget
  |
  v
Widget JS calls ServiceNow Scripted REST API
  |
  v
Business Rule writes to ECC Queue (topic: ClaudeTerminalCommand, state: ready)
  |
  v
[~5 seconds] ECC Poller (Go binary) polls ServiceNow REST API
  |
  v
ECC Poller parses payload, makes HTTP call to localhost:3000
  |
  v
Claude Terminal Service processes request (PTY write/read)
  |
  v
ECC Poller writes response to ECC Queue (topic: ClaudeTerminalResponse)
  |
  v
Widget polls for output (or AMB notification triggers fetch)
  |
  v
User sees output in xterm.js

Total roundtrip: ~5-10 seconds per command
  - Widget -> ServiceNow: ~100ms
  - ECC Queue wait: ~5s (polling interval)
  - Poller -> HTTP Service: ~50ms
  - HTTP Service -> Poller: ~50ms
  - ECC Response: ~100ms
  - Widget poll/AMB: ~100-500ms
```

### MID Proxy Setup

```
User types command in xterm.js widget
  |
  v
Widget JS calls ServiceNow Scripted REST API (/api/x_claude/terminal_mid/*)
  |
  v
ClaudeTerminalAPI Script Include creates JavascriptProbe
  |
  v
Probe writes to ECC Queue (topic: JavascriptProbe, state: ready)
  |
  v
[~1-3 seconds] MID Server native polling picks up probe
  |
  v
MID Server executes ClaudeTerminalProbe (MID Server Script Include)
  |
  v
Probe makes HTTP call to claude-terminal-service:3000 (Docker network)
  |
  v
Claude Terminal Service processes request (PTY write/read)
  |
  v
Probe result written to ECC Queue output
  |
  v
ClaudeTerminalAPI.getProbeResult() polls for response (500ms intervals)
  |
  v
Scripted REST API returns result to widget
  |
  v
User sees output in xterm.js

Total roundtrip: ~3-7 seconds per command
  - Widget -> REST API: ~100ms
  - Probe -> ECC Queue: ~100ms
  - MID Server pickup: ~1-3s (faster native polling)
  - Probe execution: ~100-300ms
  - ECC Response write: ~100ms
  - getProbeResult poll: ~500-1500ms
  - REST API -> Widget: ~100ms
```

## Component Mapping

```
ORIGINAL                          MID PROXY
========                          =========

docker-compose.yml                mid-proxy/docker-compose.yml
  4 services                        3 services
  |                                 |
  +-- postgres                      +-- postgres
  +-- claude-terminal-service       +-- claude-terminal-service (same binary)
  +-- ecc-poller (Go binary)        +-- servicenow-mid-server
  +-- servicenow-mid-server               |
                                           +-- ClaudeTerminalProbe.js
                                               (replaces ecc-poller)

ServiceNow Instance               ServiceNow Instance
  |                                 |
  +-- Scripted REST API             +-- Scripted REST API (MID Proxy)
  |   (direct ECC Queue write)      |   (JavascriptProbe trigger)
  |                                 |
  +-- Business Rules                +-- ClaudeTerminalAPI (Script Include)
  |   (AMB notifications)           |   (probe execution + result polling)
  |                                 |
  +-- Widget                        +-- Widget (MID Proxy version)
      (polls output via ECC)            (polls output via Scripted REST)
```

## What Stays the Same

The Go HTTP service (`claude-terminal-service`) is **identical** in both setups. It receives the same HTTP requests regardless of whether they come from the ECC Poller or the MID Server probe.

```
HTTP Service API (unchanged):
  POST   /api/session/create
  POST   /api/session/:id/command
  GET    /api/session/:id/output
  GET    /api/session/:id/status
  POST   /api/session/:id/resize
  DELETE /api/session/:id
  GET    /api/sessions
  GET    /health
```

## What Changes

| Layer | Original | MID Proxy |
|-------|----------|-----------|
| ECC Processing | Custom Go poller binary | MID Server native + JS probe |
| ServiceNow API | Direct ECC Queue table writes | JavascriptProbe framework |
| Widget API calls | Direct to /api/x_claude/terminal/* | Via /api/x_claude/terminal_mid/* |
| Credential scope | HTTP service needs SN credentials | Only MID Server needs them |
| Output delivery | Widget polls ECC Queue + AMB | Widget polls Scripted REST API |

## Security Comparison

| Aspect | Original | MID Proxy |
|--------|----------|-----------|
| SN credentials in HTTP service | Yes (for config validation) | No (only MID Server) |
| SN credentials in ECC Poller | Yes (for REST API calls) | N/A (no poller) |
| SN credentials in MID Server | Yes | Yes |
| Network exposure | ECC Poller makes outbound HTTPS | MID Server makes outbound HTTPS |
| Internal HTTP | Poller -> Service (localhost) | MID -> Service (Docker network) |
| Auth token | Bearer token on HTTP API | Bearer token on HTTP API |
| User ownership | X-User-ID header | X-User-ID header (via probe) |

**MID Proxy is more secure** because ServiceNow credentials exist in fewer places (only the MID Server, which is designed for this purpose).

## Performance Expectations

| Metric | Original | MID Proxy | Notes |
|--------|----------|-----------|-------|
| ECC pickup latency | ~5s (configurable) | ~1-3s (native) | MID is faster |
| HTTP call latency | ~50ms (localhost) | ~50-100ms (Docker net) | Similar |
| E2E command roundtrip | ~5-10s | ~3-7s | MID wins slightly |
| Output polling | Widget polls ECC (variable) | Widget polls REST (adaptive) | Similar |
| Throughput | 5 concurrent workers | MID thread pool (configurable) | MID may handle more |
| Memory overhead | ~20MB (Go binary) | ~1-4GB (MID Server JVM) | MID heavier |
| CPU overhead | Minimal | Moderate (JVM) | MID heavier |

**Trade-off:** MID Proxy is faster for E2E latency but uses significantly more memory due to the JVM-based MID Server. For the Claude Terminal use case, the latency improvement matters more since users are waiting for interactive responses.
