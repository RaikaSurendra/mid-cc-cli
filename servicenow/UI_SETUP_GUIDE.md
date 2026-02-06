# ServiceNow UI Setup Guide

## Overview

The Claude Terminal MID Service has **2 Service Portal widgets** that provide the user interface in ServiceNow.

---

## Widgets Included

### 1. Claude Terminal Widget (`claude_terminal`)

**Purpose:** Interactive terminal UI using xterm.js

**Files:**
- `widgets/claude_terminal/widget.html` - UI template
- `widgets/claude_terminal/client_script.js` - AngularJS controller (7,937 bytes)
- `widgets/claude_terminal/styles.scss` - CSS styling (2,269 bytes)

**Features:**
- xterm.js interactive terminal
- Real-time output via AMB notifications
- Adaptive polling (100ms - 5s)
- Session status indicators
- Terminate button
- Responsive design

### 2. Credential Setup Widget (`claude_credential_setup`)

**Purpose:** API key configuration and management

**Files:**
- `widgets/claude_credential_setup/widget.html` - Setup form (4,490 bytes)
- `widgets/claude_credential_setup/client_script.js` - Controller (4,056 bytes)

**Features:**
- API key input (encrypted)
- GitHub token (optional)
- Test connection button
- Credential validation
- Security notices
- Help documentation

---

## How to Create Widgets in ServiceNow

### Step 1: Create Claude Terminal Widget

1. **Navigate to Service Portal**
   ```
   Filter Navigator → "Service Portal"
   Click: Service Portal > Widgets
   ```

2. **Create New Widget**
   - Click **New**
   - Set **Widget ID:** `claude_terminal`
   - Set **Widget Name:** Claude Terminal
   - Set **Data Table:** (leave empty)

3. **Add HTML Template**
   - Copy content from: `servicenow/widgets/claude_terminal/widget.html`
   - Paste into **HTML Template** section

4. **Add Client Controller**
   - Copy content from: `servicenow/widgets/claude_terminal/client_script.js`
   - Paste into **Client Controller** section
   - Set **Controller name:** `ClaudeTerminalController`

5. **Add CSS/SCSS**
   - Copy content from: `servicenow/widgets/claude_terminal/styles.scss`
   - Paste into **SCSS** section

6. **Dependencies**
   - xterm.js will load from CDN (included in client script)

7. **Save the widget**

### Step 2: Create Credential Setup Widget

1. **Create New Widget**
   - Click **New**
   - Set **Widget ID:** `claude_credential_setup`
   - Set **Widget Name:** Claude Credential Setup

2. **Add HTML Template**
   - Copy from: `servicenow/widgets/claude_credential_setup/widget.html`

3. **Add Client Controller**
   - Copy from: `servicenow/widgets/claude_credential_setup/client_script.js`
   - Set **Controller name:** `ClaudeCredentialSetupController`

4. **Save the widget**

### Step 3: Create Portal Pages

#### Page 1: Claude Terminal

1. **Navigate to Pages**
   ```
   Service Portal > Pages
   ```

2. **Create New Page**
   - Click **New**
   - **Page ID:** `claude_terminal`
   - **Title:** Claude Terminal
   - **Short Description:** Interactive Claude Code terminal

3. **Add Widget to Page**
   - In Page Designer
   - Add container (12 columns)
   - Add widget: `claude_terminal`
   - Save and publish

#### Page 2: API Key Setup

1. **Create New Page**
   - **Page ID:** `claude_credential_setup`
   - **Title:** API Key Setup
   - **Short Description:** Configure your Anthropic API key

2. **Add Widget**
   - Add widget: `claude_credential_setup`
   - Save and publish

### Step 4: Add to Portal Navigation

1. **Open Your Portal**
   ```
   Service Portal > Portals
   Select: Employee Center (or your portal)
   ```

2. **Add Menu Items**
   - Click **Menu**
   - Add item:
     - **Title:** Claude Terminal
     - **Type:** Page
     - **Page:** claude_terminal
     - **Icon:** fa-terminal (or preferred)
     - **Order:** 100

   - Add item:
     - **Title:** API Setup
     - **Type:** Page
     - **Page:** claude_credential_setup
     - **Icon:** fa-key
     - **Order:** 101

3. **Save portal configuration**

---

## Widget Details

### Claude Terminal Widget Features

