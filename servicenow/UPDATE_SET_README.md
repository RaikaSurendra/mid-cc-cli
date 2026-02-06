# Claude Terminal FULL Update Set - Installation Guide

## Overview

This is a **production-ready, complete ServiceNow update set** for the Claude Terminal MID Service integration. The update set contains **EVERYTHING** needed to run Claude Code Terminal directly in ServiceNow via MID Server.

**File:** `Claude-Terminal-FULL-UpdateSet.xml`
**Size:** 124KB (1,919 lines)
**Components:** 54 update records
**Scope:** x_snc_claude_mid
**Version:** 1.0.0

## What's Included

### 1. Application Scope (1 component)
- **sys_app**: x_snc_claude_mid
  - Application name: Claude Terminal MID Service
  - Scope: x_snc_claude_mid
  - Fully configured for scoped application development

### 2. Database Tables (2 tables with 14 field definitions)

#### Table: x_snc_claude_mid_terminal_session
- **session_id** (string, 40 chars, unique, mandatory)
- **user** (reference to sys_user, mandatory)
- **status** (choice: initializing, active, idle, terminated, error)
- **output_buffer** (JSON, 65000 chars) - stores terminal output
- **workspace_path** (string, 255 chars)
- **workspace_type** (choice: isolated, persistent)
- **last_activity** (glide_date_time, mandatory)
- **mid_server** (string, 100 chars)
- **error_message** (string, 1000 chars)

#### Table: x_snc_claude_mid_credentials
- **user** (reference to sys_user, unique, mandatory, display field)
- **anthropic_api_key** (password2, encrypted, mandatory)
- **github_token** (password2, encrypted, optional)
- **validation_status** (choice: valid, invalid, untested)
- **last_used** (glide_date_time)
- **last_validated** (glide_date_time)

All fields include:
- Proper encryption for sensitive data
- Choice lists with labels
- Correct data types and constraints
- Mandatory/unique flags as needed

### 3. Script Include (1 component)
**Name:** ClaudeTerminalAPI

**Methods:**
- `createSession(userId, workspaceType)` - Create new terminal session
- `sendCommand(sessionId, command)` - Send command to terminal
- `getOutput(sessionId, clear)` - Retrieve terminal output
- `getStatus(sessionId)` - Get session status
- `terminateSession(sessionId)` - Terminate session
- `resizeTerminal(sessionId, cols, rows)` - Resize terminal

**Features:**
- ECC Queue integration for MID Server communication
- Secure credential handling with decryption
- UUID generation for session IDs
- Comprehensive error handling
- Activity tracking

### 4. Business Rule (1 component)
**Name:** AMB Output Notification

- **Table:** x_snc_claude_mid_terminal_session
- **When:** After Update
- **Condition:** `output_buffer.changes()`
- **Purpose:** Publishes AMB (Ambulatory Message Bus) notifications when terminal output is updated
- **Channel:** `claude.terminal.{sessionId}`

This enables real-time terminal output updates without polling.

### 5. Scripted REST API (7 components)

#### REST API Definition
- **Base URI:** /api/x_snc_claude_mid/terminal
- **Namespace:** x_snc_claude_mid
- **Service Name:** terminal

#### REST API Operations

1. **POST /session/create**
   - Creates new terminal session
   - Body: `{workspaceType: "isolated" | "persistent"}`
   - Returns: `{success, sessionId, status}`

2. **POST /session/{sessionId}/command**
   - Sends command to terminal
   - Body: `{command: string}`
   - Returns: `{success}`

3. **GET /session/{sessionId}/output**
   - Gets terminal output
   - Query param: `clear=true` (optional)
   - Returns: `{success, sessionId, output[], status}`

4. **GET /session/{sessionId}/status**
   - Gets session status
   - Returns: `{success, sessionId, status, userId, workspacePath, lastActivity, created}`

5. **DELETE /session/{sessionId}**
   - Terminates session
   - Returns: `{success, message}`

6. **POST /session/{sessionId}/resize**
   - Resizes terminal dimensions
   - Body: `{cols: number, rows: number}`
   - Returns: `{success}`

