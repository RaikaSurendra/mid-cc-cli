# MID Server Proxy Setup Guide

## Overview

This setup replaces the custom ECC Poller with the MID Server acting as a native proxy. The MID Server picks up ECC Queue items and forwards HTTP requests to the Claude Terminal Service.

```
Original:   ServiceNow -> ECC Queue -> ECC Poller (Go) -> HTTP Service
MID Proxy:  ServiceNow -> ECC Queue -> MID Server (native) -> HTTP Service
```

## Architecture Comparison

| Aspect | Original (ECC Poller) | MID Proxy |
|--------|----------------------|-----------|
| Services | 4 (postgres, http, poller, mid) | 3 (postgres, http, mid) |
| ECC polling | Custom Go binary (5s) | MID Server native (~2s) |
| ServiceNow credentials | Needed by both poller and MID | Only MID Server |
| Code to maintain | ~300 lines Go (ecc-poller) | ~200 lines JS (ServiceNow) |
| Monitoring | Custom logging | ServiceNow MID Server dashboard |
| Failover | Manual | MID Server cluster support |
| Latency (E2E) | ~5-7s per command roundtrip | ~3-5s per command roundtrip |

## Prerequisites

1. ServiceNow instance with admin access
2. MID Server validated and connected
3. Docker and Docker Compose installed
4. Original project built (`make build`)

## Step 1: Deploy Docker Services

```bash
# From project root
docker compose -f mid-proxy/docker-compose.yml up --build -d
```

This starts 3 services:
- `midproxy-postgres` (port 5434)
- `midproxy-terminal-service` (port 3001)
- `midproxy-mid-server` (MID Server)

## Step 2: ServiceNow Configuration

### 2.1 System Properties

Navigate to **System Properties > All Properties** and create:

| Property | Value |
|----------|-------|
| `x_claude.terminal.mid_server` | `k8s-mid-proxy-01` |
| `x_claude.terminal.service_url` | `http://claude-terminal-service:3000` |
| `x_claude.terminal.auth_token` | `mid-llm-cli-dev-token-2026` |

### 2.2 MID Server Script Include

1. Navigate to **MID Server > Script Includes**
2. Click **New**
3. Set:
   - **Name:** `ClaudeTerminalProbe`
   - **Active:** true
   - **Script:** Paste contents of `servicenow/script-includes/ClaudeTerminalProbe.js`
4. Save

### 2.3 Server-side Script Include

1. Navigate to **System Definition > Script Includes**
2. Click **New**
3. Set:
   - **Name:** `ClaudeTerminalAPI`
   - **Client callable:** false
   - **Active:** true
   - **Script:** Paste contents of `servicenow/script-includes/ClaudeTerminalAPI.js`
4. Save

### 2.4 Scripted REST API

1. Navigate to **System Web Services > Scripted REST APIs**
2. Click **New**
3. Set:
   - **Name:** Claude Terminal MID Proxy
   - **API ID:** `x_claude_terminal_mid`
4. Save, then create these Resources:

| Name | Method | Relative Path | Script Source |
|------|--------|---------------|---------------|
| Create Session | POST | `/session` | `ClaudeTerminalMIDProxyAPI.js` (createSession block) |
| Send Command | POST | `/session/{session_id}/command` | (sendCommand block) |
| Get Output | GET | `/session/{session_id}/output` | (getOutput block) |
| Get Status | GET | `/session/{session_id}/status` | (getStatus block) |
| Terminate | DELETE | `/session/{session_id}` | (terminateSession block) |
| Resize | POST | `/session/{session_id}/resize` | (resizeTerminal block) |

### 2.5 Service Portal Widget

1. Navigate to **Service Portal > Widgets**
2. Clone the existing Claude Terminal widget (or create new)
3. Replace:
   - **HTML Template:** `widgets/claude_terminal_mid/widget.html`
   - **Client Script:** `widgets/claude_terminal_mid/client_script.js`
   - **CSS/SCSS:** `widgets/claude_terminal_mid/styles.scss`
4. Add to a Service Portal page

## Step 3: Verify

### Test MID Server connectivity:

```bash
# Check MID Server is connected
# In ServiceNow: MID Server > Servers > k8s-mid-proxy-01 > Status = Up

# Test HTTP service directly
curl -s http://localhost:3001/health | python3 -m json.tool
```

### Test probe execution (from ServiceNow Scripts - Background):

```javascript
var api = new ClaudeTerminalAPI();
var ref = api.createSession('test.user', 'sk-ant-test-key', '', 'isolated');
gs.info('Probe submitted: ' + JSON.stringify(ref));

// Wait for result
var result = api.getProbeResult(ref.ecc_sys_id, 15000);
gs.info('Result: ' + JSON.stringify(result));
```

## Step 4: Run Benchmarks (Optional)

Run both setups simultaneously to compare:

```bash
# Start original setup (port 3000)
docker compose -f docker-compose.yml up -d

# Start MID proxy setup (port 3001)
docker compose -f mid-proxy/docker-compose.yml up -d

# Run benchmark
./mid-proxy/scripts/benchmark.sh
```

Results are saved to `mid-proxy/benchmark-results/`.

## Troubleshooting

### MID Server not picking up probes

1. Check MID Server status in ServiceNow instance
2. Verify ECC Queue has items: `ecc_queue.list` filter `topic=JavascriptProbe^state=ready`
3. Check MID Server logs: `docker logs midproxy-mid-server`

### Probe timeout (504 from REST API)

1. Increase `maxWaitMs` in `ClaudeTerminalAPI.getProbeResult()`
2. Check MID Server agent log for errors
3. Verify `claude-terminal-service` is reachable from MID container:
   ```bash
   docker exec midproxy-mid-server curl -s http://claude-terminal-service:3000/health
   ```

### Script Include not found on MID

1. Ensure the MID Server Script Include is **Active**
2. Restart the MID Server: `docker restart midproxy-mid-server`
3. Check MID Server > Script Includes to confirm it synced
