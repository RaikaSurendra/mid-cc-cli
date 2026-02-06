# Widget Access Troubleshooting Guide

## Common Issues & Solutions

### Issue 1: "Page not found" or "Widget does not exist"

**Cause:** Widgets need to be created manually in ServiceNow (they're not in the update set)

**Solution: Create Widgets Manually**

#### Step-by-Step: Create Claude Terminal Widget

1. **Login to ServiceNow**
   ```
   https://your-instance.service-now.com
   ```

2. **Navigate to Widgets**
   ```
   Filter Navigator → Type "widget"
   Click: Service Portal > Widgets
   ```

3. **Create New Widget**
   - Click **New** button (top right)

4. **Fill Widget Form:**
   ```
   Widget Name: Claude Terminal
   Widget ID: claude_terminal
   Description: Interactive Claude Code terminal interface
   Data table: (leave empty)
   Application: x_snc_claude_mid (if available, else leave as global)
   ```

5. **HTML Template Tab:**
   - Click **HTML Template** tab
   - Delete any default content
   - Copy entire content from: `servicenow/widgets/claude_terminal/widget.html`
   - Paste it in

6. **Client Script Tab:**
   - Click **Client Script** tab
   - Delete any default content
   - Copy entire content from: `servicenow/widgets/claude_terminal/client_script.js`
   - Paste it in

7. **CSS Tab:**
   - Click **CSS - SCSS** tab
   - Delete any default content
   - Copy entire content from: `servicenow/widgets/claude_terminal/styles.scss`
   - Paste it in

8. **Server Script Tab:**
   - Leave empty (no server script needed)

9. **Save the Widget**
   - Click **Submit** or **Update**

#### Step-by-Step: Create Credential Setup Widget

Repeat the same process:
1. Click **New**
2. **Widget ID:** `claude_credential_setup`
3. **Widget Name:** Claude Credential Setup
4. Copy HTML from: `servicenow/widgets/claude_credential_setup/widget.html`
5. Copy Client Script from: `servicenow/widgets/claude_credential_setup/client_script.js`
6. (No CSS for this widget)
7. Save

### Issue 2: "Access Denied" or "Insufficient Permissions"

**Solution: Check User Roles**

1. **Navigate to User Profile**
   ```
   User Menu > Impersonate User > Your User
   ```

2. **Check Roles:**
   - Need: `sp_admin` or `admin`
   - Or: Portal-specific roles

3. **Grant Roles if Missing:**
   ```
   User Administration > Users
   Find your user
   Related Lists > Roles
   Add: sp_admin
   ```

### Issue 3: Widget Created but Page Shows Error

**Solution: Create Portal Page**

1. **Navigate to Pages**
   ```
   Service Portal > Pages
   ```

2. **Create Claude Terminal Page:**
   - Click **New**
   - **Page ID:** `claude_terminal`
   - **Title:** Claude Terminal
   - **Short description:** Interactive Claude Code terminal

3. **Add Widget to Page:**
   - In the **Page** record, look for **Containers** section
   - Or open **Designer** view
   - Add a **Container** (12 columns)
   - Add **Widget:** claude_terminal
   - Save

4. **Make Page Public:**
   - Check **Public** checkbox if you want non-logged-in access
   - Or configure page roles

### Issue 4: "REST API not found" Error in Widget

**Solution: Verify REST API is Created**

1. **Check if API Exists:**
   ```
   System Web Services > Scripted REST APIs
   Search: "Claude Terminal API"
   ```

2. **If Not Found, Create It:**

   **Create REST API Resource:**
   - Click **New**
   - **Name:** Claude Terminal API
   - **API ID:** terminal
   - **Namespace:** x_snc_claude_mid
   - **Base path:** /api/x_snc_claude_mid/terminal
   - **Active:** true

   **Add Operations** (see below)

3. **Create Each Operation:**

   **Operation: Create Session**
   ```
   Name: Create Session
   HTTP method: POST
   Relative path: /session/create
   ```

   Script: (Copy from `servicenow/rest-api/claude_terminal_api.xml` - the createSession function)

   **Operation: Send Command**
   ```
   HTTP method: POST
   Relative path: /session/{sessionId}/command
   ```

   **Operation: Get Output**
   ```
   HTTP method: GET
   Relative path: /session/{sessionId}/output
   ```

   **Operation: Get Status**
   ```
   HTTP method: GET
   Relative path: /session/{sessionId}/status
   ```

   **Operation: Terminate**
   ```
   HTTP method: DELETE
   Relative path: /session/{sessionId}
   ```

   **Operation: Resize**
   ```
   HTTP method: POST
   Relative path: /session/{sessionId}/resize
   ```

### Issue 5: Tables Not Found

**Solution: Verify Update Set Imported**

1. **Check Tables Exist:**
   ```
   System Definition > Tables
   Search: x_snc_claude_mid
   ```

2. **Should See:**
   - `x_snc_claude_mid_terminal_session`
   - `x_snc_claude_mid_credentials`

3. **If Missing:**
   - Re-import update set
   - Check for import errors
   - Verify update set was committed (not just previewed)

---

## Quick Diagnostic

### Run This Checklist:

```
1. □ Update set imported and committed?
   Check: System Update Sets > Retrieved Update Sets

2. □ Tables exist?
   Check: x_snc_claude_mid_terminal_session.list

3. □ Widgets created?
   Check: Service Portal > Widgets
   Search: claude_terminal

4. □ Pages created?
   Check: Service Portal > Pages
   Search: claude_terminal

5. □ REST API created?
   Check: System Web Services > Scripted REST APIs

6. □ User has roles?
   Check: User profile > Roles
   Need: sp_admin or admin

7. □ Portal configured?
   Check: Service Portal > Portals
   Menu should have Claude Terminal item
```

---

## Alternative: Quick Test URL

Try accessing the widget directly with parameters:

```
https://your-instance.service-now.com/sp?id=widget&widget=claude_terminal
```

If this shows "Widget not found", the widget doesn't exist yet.

---

## Step-by-Step Widget Creation (Simplified)

### 1. Open ServiceNow Studio (Easier Method)

```
Filter Navigator → "Studio"
Click: System Applications > Studio
Select: x_snc_claude_mid (if exists) or Create New
```

### 2. Create Widget in Studio

```
Create Application File > Service Portal > Widget
Widget Name: Claude Terminal
Widget ID: claude_terminal
```

Then paste the code from the files.

---

## What URL Are You Trying to Access?

**Common URLs:**

1. **Service Portal Widget Page:**
   ```
   https://your-instance.service-now.com/sp?id=claude_terminal
   ```

2. **Direct Widget Test:**
   ```
   https://your-instance.service-now.com/sp?id=widget&widget=claude_terminal
   ```

3. **Widget List (to verify it exists):**
   ```
   https://your-instance.service-now.com/sp_widget_list.do
   ```

4. **Classic UI (table view):**
   ```
   https://your-instance.service-now.com/x_snc_claude_mid_terminal_session_list.do
   ```

---

## Quick Fix: Create Simple Test Widget First

### Test if Service Portal is Working:

1. **Create Test Widget:**
   ```
   Service Portal > Widgets > New
   Widget ID: test_widget
   HTML: <h1>Test Widget Works!</h1>
   ```

2. **Access It:**
   ```
   https://your-instance.service-now.com/sp?id=widget&widget=test_widget
   ```

3. **If This Works:**
   - Service Portal is working
   - Problem is with the Claude widgets specifically
   - Proceed to create Claude widgets

4. **If This Doesn't Work:**
   - Service Portal might not be configured
   - Check if Service Portal is enabled on your instance
   - May need to enable Service Portal plugin

---

## Tell Me:

**What exactly are you seeing?**

1. Error message?
2. Blank page?
3. "Page not found"?
4. "Widget does not exist"?
5. Login prompt?
6. Something else?

**What URL are you trying?**

Once you tell me, I can give you the exact solution!