All operations include:
- Authentication required
- ACL authorization
- Error handling with proper HTTP status codes
- JSON request/response

### 6. Service Portal Widgets (2 components)

#### Widget: claude_terminal
**ID:** claude_terminal

**Features:**
- Full xterm.js terminal integration (v5.3.0)
- Real-time terminal output via AMB subscriptions
- Adaptive polling (100ms-5000ms based on activity)
- Keyboard input handling
- Terminal resize detection
- Status indicators (initializing, active, idle, terminated, error)
- Error handling with retry functionality
- Credential check with setup link
- Clean session termination

**Client Script:** 305 lines of AngularJS controller
**Template:** Complete HTML with terminal UI
**CSS:** 133 lines of responsive styling with VS Code dark theme

#### Widget: claude_credential_setup
**ID:** claude_credential_setup

**Features:**
- Anthropic API key management (required)
- GitHub token management (optional)
- Credential validation/testing
- Save/delete operations
- Security notice display
- Status tracking (last used, validation status)
- Getting started guide
- Form validation

**Client Script:** 152 lines of AngularJS controller
**Template:** Complete HTML with Bootstrap panels

## Installation Instructions

### Prerequisites
1. ServiceNow instance (Helsinki or later)
2. Admin access
3. MID Server configured and running
4. Service Portal enabled (for widgets)

### Step 1: Import Update Set

1. Navigate to **Retrieved Update Sets** (sys_remote_update_set_list.do)
2. Click **Import Update Set from XML**
3. Upload `Claude-Terminal-FULL-UpdateSet.xml`
4. Wait for import to complete

### Step 2: Preview Update Set

1. Open the imported update set: "Claude Terminal MID Service - Complete"
2. Click **Preview Update Set**
3. Wait for preview to complete
4. Review any warnings or errors (there should be none)

### Step 3: Commit Update Set

1. After successful preview, click **Commit Update Set**
2. Wait for commit to complete
3. Verify all 54 components are committed successfully

### Step 4: Verify Installation

Navigate to the following to verify:

1. **Tables:**
   - `x_snc_claude_mid_terminal_session.list`
   - `x_snc_claude_mid_credentials.list`

2. **Script Include:**
   - Navigate to **Script Includes**
   - Search for "ClaudeTerminalAPI"

3. **Business Rule:**
   - Navigate to **Business Rules**
   - Search for "AMB Output Notification"

4. **REST API:**
   - Navigate to **Scripted REST APIs**
   - Search for "Claude Terminal API"
   - Verify 6 operations exist

5. **Service Portal Widgets:**
   - Navigate to **Service Portal > Widgets**
   - Search for "claude_terminal"
   - Search for "claude_credential_setup"

### Step 5: Configure MID Server

You need to install the MID Server agent that handles the Claude Code integration:

1. Ensure your MID Server is running
2. Deploy the Claude Terminal MID Server agent (separate component)
3. Configure ECC Queue topic subscriptions:
   - Topic: ClaudeTerminalCommand
   - Topic: ClaudeTerminalResponse

### Step 6: Create Service Portal Page (Optional)

To make the widgets accessible:

1. Navigate to **Service Portal > Pages**
2. Create a new page with ID: `claude_terminal`
3. Add the `claude_terminal` widget to the page
4. Create another page with ID: `claude_credential_setup`
5. Add the `claude_credential_setup` widget to the page

Or add the widgets to existing pages.

## Post-Installation Configuration

### User Setup

Each user needs to configure their credentials:

1. Navigate to the credential setup page (or widget)
2. Enter Anthropic API key (get from https://console.anthropic.com/)
3. Optionally enter GitHub token for enhanced integration
4. Click **Test Connection** to verify
5. Click **Save Credentials**

### Testing

1. Navigate to the Claude Terminal page/widget
2. System will automatically create a terminal session
3. Terminal should initialize and become active
4. Type commands and verify output appears
5. Test session termination

## Update Set Contents Summary

```
Total Components: 54

Breakdown:
- 1 Application Scope (sys_app)
- 1 Remote Update Set Metadata (sys_remote_update_set)
- 2 Tables (sys_db_object)
- 14 Table Fields (sys_dictionary with choice lists)
- 1 Script Include (sys_script_include)
- 1 Business Rule (sys_script)
- 1 REST API Definition (sys_ws_definition)
- 6 REST API Operations (sys_ws_operation)
- 2 Service Portal Widgets (sys_sp_widget)
- 25 Supporting records (sys_choice, metadata, etc.)
```

## Architecture Overview

### Communication Flow

```
Service Portal Widget (Browser)
    ↓ REST API
ServiceNow Instance
    ↓ ClaudeTerminalAPI
    ↓ ECC Queue
MID Server (Claude Terminal Agent)
    ↓ Node.js Process
    ↓ PTY (Pseudo Terminal)
Claude Code CLI
    ↑ Terminal Output
    ↓ ECC Queue Response
ServiceNow Instance
    ↓ AMB Notification
Service Portal Widget (Real-time Update)
```

### Data Flow

1. **User Input** → Widget captures keystrokes
2. **REST API** → POST to /session/{id}/command
3. **Script Include** → Writes to ECC Queue
4. **MID Server** → Reads ECC Queue, executes command
5. **Terminal Output** → MID Server writes to session record
6. **Business Rule** → Triggers AMB notification
7. **Widget** → Receives AMB, polls for output
8. **Display** → Writes to xterm.js terminal

## Security Features

### Encryption
- API keys stored with password2 (2-way encryption)
- Edge encryption enabled for credential fields
- Keys never transmitted to browser

### Access Control
- User-specific credential isolation
- REST API requires authentication
- ACL authorization on all operations
- Session-user validation

### Audit
- Activity tracking on sessions
- Last used timestamps on credentials
- Validation status tracking
- Error logging

## Troubleshooting

### Import Issues

**Problem:** Update set fails to import
- **Solution:** Check XML file integrity, ensure no corruption

**Problem:** Preview shows errors
- **Solution:** Review errors, may need to manually create dependencies

### Runtime Issues

**Problem:** Terminal doesn't initialize
- **Solution:** Check credentials are configured, MID Server is running

**Problem:** No output appears
- **Solution:** Verify ECC Queue is processing, check MID Server logs

**Problem:** AMB notifications not working
- **Solution:** Ensure AMB is enabled on instance, check business rule is active

### MID Server Issues

**Problem:** Commands not executing
- **Solution:** Check MID Server logs, verify ECC Queue subscription

**Problem:** Sessions stuck in initializing
- **Solution:** Restart MID Server, check agent configuration

## File Structure

```
servicenow/
├── Claude-Terminal-FULL-UpdateSet.xml  (This file - COMPLETE update set)
├── UPDATE_SET_README.md                (This documentation)
└── [Other development files - not needed for import]
```

## Version History

### Version 1.0.0 (2024-01-24)
- Initial release
- Complete production-ready update set
- All 54 components included
- Scope: x_snc_claude_mid
- Tables: terminal_session, credentials
- Script Include: ClaudeTerminalAPI
- Business Rule: AMB Output Notification
- REST API: 6 operations
- Widgets: claude_terminal, claude_credential_setup

## Support

### Documentation
- README files in repository
- Inline code comments
- Widget descriptions

### Known Limitations
- Requires active MID Server
- One session per user at a time (by design)
- Terminal limited to 600px height (configurable in CSS)
- Output buffer limited to 65KB (expandable if needed)

## License

[Your License Here]

## Credits

Built for ServiceNow Claude Terminal MID Service integration.

---

## Quick Start Checklist

- [ ] Import update set XML
- [ ] Preview update set (verify no errors)
- [ ] Commit update set
- [ ] Verify all 54 components installed
- [ ] Configure MID Server with Claude agent
- [ ] Create Service Portal pages for widgets
- [ ] Configure user credentials (Anthropic API key)
- [ ] Test terminal session creation
- [ ] Verify command execution and output
- [ ] Test session termination

---

**This is a COMPLETE, production-ready update set. No manual configuration required after import (except MID Server and user credentials).**