**UI Elements:**
- Terminal header with status indicator
- xterm.js terminal display (80x24)
- Terminate session button
- Loading indicator
- Error handling
- Credential setup link (if no API key)

**Client-Side Logic:**
- Session creation on page load
- AMB subscription for real-time updates
- Adaptive polling (100ms when active, 5s when idle)
- User input handling
- Terminal resize on window resize
- Graceful cleanup on page exit

**Styling:**
- Dark terminal theme (#1e1e1e background)
- Syntax highlighting preserved
- Responsive design (mobile friendly)
- Status indicators with color coding

### Credential Setup Widget Features

**UI Elements:**
- API key input (password field)
- GitHub token input (optional)
- Test connection button
- Save credentials button
- Delete credentials button
- Current status display
- Security notices
- Getting started guide

**Client-Side Logic:**
- Load existing credential info
- Save credentials (encrypted)
- Test API key validation
- Delete credentials
- Success/error messaging

---

## Alternative: UI Policy for Classic UI

If you prefer Classic UI instead of Service Portal:

### Create Form Section

1. **Navigate to:**
   ```
   System UI > UI Policies
   ```

2. **Create UI for Terminal Session Table**
   - Open: `x_snc_claude_mid_terminal_session`
   - Add sections for:
     - Session Information
     - Output Display (read-only)
     - Session Controls

3. **Create UI for Credentials Table**
   - Open: `x_snc_claude_mid_credentials`
   - Add form layout
   - Password fields for API keys

---

## Testing Widgets

### Test Terminal Widget

1. **Navigate to Page**
   ```
   https://your-instance.service-now.com/sp?id=claude_terminal
   ```

2. **Should See:**
   - Terminal loading indicator
   - After ~2-3 seconds: Claude CLI welcome screen
   - Interactive terminal cursor
   - Status showing "Active"

3. **Test Interaction:**
   - Type: `help`
   - Press Enter
   - Should see Claude's help output

### Test Credential Setup Widget

1. **Navigate to Page**
   ```
   https://your-instance.service-now.com/sp?id=claude_credential_setup
   ```

2. **Should See:**
   - API key input field
   - Test connection button
   - Save button
   - Help text

3. **Test:**
   - Enter API key
   - Click "Test Connection"
   - Should validate key
   - Click "Save"
   - Should save to database (encrypted)

---

## Troubleshooting Widgets

### Issue: Widget Not Found

**Solution:**
```
1. Check widget ID matches page reference
2. Verify widget is in correct application scope
3. Clear cache: /?cache.do
4. Impersonate admin and retry
```

### Issue: xterm.js Not Loading

**Solution:**
```
1. Check CDN is accessible
2. Verify client script has CDN URLs
3. Check browser console for errors
4. Try different CDN: unpkg.com or jsdelivr.net
```

### Issue: API Calls Failing

**Solution:**
```
1. Verify REST API is imported and active
2. Check user has proper roles
3. Test REST API directly via REST API Explorer
4. Check browser console for 401/403 errors
```

### Issue: Terminal Shows "Initializing" Forever

**Solution:**
```
1. Check Kubernetes pods are running:
   kubectl get pods -n claude-mid-service

2. Check ECC Queue is being processed
   Navigate to: MID Server > ECC Queue

3. Check MID Server is up
   Navigate to: MID Server > Servers

4. Verify integration user credentials are correct
```

---

## Widget URLs

After setup, users access:

**Main Terminal:**
```
https://your-instance.service-now.com/sp?id=claude_terminal
```

**API Setup:**
```
https://your-instance.service-now.com/sp?id=claude_credential_setup
```

---

## Widget Configuration

### Permissions

Widgets use:
- Current user context (gs.getUserID())
- REST API authentication
- Table ACLs for security

### Dependencies

External:
- xterm.js v5.3.0 (CDN)
- xterm.css (CDN)

Internal:
- AngularJS (Service Portal framework)
- AMB (Asynchronous Message Bus)
- ServiceNow REST API

---

## Quick Setup Checklist

- [ ] Import update set
- [ ] Create claude_terminal widget
- [ ] Create claude_credential_setup widget
- [ ] Create portal pages
- [ ] Add to portal navigation
- [ ] Configure user roles
- [ ] Test both widgets
- [ ] Verify API connectivity

---

**Next:** Import the update set, then create these widgets in ServiceNow!
