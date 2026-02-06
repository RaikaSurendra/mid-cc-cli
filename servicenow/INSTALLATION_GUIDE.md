# ServiceNow Components Installation Guide

## Overview

This guide explains how to install the Claude Terminal MID Service components into your ServiceNow instance.

## What's Included

The update set (`Claude-Terminal-MID-Update-Set.xml`) includes:

### 1. Application Scope
- **Scope:** `x_snc_claude_mid`
- **Name:** Claude Terminal MID Service
- **Version:** 1.0.0

### 2. Tables

**x_snc_claude_mid_terminal_session**
- Session ID (String, unique)
- User (Reference to sys_user)
- Status (Choice: initializing, active, idle, terminated, error)
- Output Buffer (JSON, encrypted)
- Workspace Path (String)
- Last Activity (Date/Time)

**x_snc_claude_mid_credentials**
- User (Reference to sys_user, unique)
- Anthropic API Key (Password2, encrypted)
- GitHub Token (Password2, encrypted, optional)
- Last Used (Date/Time)
- Validation Status (Choice)

### 3. REST API

**Base URI:** `/api/x_snc_claude_mid/terminal`

**Endpoints:**
- `POST /session/create` - Create new session
- `POST /session/{id}/command` - Send command
- `GET /session/{id}/output` - Get output
- `GET /session/{id}/status` - Get status
- `DELETE /session/{id}` - Terminate session

### 4. Business Rules
- AMB Output Notification (triggers on output_buffer changes)

### 5. Service Portal Widgets
- claude_terminal - Interactive terminal UI
- claude_credential_setup - API key configuration

## Installation Steps

### Prerequisites

- ServiceNow instance (Tokyo or later recommended)
- Admin access to the instance
- MID Server configured and running

### Step 1: Import the Update Set

1. **Navigate to System Update Sets**
   ```
   System Update Sets > Retrieved Update Sets
   ```

2. **Import XML File**
   - Click "Import Update Set from XML"
   - Select `Claude-Terminal-MID-Update-Set.xml`
   - Click "Upload"

3. **Wait for Import**
   - The system will parse the XML
   - Verify no errors appear

### Step 2: Preview the Update Set

1. **Open the Update Set**
   - Find "Claude Terminal MID Service v1.0.0"
   - Click to open

2. **Preview Updates**
   - Click "Preview Update Set"
   - Review any preview problems
   - Resolve conflicts if any appear

### Step 3: Commit the Update Set

1. **Commit Changes**
   - Click "Commit Update Set"
   - Wait for commit to complete
   - Verify all updates applied successfully

2. **Verify Installation**
   ```
   System Applications > Applications
   Search for: "Claude Terminal MID Service"
   Status should be: Active
   ```

### Step 4: Configure ACLs

1. **Create Role for Users**
   ```
   User Administration > Roles > New
   Name: x_snc_claude_mid.user
   Description: Claude Terminal User Access
   ```

2. **Create Role for Admins**
   ```
   Name: x_snc_claude_mid.admin
   Description: Claude Terminal Administrator Access
   ```

3. **Set Table ACLs**

   **For x_snc_claude_mid_terminal_session:**
   - Read: User can read their own sessions
     ```javascript
     // ACL Script
     (function() {
         if (current.user == gs.getUserID()) return true;
         if (gs.hasRole('x_snc_claude_mid.admin')) return true;
         return false;
     })();
     ```

   - Write: User can update their own sessions
   - Delete: User can delete their own sessions

   **For x_snc_claude_mid_credentials:**
   - Read/Write: User can only access their own credentials
     ```javascript
     // ACL Script
     (function() {
         return current.user == gs.getUserID();
     })();
     ```

### Step 5: Configure Service Portal

1. **Add Widgets to Portal**
   ```
   Service Portal > Portals
   Select your portal (e.g., "Employee Center")
   ```

2. **Create Terminal Page**
   - Navigate to: Service Portal > Pages
   - Click "New"
   - **Page ID:** `claude_terminal`
   - **Title:** Claude Terminal
   - **Short Description:** Interactive Claude Code terminal

3. **Add Terminal Widget**
   - In the page designer
   - Add widget: `claude_terminal`
   - Save and publish

4. **Create Credentials Page**
   - **Page ID:** `claude_credentials`
   - **Title:** API Key Setup
   - Add widget: `claude_credential_setup`

5. **Add to Navigation**
   - Portal menu > Add menu item
   - **Title:** Claude Terminal
   - **Link:** `/claude_terminal`
   - **Icon:** terminal (or preferred icon)

