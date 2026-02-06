# Deployment Guide

This guide provides step-by-step instructions for deploying the Claude Code Integration to your ServiceNow MID Server environment.

## Prerequisites Checklist

Before starting deployment, ensure you have:

- [ ] ServiceNow instance (Tokyo or later)
- [ ] MID Server with root/sudo access
- [ ] Go 1.21+ installed on MID Server
- [ ] Claude Code CLI installed on MID Server
- [ ] Anthropic API key
- [ ] ServiceNow integration user credentials
- [ ] Network connectivity: MID Server → ServiceNow
- [ ] Firewall rules allowing localhost:3000

## Phase 1: ServiceNow Configuration (30-45 minutes)

### Step 1: Create Custom Tables

1. Navigate to **System Definition > Tables**

2. **Create Table: x_claude_terminal_session**
   - Copy the table definition from `servicenow/tables/x_claude_terminal_session.json`
   - Click **New** → **Create from file**
   - Or manually create with fields as specified in the JSON

3. **Create Table: x_claude_credentials**
   - Copy the table definition from `servicenow/tables/x_claude_credentials.json`
   - Ensure password2 encryption is enabled for API key fields

### Step 2: Configure ACLs

1. Navigate to **System Security > Access Control (ACL)**

2. **Create ACL for x_claude_terminal_session**
   ```javascript
   // Read ACL
   (function() {
       // Users can read their own sessions
       if (current.user == gs.getUserID()) {
           return true;
       }
       // Admins can read all
       if (gs.hasRole('admin')) {
           return true;
       }
       return false;
   })();
   ```

3. **Create ACL for x_claude_credentials**
   ```javascript
   // CRUD ACL
   (function() {
       // Users can only access their own credentials
       return current.user == gs.getUserID();
   })();
   ```

### Step 3: Import REST API

1. Navigate to **System Web Services > Scripted REST APIs**
2. Click **New**
3. Copy content from `servicenow/rest-api/claude_terminal_api.xml`
4. Configure authentication (basic auth recommended)
5. Test endpoints using REST API Explorer

### Step 4: Import Business Rules

1. Navigate to **System Definition > Business Rules**
2. Import `servicenow/business-rules/amb_output_notification.xml`
3. Verify AMB is enabled on your instance:
   - Navigate to **System Properties > Asynchronous Message Bus**
   - Ensure AMB is active

### Step 5: Create Service Portal Widgets

#### Widget 1: claude_terminal

1. Navigate to **Service Portal > Widgets**
2. Click **New**
3. Set:
   - **ID**: `claude_terminal`
   - **Name**: Claude Terminal
   - **Data table**: (leave empty)
4. Copy content:
   - **HTML Template**: from `servicenow/widgets/claude_terminal/widget.html`
   - **Client Script**: from `servicenow/widgets/claude_terminal/client_script.js`
   - **CSS/SCSS**: from `servicenow/widgets/claude_terminal/styles.scss`
5. Save

#### Widget 2: claude_credential_setup

1. Click **New** again
2. Set:
   - **ID**: `claude_credential_setup`
   - **Name**: Claude Credential Setup
3. Copy content:
   - **HTML Template**: from `servicenow/widgets/claude_credential_setup/widget.html`
   - **Client Script**: from `servicenow/widgets/claude_credential_setup/client_script.js`
4. Save

### Step 6: Create Portal Pages

1. Navigate to **Service Portal > Pages**

2. **Create page: claude-terminal**
   - **Title**: Claude Terminal
   - **ID**: `claude_terminal`
   - Add widget: `claude_terminal`

3. **Create page: claude-setup**
   - **Title**: API Key Setup
   - **ID**: `claude_credential_setup`
   - Add widget: `claude_credential_setup`

### Step 7: Create Menu Items

1. Navigate to **Service Portal > Portals**
2. Select your portal (e.g., "Employee Center")
3. Add menu items:
   - **Claude Terminal** → `/claude-terminal`
   - **API Setup** → `/claude-setup`

## Phase 2: MID Server Setup (30-45 minutes)

### Step 1: Install Prerequisites

```bash
# Install Go (if not installed)
wget https://go.dev/dl/go1.21.5.linux-amd64.tar.gz
sudo tar -C /usr/local -xzf go1.21.5.linux-amd64.tar.gz
echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.bashrc
source ~/.bashrc

# Verify Go installation
go version

# Install Claude Code CLI (if not installed)
# Follow instructions at: https://docs.anthropic.com/claude/docs/cli
```

### Step 2: Deploy Application

