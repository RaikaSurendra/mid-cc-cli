# MID Server Proxy — Setup Guide

## Prerequisites

- Docker + Docker Compose
- A ServiceNow instance with:
  - A registered MID Server
  - Admin access to create Script Includes, Scripted REST APIs, and Widgets
- The Claude Terminal tables already deployed (from root `servicenow/` update set)

## Step 1: Start the Docker Stack

```bash
# From project root
cd mid-proxy

# Create .env file
cat > .env << 'EOF'
SN_INSTANCE_URL=https://your-instance.service-now.com
SN_USERNAME=mid.user
SN_PASSWORD=your-mid-password
MID_SERVER_NAME=mid-docker-proxy
API_AUTH_TOKEN=your-secure-token
ENCRYPTION_KEY=your-32-char-hex-key-here-abcdef
EOF

# Start services
docker compose up -d

# Verify health
docker compose ps
curl http://localhost:3000/health
```

## Step 2: Verify MID Server Connection

1. Go to your ServiceNow instance
2. Navigate to **MID Server → Servers**
3. Verify `mid-docker-proxy` shows status **Up**
4. Run a quick test: **MID Server → Test Connection**

## Step 3: Deploy ServiceNow Artifacts

### Script Includes

1. Navigate to **System Definition → Script Includes**
2. Create two Script Includes:

**ClaudeTerminalAPI** (runs on instance):
- Name: `ClaudeTerminalAPI`
- Client callable: `false`
- Active: `true`
- Script: Copy from `servicenow/script-includes/ClaudeTerminalAPI.js`

**ClaudeTerminalProbe** (runs on MID Server):
- Name: `ClaudeTerminalProbe`
- Client callable: `false`
- Active: `true`
- **MID Server Script Include**: `true` (check this box!)
- Script: Copy from `servicenow/script-includes/ClaudeTerminalProbe.js`

### Scripted REST API

1. Navigate to **System Web Services → Scripted REST APIs**
2. Create new API:
   - Name: `Claude Terminal MID Proxy`
   - API ID: `x_claude_terminal_mid`
   - API namespace: `x_claude`
3. Create a **Default GET** resource and a **Default POST** resource
4. In each resource script, paste from `servicenow/scripted-rest/ClaudeTerminalMIDProxyAPI.js`

### System Properties

1. Navigate to **System Properties → Properties**
2. Create these properties:

| Name | Value |
|------|-------|
| `x_claude.terminal.mid_server` | `mid-docker-proxy` |
| `x_claude.terminal.service_url` | `http://claude-terminal-service:3000` |
| `x_claude.terminal.auth_token` | Same value as `API_AUTH_TOKEN` in .env |

### Widget (Optional)

1. Navigate to **Service Portal → Widgets**
2. Create new widget: `Claude Terminal MID`
3. Copy HTML from `servicenow/widgets/claude_terminal_mid/widget.html`
4. Copy Client Script from `servicenow/widgets/claude_terminal_mid/client_script.js`
5. Copy CSS from `servicenow/widgets/claude_terminal_mid/styles.scss`
6. Add to a Service Portal page

## Step 4: Test End-to-End

### Quick Test via REST

```bash
# Create a session via the MID proxy REST API
curl -X POST "https://your-instance.service-now.com/api/x_claude/terminal_mid/session" \
  -H "Content-Type: application/json" \
  -H "Authorization: Basic $(echo -n 'admin:password' | base64)" \
  -d '{
    "credentials": { "anthropicApiKey": "sk-ant-..." },
    "workspaceType": "temp"
  }'
```

### Monitor the Flow

1. **ECC Queue**: Navigate to `ecc_queue.list` and filter by `topic=JavascriptProbe`
2. **MID Server Logs**: Check **MID Server → Log Files** for `ClaudeTerminalProbe` entries
3. **System Logs**: Check `syslog.list` for `ClaudeTerminalAPI` debug messages

## Troubleshooting

| Symptom | Cause | Fix |
|---------|-------|-----|
| "mid_server property not configured" | Missing system property | Add `x_claude.terminal.mid_server` property |
| Timeout waiting for response | MID Server not picking up probes | Check MID Server is Up, restart if needed |
| HTTP 502 from REST API | Service unreachable from MID Server | Verify Docker networking, check `service_url` |
| "Unknown action" in MID logs | Probe parameter mismatch | Verify ClaudeTerminalAPI is sending correct `action` |
| Widget shows "Disconnected" | REST API errors | Check browser console, verify REST API exists |

## Benchmarking

Compare ECC Poller vs MID Proxy latency:

```bash
./scripts/benchmark.sh
```

This sends 10 test commands through each approach and reports average roundtrip time.
