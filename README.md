# Claude Code Integration for ServiceNow MID Server

A comprehensive terminal-based integration that brings Claude Code CLI capabilities directly into your ServiceNow environment using MID Servers.

## Overview

This solution provides an interactive terminal UI in ServiceNow that connects to Claude Code CLI running on a MID server, enabling users to leverage Claude's AI-powered coding capabilities within their ServiceNow environment.

### Key Features

- 🖥️ **Interactive Terminal**: xterm.js-based terminal in ServiceNow UI
- ⚡ **Real-time Communication**: AMB notifications + adaptive polling (100ms-5s)
- 🔒 **User Isolation**: Separate Claude Code session per user with encrypted credentials
- 💾 **Session Persistence**: Resume capability for interrupted sessions
- 📊 **Audit Trail**: All commands and outputs logged in ServiceNow
- 🔐 **Secure Credentials**: User-based API key isolation with encryption

## Architecture

```
ServiceNow Instance                          MID Server
┌──────────────────────────┐                ┌────────────────────────┐
│                          │                │                        │
│  Service Portal Widget   │                │  Go HTTP Service       │
│  (xterm.js terminal)     │                │  (localhost:3000)      │
│         │                │                │         │              │
│         ↓                │                │         ↓              │
│  Scripted REST API       │   ECC Queue    │  Session Manager       │
│  + Session Table         │◄──────────────►│  + PTY Handler         │
│         │                │   (5s poll)    │         │              │
│         ↓                │                │         ↓              │
│  AMB Notifications       │                │  Claude Code CLI       │
│  (output_available)      │                │  (interactive mode)    │
│                          │                │                        │
└──────────────────────────┘                └────────────────────────┘
```

## Components

### MID Server Components (Go)

1. **HTTP Service** (`cmd/server/main.go`)
   - Manages Claude Code CLI sessions via PTY
   - REST API for session management
   - Output buffering and streaming
   - Session timeout handling

2. **ECC Queue Poller** (`cmd/ecc-poller/main.go`)
   - Polls ServiceNow ECC Queue for commands
   - Routes commands to HTTP service
   - Returns results to ServiceNow

### ServiceNow Components

1. **Tables**
   - `x_claude_terminal_session`: Session management
   - `x_claude_credentials`: Encrypted user credentials

2. **REST API**
   - Session CRUD operations
   - Command/output handling
   - Credential management

3. **Widgets**
   - `claude_terminal`: Interactive terminal UI
   - `claude_credential_setup`: API key configuration

4. **Business Rules**
   - AMB notifications on output updates

## Prerequisites

- ServiceNow instance (Tokyo or later)
- MID Server with:
  - Go 1.21+ installed
  - Claude Code CLI installed
  - Network access to ServiceNow instance
  - 2GB+ RAM recommended

## Installation

### 1. MID Server Setup

```bash
# Clone repository to MID Server
cd /opt/servicenow-mid
git clone <repo-url> claude-terminal-service

# Navigate to project directory
cd claude-terminal-service

# Copy and configure environment
cp .env.example .env
nano .env  # Edit with your ServiceNow details

# Build the binaries
make build

# Or build manually
go build -o bin/claude-terminal-service cmd/server/main.go
go build -o bin/ecc-poller cmd/ecc-poller/main.go
```

### 2. Configure Environment Variables

Edit `.env`:

```bash
SERVICENOW_INSTANCE=your-instance.service-now.com
SERVICENOW_API_USER=integration_user
SERVICENOW_API_PASSWORD=your_password
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

### 3. ServiceNow Configuration

#### Import Tables

1. Navigate to **System Definition > Tables**
2. Import table definitions from:
   - `servicenow/tables/x_claude_terminal_session.json`
   - `servicenow/tables/x_claude_credentials.json`

#### Import REST API

1. Navigate to **System Web Services > Scripted REST APIs**
2. Import: `servicenow/rest-api/claude_terminal_api.xml`

#### Import Business Rules

1. Navigate to **System Definition > Business Rules**
2. Import: `servicenow/business-rules/amb_output_notification.xml`

#### Import Widgets

1. Navigate to **Service Portal > Widgets**
2. Create widget `claude_terminal`:
   - Copy from `servicenow/widgets/claude_terminal/`
3. Create widget `claude_credential_setup`:
   - Copy from `servicenow/widgets/claude_credential_setup/`

#### Configure ACLs

Create the following ACLs:

**Table: x_claude_terminal_session**
- Users can CRUD their own sessions (user = current user)
- Admins can read all sessions

**Table: x_claude_credentials**
- Users can CRUD their own credentials (user = current user)
- No admin access to credential values

### 4. Start Services

```bash
# Start HTTP service
./bin/claude-terminal-service &

# Start ECC Queue poller
./bin/ecc-poller &

# Or use systemd (recommended)
sudo cp deployment/systemd/*.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable claude-terminal-service
sudo systemctl enable claude-ecc-poller
sudo systemctl start claude-terminal-service
sudo systemctl start claude-ecc-poller
```

## Usage

### User Setup

1. Navigate to **Claude Credential Setup** page
2. Enter your Anthropic API key
3. (Optional) Add GitHub token for enhanced integration
4. Test connection
5. Save credentials

### Using Claude Terminal

1. Navigate to **Claude Terminal** page
2. Terminal will initialize automatically
3. Type commands and interact with Claude Code
4. Sessions auto-save and can be resumed
5. Click "Terminate" to end session

## API Reference

### REST API Endpoints

#### Create Session
```http
POST /api/now/claude/terminal/session/create
Content-Type: application/json

{
  "workspaceType": "isolated"
}
```

#### Send Command
```http
POST /api/now/claude/terminal/session/{sessionId}/command
Content-Type: application/json

{
  "command": "help\n"
}
```

#### Get Output
```http
GET /api/now/claude/terminal/session/{sessionId}/output?clear=true
```

#### Get Status
```http
GET /api/now/claude/terminal/session/{sessionId}/status
```

#### Terminate Session
```http
DELETE /api/now/claude/terminal/session/{sessionId}
```

## Security

### Credential Storage

- API keys are encrypted using ServiceNow's password2 field type
- Keys are never transmitted to client browsers
- Each user can only access their own credentials
- Keys are passed to MID Server via encrypted HTTPS

### Session Isolation

- Each user gets separate Claude Code CLI process
- Isolated workspace directories per session
- No cross-user data access
- Automatic cleanup on session end

### Audit Logging

All activities are logged:
- Session creation/termination
- Commands executed (truncated)
- Output size and timing
- Errors and failures

## Troubleshooting

### Session won't start

1. Check MID Server logs: `tail -f /var/log/claude-terminal-service.log`
2. Verify Claude CLI is installed: `claude --version`
3. Check API key is valid in credential setup
4. Verify MID Server can reach ServiceNow

### No output appearing

1. Check AMB notifications are enabled
2. Verify ECC Queue poller is running
3. Check browser console for errors
4. Increase polling frequency in widget

### Performance issues

1. Reduce `MAX_SESSIONS_PER_USER` in .env
2. Increase `SESSION_TIMEOUT_MINUTES` to free resources
3. Monitor MID Server resources
4. Consider adding more MID Servers

## Development

### Build

```bash
make build
```

### Run Tests

```bash
make test
```

### Run Locally

```bash
make run
```

### Clean

```bash
make clean
```

## Contributing

1. Fork the repository
2. Create a feature branch
3. Commit your changes
4. Push to the branch
5. Create a Pull Request

## License

MIT License - see LICENSE file for details

## Support

For issues and questions:
- GitHub Issues: [repo-url]/issues
- ServiceNow Community: [community-link]

## Roadmap

- [ ] Multi-MID Server load balancing
- [ ] WebSocket direct connection option
- [ ] Session sharing and collaboration
- [ ] Enhanced audit dashboard
- [ ] Integration with ServiceNow ITSM workflows
- [ ] Custom Claude prompts/templates