```bash
# Create application directory
sudo mkdir -p /opt/servicenow-mid/claude-terminal
cd /opt/servicenow-mid/claude-terminal

# Copy files (or clone from git)
# If using git:
git clone <your-repo-url> .

# Or manually copy all files to this directory
```

### Step 3: Configure Environment

```bash
# Copy environment template
cp .env.example .env

# Edit configuration
nano .env
```

Update the following values:
```bash
SERVICENOW_INSTANCE=yourinstance.service-now.com
SERVICENOW_API_USER=claude_integration_user
SERVICENOW_API_PASSWORD=your_secure_password
MID_SERVER_NAME=your_mid_server_name

NODE_SERVICE_PORT=3000
NODE_SERVICE_HOST=localhost

WORKSPACE_BASE_PATH=/tmp/claude-sessions
WORKSPACE_TYPE=isolated

SESSION_TIMEOUT_MINUTES=30
MAX_SESSIONS_PER_USER=3

LOG_LEVEL=info
LOG_FILE=/var/log/claude-terminal-service.log
```

### Step 4: Build Binaries

```bash
# Download dependencies
make deps

# Build both services
make build

# Verify binaries
ls -lh bin/
# Should see:
# - claude-terminal-service
# - ecc-poller
```

### Step 5: Create SystemD Services

```bash
# Create systemd service files
sudo nano /etc/systemd/system/claude-terminal-service.service
```

Paste:
```ini
[Unit]
Description=Claude Terminal Service
After=network.target

[Service]
Type=simple
User=mid
WorkingDirectory=/opt/servicenow-mid/claude-terminal
Environment="PATH=/usr/local/go/bin:/usr/local/bin:/usr/bin:/bin"
EnvironmentFile=/opt/servicenow-mid/claude-terminal/.env
ExecStart=/opt/servicenow-mid/claude-terminal/bin/claude-terminal-service
Restart=always
RestartSec=10

[Install]
WantedBy=multi-user.target
```

Create ECC poller service:
```bash
sudo nano /etc/systemd/system/claude-ecc-poller.service
```

Paste:
```ini
[Unit]
Description=Claude ECC Queue Poller
After=network.target claude-terminal-service.service

[Service]
Type=simple
User=mid
WorkingDirectory=/opt/servicenow-mid/claude-terminal
Environment="PATH=/usr/local/go/bin:/usr/local/bin:/usr/bin:/bin"
EnvironmentFile=/opt/servicenow-mid/claude-terminal/.env
ExecStart=/opt/servicenow-mid/claude-terminal/bin/ecc-poller
Restart=always
RestartSec=10

[Install]
WantedBy=multi-user.target
```

### Step 6: Start Services

```bash
# Reload systemd
sudo systemctl daemon-reload

# Enable services (start on boot)
sudo systemctl enable claude-terminal-service
sudo systemctl enable claude-ecc-poller

# Start services
sudo systemctl start claude-terminal-service
sudo systemctl start claude-ecc-poller

# Check status
sudo systemctl status claude-terminal-service
sudo systemctl status claude-ecc-poller

# View logs
sudo journalctl -u claude-terminal-service -f
sudo journalctl -u claude-ecc-poller -f
```

## Phase 3: Testing & Validation (15-30 minutes)

### Test 1: Service Health

```bash
# Test HTTP service directly
curl http://localhost:3000/health

# Expected response:
# {"status":"healthy","uptime":...}
```

### Test 2: ServiceNow Credentials Setup

1. Log into ServiceNow
2. Navigate to **API Setup** page
3. Enter test API key
4. Click **Test Connection**
5. Verify success message
6. Click **Save Credentials**

### Test 3: Terminal Session

1. Navigate to **Claude Terminal** page
2. Wait for terminal to initialize (5-10 seconds)
3. Terminal should display Claude Code prompt
4. Type: `help` + Enter
5. Verify output appears in terminal
6. Try a simple command: `What can you help me with?`

### Test 4: Multi-User Isolation

1. Open terminal in two different browser sessions (different users)
2. Start sessions for both users
3. Verify each gets their own session ID
4. Verify users cannot see each other's sessions in the database

### Test 5: Session Persistence

1. Start a terminal session
2. Begin a conversation with Claude
3. Close browser tab
4. Reopen and navigate back to terminal page
5. Verify session can resume (if persistence enabled)

### Test 6: ECC Queue Processing

1. Navigate to **System Logs > ECC Queue**
2. Filter: `topic=ClaudeTerminalCommand`
3. Verify items are being processed (state changes)
4. Check processing time is reasonable (<1 second)

