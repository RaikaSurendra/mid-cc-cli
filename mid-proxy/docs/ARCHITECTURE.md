# MID Server Proxy Architecture

## Overview

The MID Proxy is an **alternative** to the ECC Queue Poller for bridging the Claude Terminal Service to ServiceNow. Instead of a custom Go binary that polls every 5 seconds, it uses the MID Server's native **JavascriptProbe** framework.

## Data Flow Comparison

### Original: ECC Queue Poller (cmd/ecc-poller/)

```
Browser → ServiceNow Widget → Scripted REST API
    → INSERT into ecc_queue (topic=ClaudeTerminalCommand)
        → [5s polling delay]
        → Go ECC Poller (custom binary)
            → HTTP POST → claude-terminal-service:3000
            ← HTTP response
        ← UPDATE ecc_queue (output + state=processed)
    ← Poll output from ecc_queue
← Display in terminal widget
```

**Latency:** ~5–10 seconds per command roundtrip
**Services:** 4 (postgres, HTTP service, Go poller, MID server)

### Alternative: MID Server Proxy (mid-proxy/)

```
Browser → ServiceNow Widget → Scripted REST API
    → ClaudeTerminalAPI.js (Script Include)
        → JavascriptProbe → INSERT into ecc_queue
            → [~2s MID Server native pickup]
            → ClaudeTerminalProbe.js (runs ON MID Server)
                → HTTP POST → claude-terminal-service:3000
                ← HTTP response
            ← Probe output → ecc_queue output queue
        ← Poll output from ecc_queue
    ← Return to REST API
← Display in terminal widget
```

**Latency:** ~3–7 seconds per command roundtrip
**Services:** 3 (postgres, HTTP service, MID server)

## Component Map

| Component | Type | Runs On | Purpose |
|-----------|------|---------|---------|
| `ClaudeTerminalAPI.js` | Script Include | SN Instance | Creates JavascriptProbes, polls for responses |
| `ClaudeTerminalProbe.js` | Script Include | MID Server | Handles probe execution, calls HTTP service |
| `ClaudeTerminalMIDProxyAPI.js` | Scripted REST | SN Instance | REST endpoints that route through MID |
| `claude_terminal_mid` widget | Widget | SN Portal | UI for MID proxy variant |

## Why Use MID Proxy Over ECC Poller?

| Factor | ECC Poller | MID Proxy |
|--------|-----------|-----------|
| Latency | ~5–10s (configurable poll interval) | ~3–7s (MID native pickup) |
| Custom code | Go binary to maintain | JavaScript only (SN-native) |
| Deployment | Extra binary on MID server host | Uses existing MID Server |
| Monitoring | Custom logging | SN MID Server dashboard |
| Scaling | One poller per MID host | MID Server handles concurrency |
| Debugging | SSH into MID host + check logs | SN System Logs + ECC Queue |

## When to Use Which

- **ECC Poller**: When you need full control, custom retry logic, or want to avoid MID Server JavaScript execution overhead
- **MID Proxy**: When you want simpler deployment, lower latency, and prefer managing everything through ServiceNow

## Shared Components

Both approaches share:
- **Tables**: `x_claude_terminal_session`, `x_claude_credentials` (in root `servicenow/tables/`)
- **HTTP Service**: Same Go binary (`cmd/server/`) — no changes needed
- **Business Rules**: Same AMB notification rule (in root `servicenow/business-rules/`)

## System Properties

Configure these in ServiceNow (System Properties):

| Property | Example | Description |
|----------|---------|-------------|
| `x_claude.terminal.mid_server` | `mid-docker-proxy` | MID Server name to use for probes |
| `x_claude.terminal.service_url` | `http://claude-terminal-service:3000` | URL the MID Server uses to reach the HTTP service |
| `x_claude.terminal.auth_token` | `dev-token-change-me` | Bearer token for HTTP service auth |