### Step 6: Test the Installation

1. **Test Table Access**
   ```
   Navigate to: x_snc_claude_mid_terminal_session.list
   Verify table loads without errors
   ```

2. **Test REST API**
   ```
   Navigate to: System Web Services > Scripted REST APIs
   Find: Claude Terminal API
   Test: Open REST API Explorer
   ```

3. **Test Portal Widgets**
   ```
   Navigate to your portal
   Access: /claude_credentials
   Verify widget loads correctly
   ```

## Post-Installation Configuration

### 1. Configure MID Server

Ensure MID Server has:
- Go service running (claude-terminal-service)
- ECC Queue poller running (ecc-poller)
- Proper .env configuration

### 2. Set Up Users

1. **Assign Roles**
   ```
   User Administration > Users
   Select user > Roles tab
   Add role: x_snc_claude_mid.user
   ```

2. **User Onboarding**
   - Direct users to: `/claude_credentials`
   - Users enter their Anthropic API key
   - Test connection
   - Save credentials

### 3. Configure Monitoring

1. **Set Up Health Checks**
   - Monitor ECC Queue processing
   - Monitor session counts
   - Monitor MID Server health

2. **Set Up Alerting**
   - Alert on session failures
   - Alert on MID Server down
   - Alert on API errors

## Troubleshooting

### Import Errors

**Problem:** "Collision detected"
**Solution:**
- Review colliding records
- Choose to skip or overwrite
- Typically safe to overwrite for new installation

**Problem:** "Missing dependencies"
**Solution:**
- Ensure instance is up to date
- Check for required plugins
- Install missing dependencies

### Preview Problems

**Problem:** "Record not found"
**Solution:**
- This is normal for new records
- Proceed with commit

**Problem:** "ACL will block access"
**Solution:**
- Note the warning
- Configure ACLs after commit (Step 4)

### Commit Failures

**Problem:** "Update set failed to commit"
**Solution:**
1. Check system logs
2. Verify no active sessions
3. Retry commit
4. Contact ServiceNow support if persists

### Runtime Errors

**Problem:** "Table not found"
**Solution:**
- Verify update set committed
- Check table exists: `x_snc_claude_mid_terminal_session.list`
- Impersonate admin and retry

**Problem:** "Access denied"
**Solution:**
- Configure ACLs (Step 4)
- Assign proper roles to users
- Clear cache: `cache.do`

## Verification Checklist

After installation, verify:

- [ ] Application appears in System Applications
- [ ] Tables exist and are accessible
- [ ] REST API appears in Scripted REST APIs
- [ ] Widgets appear in Service Portal > Widgets
- [ ] Portal pages are accessible
- [ ] ACLs are configured correctly
- [ ] Users can access credential setup page
- [ ] Test session can be created (after MID Server setup)

## Uninstallation

To remove the application:

1. **Deactivate Application**
   ```
   System Applications > Applications
   Find: Claude Terminal MID Service
   Actions > Deactivate
   ```

2. **Delete Update Set** (optional)
   ```
   System Update Sets > Retrieved Update Sets
   Find and delete the update set
   ```

3. **Remove Tables** (if needed)
   ```
   System Definition > Tables
   Delete: x_snc_claude_mid_terminal_session
   Delete: x_snc_claude_mid_credentials
   ```

**Warning:** Deleting tables will remove all session data and user credentials!

## Support

### Documentation
- Main README: See repository root
- MID Server Setup: See DEPLOYMENT.md
- API Reference: See REST API Explorer in instance

### Issues
- Check system logs: System Logs > System Log > All
- Check ECC Queue: MID Server > ECC Queue
- Review error messages in terminal widget

### Getting Help
1. Review this guide
2. Check logs (ServiceNow + MID Server)
3. Verify configuration
4. Contact your ServiceNow admin

## Appendix: Manual Component Creation

If the update set import fails, you can manually create components:

### Create Application

```
System Applications > Studio
New Application
Scope: x_snc_claude_mid
Name: Claude Terminal MID Service
```

### Create Tables

```
System Definition > Tables
New
Name: x_snc_claude_mid_terminal_session
Label: Claude Terminal Session
```

Follow the table structure from the "What's Included" section.

### Create REST API

```
System Web Services > Scripted REST APIs
New
Name: Claude Terminal API
API ID: x_snc_claude_mid
Base URI: /api/x_snc_claude_mid/terminal
```

Add operations manually using scripts from `/servicenow/rest-api/` directory.

---

**Version:** 1.0.0
**Last Updated:** 2026-01-24
**Compatibility:** ServiceNow Tokyo+