## Phase 4: Monitoring Setup (Optional, 15 minutes)

### Configure Log Rotation

```bash
sudo nano /etc/logrotate.d/claude-terminal
```

Paste:
```
/var/log/claude-terminal-service.log {
    daily
    rotate 7
    compress
    delaycompress
    notifempty
    create 0640 mid mid
    sharedscripts
    postrotate
        systemctl reload claude-terminal-service > /dev/null 2>&1 || true
    endscript
}
```

### Set Up Health Check

Add to cron:
```bash
crontab -e
```

Add:
```
*/5 * * * * curl -s http://localhost:3000/health > /dev/null || systemctl restart claude-terminal-service
```

## Troubleshooting

### Issue: Services won't start

**Solution:**
```bash
# Check logs
sudo journalctl -u claude-terminal-service -n 50

# Common issues:
# 1. Missing .env file
# 2. Invalid ServiceNow credentials
# 3. Port 3000 already in use
# 4. Claude CLI not in PATH

# Fix PATH issue:
which claude
sudo ln -s /path/to/claude /usr/local/bin/claude
```

### Issue: Terminal won't connect

**Solution:**
1. Check MID Server can reach ServiceNow:
   ```bash
   curl -I https://yourinstance.service-now.com
   ```
2. Verify ECC Queue poller is running
3. Check browser console for errors
4. Verify AMB is enabled in ServiceNow

### Issue: Credentials not saving

**Solution:**
1. Check ACLs on x_claude_credentials table
2. Verify user has write access
3. Check browser console for API errors
4. Verify REST API authentication

## Rollback Procedure

If deployment fails:

1. **Stop services:**
   ```bash
   sudo systemctl stop claude-terminal-service
   sudo systemctl stop claude-ecc-poller
   ```

2. **Remove ServiceNow components:**
   - Delete widgets
   - Delete business rules
   - Delete REST API
   - (Optional) Delete tables if no data

3. **Remove MID Server files:**
   ```bash
   sudo rm -rf /opt/servicenow-mid/claude-terminal
   sudo rm /etc/systemd/system/claude-*.service
   sudo systemctl daemon-reload
   ```

## Post-Deployment Checklist

- [ ] All services running and healthy
- [ ] Users can create sessions
- [ ] Terminal displays output correctly
- [ ] Credentials are encrypted and isolated
- [ ] ACLs prevent unauthorized access
- [ ] Logs are being written correctly
- [ ] Log rotation configured
- [ ] Health checks in place
- [ ] Documentation provided to users
- [ ] Backup/disaster recovery plan documented

## Next Steps

1. Train users on credential setup
2. Monitor resource usage (CPU, memory, disk)
3. Set up alerts for service failures
4. Plan for scaling (add more MID Servers if needed)
5. Regular security audits of credentials
6. Review and optimize session timeout settings

## Customization

### Repository URLs

Replace the following placeholders in documentation with your actual repository URL:
- `README.md` - `git clone <repo-url>` and support section
- `DEPLOYMENT.md` - `git clone <your-repo-url>`

### Go Module Path

To use your own Go module path:

1. Update `go.mod`:
```go
module github.com/your-org/your-module-name
```

2. Update all import statements in:
   - `cmd/server/main.go`
   - `cmd/ecc-poller/main.go`
   - `internal/server/server.go`
   - `internal/servicenow/client.go`
   - `internal/session/session.go`

3. Run `go mod tidy`

### ServiceNow Application Scope

To deploy as a scoped application:

1. Create a new application scope in ServiceNow
2. Update table names to include your scope prefix:
   ```
   x_yourscope_terminal_session
   x_yourscope_credentials
   ```
3. Update all references in REST API, business rules, widget scripts, and Go code

### Widget Branding

- Terminal Widget: update header in `servicenow/widgets/claude_terminal/widget.html`, colors in `styles.scss`
- Credential Setup Widget: update help text in `servicenow/widgets/claude_credential_setup/widget.html`

### Deployment Path

Default installation path: `/opt/servicenow-mid/claude-terminal`

To change, update SystemD service files (`WorkingDirectory`, `ExecStart`, `EnvironmentFile` paths).

### Security: Integration User

Create a dedicated integration user with minimal permissions:
- `rest_api_explorer` role for REST API access
- Custom role for ECC Queue access
- Read/Write on session and credentials tables
- Do NOT use `admin` role or personal accounts

## Support

For issues during deployment:
- Check logs: `/var/log/claude-terminal-service.log`
- Review ServiceNow system logs
- GitHub Issues: [your-repo]/issues
