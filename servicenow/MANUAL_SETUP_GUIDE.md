# Manual Setup Guide for ServiceNow

## Quick Setup (15 minutes)

Since ServiceNow update sets with complex components can be tricky, here's the **simple manual approach** that works every time.

---

## Step 1: Import Core Components (Tables)

**File:** Use `Claude-Terminal-UpdateSet-Valid.xml` or create manually

### Option A: Import Update Set (Tables Only)

1. Login: https://your-instance.service-now.com
2. Navigate: **Retrieved Update Sets**
3. Import: `Claude-Terminal-UpdateSet-Valid.xml`
4. Preview and Commit

### Option B: Create Tables Manually (5 minutes)

#### Table 1: Terminal Sessions

1. Navigate: **System Definition > Tables**
2. Click **New**
3. Set:
   ```
   Label: Claude Terminal Session
   Name: x_snc_claude_mid_terminal_session
   Extends table: (leave empty)
   Application: x_snc_claude_mid (create if needed)
   ```

4. Add Columns:

   **session_id** (String, 40, Unique, Display, Mandatory)

   **user** (Reference to sys_user, Mandatory)

   **status** (Choice, 40, Mandatory, Default: initializing)
   - Choices: initializing, active, idle, terminated, error

   **output_buffer** (JSON, 65000)
   - Attributes: edge_encryption_enabled=true

   **workspace_path** (String, 255)

   **workspace_type** (Choice, 40, Default: isolated)
   - Choices: isolated, persistent

   **last_activity** (Date/Time, Mandatory)
   - Default: javascript:gs.nowDateTime()

   **mid_server** (String, 100)

   **error_message** (String, 1000)

#### Table 2: Credentials

1. Click **New** (create another table)
2. Set:
   ```
   Label: Claude Credentials
   Name: x_snc_claude_mid_credentials
   ```

3. Add Columns:

   **user** (Reference to sys_user, Unique, Display, Mandatory)

   **anthropic_api_key** (Password (2 Way Encrypted), 255, Mandatory)
   - Attributes: edge_encryption_enabled=true

   **github_token** (Password (2 Way Encrypted), 255)

   **last_used** (Date/Time)

   **last_validated** (Date/Time)

   **validation_status** (Choice, 40, Default: untested)
   - Choices: valid, invalid, untested

---

## Step 2: Create Script Include (2 minutes)

1. Navigate: **System Definition > Script Includes**
2. Click **New**
3. Set:
   ```
   Name: ClaudeTerminalAPI
   API Name: ClaudeTerminalAPI
   Application: x_snc_claude_mid
   Client callable: false
   ```

4. **Script:** Copy this complete code:

```javascript
var ClaudeTerminalAPI = Class.create();
ClaudeTerminalAPI.prototype = {
    initialize: function() {
        this.TABLE_SESSION = 'x_snc_claude_mid_terminal_session';
        this.TABLE_CREDENTIALS = 'x_snc_claude_mid_credentials';
        this.ECC_TOPIC_COMMAND = 'ClaudeTerminalCommand';
    },

    createSession: function(userId, workspaceType) {
        try {
            var credentials = this._getUserCredentials(userId);
            if (!credentials) {
                return { success: false, error: 'Credentials not found' };
            }

            var sessionGR = new GlideRecord(this.TABLE_SESSION);
            sessionGR.initialize();
            sessionGR.session_id = gs.generateGUID();
            sessionGR.user = userId;
            sessionGR.status = 'initializing';
            sessionGR.workspace_type = workspaceType || 'isolated';
            sessionGR.last_activity = new GlideDateTime();
            sessionGR.insert();

            var sessionId = sessionGR.session_id.toString();

            var payload = {
                action: 'create_session',
                sessionId: sessionId,
                userId: userId,
                credentials: {
                    anthropicApiKey: credentials.anthropic_api_key,
                    githubToken: credentials.github_token
                },
                workspaceType: workspaceType
            };

            this._sendToECCQueue(payload, sessionId);

            return { success: true, sessionId: sessionId, status: 'initializing' };
        } catch (e) {
            gs.error('ClaudeTerminalAPI.createSession: ' + e.message);
            return { success: false, error: e.message };
        }
    },

    sendCommand: function(sessionId, command) {
        try {
            var sessionGR = this._getSession(sessionId);
            if (!sessionGR) {
                return { success: false, error: 'Session not found' };
            }

            var payload = {
                action: 'send_command',
                sessionId: sessionId,
                command: command
            };

            this._sendToECCQueue(payload, sessionId);
            sessionGR.last_activity = new GlideDateTime();
            sessionGR.update();

            return { success: true };
        } catch (e) {
            gs.error('ClaudeTerminalAPI.sendCommand: ' + e.message);
            return { success: false, error: e.message };
        }
    },

    getOutput: function(sessionId, clear) {
        try {
            var sessionGR = this._getSession(sessionId);
            if (!sessionGR) {
                return { success: false, error: 'Session not found' };
            }

            var output = [];
            if (sessionGR.output_buffer) {
                try {
                    output = JSON.parse(sessionGR.output_buffer.toString());
                } catch (e) {
                    gs.warn('Failed to parse output buffer: ' + e.message);
                }
            }

            if (clear) {
                sessionGR.output_buffer = '[]';
                sessionGR.update();
            }

            return {
                success: true,
                sessionId: sessionId,
                output: output,
                status: sessionGR.status.toString()
            };
        } catch (e) {
            gs.error('ClaudeTerminalAPI.getOutput: ' + e.message);
            return { success: false, error: e.message };
        }
    },

    getStatus: function(sessionId) {
        try {
            var sessionGR = this._getSession(sessionId);
            if (!sessionGR) {
                return { success: false, error: 'Session not found' };
            }

            return {
                success: true,
                sessionId: sessionId,
                status: sessionGR.status.toString(),
                userId: sessionGR.user.toString(),
                workspacePath: sessionGR.workspace_path.toString(),
                lastActivity: sessionGR.last_activity.toString()
            };
        } catch (e) {
            gs.error('ClaudeTerminalAPI.getStatus: ' + e.message);
            return { success: false, error: e.message };
        }
    },

    terminateSession: function(sessionId) {
        try {
            var sessionGR = this._getSession(sessionId);
            if (!sessionGR) {
                return { success: false, error: 'Session not found' };
            }

            var payload = {
                action: 'terminate_session',
                sessionId: sessionId
            };

            this._sendToECCQueue(payload, sessionId);
            sessionGR.status = 'terminated';
            sessionGR.update();

            return { success: true, message: 'Session terminated' };
        } catch (e) {
            gs.error('ClaudeTerminalAPI.terminateSession: ' + e.message);
            return { success: false, error: e.message };
        }
    },

    resizeTerminal: function(sessionId, cols, rows) {
        try {
            var payload = {
                action: 'resize_terminal',
                sessionId: sessionId,
                cols: parseInt(cols),
                rows: parseInt(rows)
            };

            this._sendToECCQueue(payload, sessionId);
            return { success: true };
        } catch (e) {
            gs.error('ClaudeTerminalAPI.resizeTerminal: ' + e.message);
            return { success: false, error: e.message };
        }
    },

    _getSession: function(sessionId) {
        var gr = new GlideRecord(this.TABLE_SESSION);
        gr.addQuery('session_id', sessionId);
        gr.query();
        return gr.next() ? gr : null;
    },

    _getUserCredentials: function(userId) {
        var gr = new GlideRecord(this.TABLE_CREDENTIALS);
        gr.addQuery('user', userId);
        gr.query();
        if (gr.next()) {
            return {
                anthropic_api_key: gr.anthropic_api_key.getDecryptedValue(),
                github_token: gr.github_token.getDecryptedValue()
            };
        }
        return null;
    },

    _sendToECCQueue: function(payload, sessionId) {
        var ecc = new GlideRecord('ecc_queue');
        ecc.initialize();
        ecc.topic = this.ECC_TOPIC_COMMAND;
        ecc.name = 'Claude Terminal Command: ' + sessionId;
        ecc.queue = 'input';
        ecc.state = 'ready';
        ecc.payload = JSON.stringify(payload);
        ecc.insert();
    },

    type: 'ClaudeTerminalAPI'
};
```

5. Save

---

## Step 3: Create REST API (5 minutes)

1. Navigate: **System Web Services > Scripted REST APIs**
2. Click **New**
3. Set:
   ```
   Name: Claude Terminal API
   API ID: terminal
   Namespace: x_snc_claude_mid
   Base path: /api/x_snc_claude_mid/terminal
   Active: true
   Application: x_snc_claude_mid
   ```
4. Click **Submit**

### Add REST Operations:

#### Operation 1: Create Session

1. Open the API you just created
2. Go to **Resources** tab
3. Click **New** (under Resources)
4. Set:
   ```
   HTTP method: POST
   Name: Create Session
   Relative path: /session/create
   ```

5. **Script:**

```javascript
(function process(request, response) {
    var api = new x_snc_claude_mid.ClaudeTerminalAPI();
    var userId = gs.getUserID();
    var data = request.body.data;
    var workspaceType = data.workspaceType || 'isolated';

    var result = api.createSession(userId, workspaceType);

    response.setStatus(result.success ? 200 : 500);
    response.setBody(result);
})(request, response);
```

6. Save

#### Operation 2: Send Command

1. Click **New** (add another operation)
2. Set:
   ```
   HTTP method: POST
   Name: Send Command
   Relative path: /session/{sessionId}/command
   ```

3. **Script:**

```javascript
(function process(request, response) {
    var api = new x_snc_claude_mid.ClaudeTerminalAPI();
    var sessionId = request.pathParams.sessionId;
    var command = request.body.data.command;

    var result = api.sendCommand(sessionId, command);

    response.setStatus(result.success ? 200 : 500);
    response.setBody(result);
})(request, response);
```

#### Operation 3: Get Output

```
HTTP method: GET
Name: Get Output
Relative path: /session/{sessionId}/output
```

**Script:**
```javascript
(function process(request, response) {
    var api = new x_snc_claude_mid.ClaudeTerminalAPI();
    var sessionId = request.pathParams.sessionId;
    var clear = request.queryParams.clear === 'true';

    var result = api.getOutput(sessionId, clear);

    response.setStatus(result.success ? 200 : 404);
    response.setBody(result);
})(request, response);
```

#### Operation 4: Get Status

```
HTTP method: GET
Name: Get Status
Relative path: /session/{sessionId}/status
```

**Script:**
```javascript
(function process(request, response) {
    var api = new x_snc_claude_mid.ClaudeTerminalAPI();
    var sessionId = request.pathParams.sessionId;

    var result = api.getStatus(sessionId);

    response.setStatus(result.success ? 200 : 404);
    response.setBody(result);
})(request, response);
```

#### Operation 5: Terminate Session

```
HTTP method: DELETE
Name: Terminate Session
Relative path: /session/{sessionId}
```

**Script:**
```javascript
(function process(request, response) {
    var api = new x_snc_claude_mid.ClaudeTerminalAPI();
    var sessionId = request.pathParams.sessionId;

    var result = api.terminateSession(sessionId);

    response.setStatus(result.success ? 200 : 404);
    response.setBody(result);
})(request, response);
```

#### Operation 6: Resize Terminal

```
HTTP method: POST
Name: Resize Terminal
Relative path: /session/{sessionId}/resize
```

**Script:**
```javascript
(function process(request, response) {
    var api = new x_snc_claude_mid.ClaudeTerminalAPI();
    var sessionId = request.pathParams.sessionId;
    var data = request.body.data;

    var result = api.resizeTerminal(sessionId, data.cols, data.rows);

    response.setStatus(result.success ? 200 : 500);
    response.setBody(result);
})(request, response);
```

---

## Step 4: Create Business Rule (2 minutes)

1. Navigate: **System Definition > Business Rules**
2. Click **New**
3. Set:
   ```
   Name: AMB Output Notification
   Table: x_snc_claude_mid_terminal_session
   Application: x_snc_claude_mid
   Active: true
   Advanced: true
   When: after
   Insert: false
   Update: true
   Delete: false
   ```

4. **Condition:**
   ```
   output_buffer
   changes
   ```

5. **Script:**

```javascript
(function executeRule(current, previous) {
    try {
        if (!current.output_buffer || current.output_buffer.toString() === '[]') {
            return;
        }

        var sessionId = current.session_id.toString();
        var channel = 'claude.terminal.' + sessionId;

        var message = {
            sessionId: sessionId,
            status: current.status.toString(),
            timestamp: new GlideDateTime().toString(),
            hasOutput: true
        };

        sn_amb.AmbulatoryMessageBus.publish(channel, JSON.stringify(message));
        gs.debug('AMB notification published to: ' + channel);
    } catch (e) {
        gs.error('AMB Output Notification error: ' + e.message);
    }
})(current, previous);
```

6. Save

---

## Step 5: Create Widgets (3 minutes each)

### Widget 1: Claude Terminal

1. Navigate: **Service Portal > Widgets**
2. Click **New**
3. Set:
   ```
   Widget Name: Claude Terminal
   Widget ID: claude_terminal
   Description: Interactive Claude Code terminal
   Application: x_snc_claude_mid
   ```

4. **HTML Template:**

```html
<div class="claude-terminal-container">
  <div class="terminal-header">
    <h3>Claude Code Terminal</h3>
    <div class="terminal-controls">
      <span class="status-indicator" ng-class="c.data.statusClass">
        {{c.data.status}}
      </span>
      <button class="btn btn-sm btn-danger" ng-click="c.terminateSession()">
        <i class="fa fa-power-off"></i> Terminate
      </button>
    </div>
  </div>

  <div class="terminal-wrapper" ng-if="c.data.sessionId">
    <div id="terminal-{{c.data.sessionId}}" class="terminal-display"></div>
  </div>

  <div class="terminal-loading" ng-if="!c.data.sessionId && !c.data.error">
    <i class="fa fa-spinner fa-spin"></i> Initializing Claude Code terminal...
  </div>

  <div class="terminal-error" ng-if="c.data.error">
    <div class="alert alert-danger">
      <i class="fa fa-exclamation-triangle"></i>
      <strong>Error:</strong> {{c.data.error}}
      <br>
      <button class="btn btn-sm btn-primary" ng-click="c.retry()">
        <i class="fa fa-refresh"></i> Retry
      </button>
    </div>
  </div>
</div>
```

5. **Client Controller:**

Copy from: `servicenow/widgets/claude_terminal/client_script.js`
Or use this simplified version:

```javascript
function($scope, $http, $timeout, spUtil) {
  var c = this;
  var terminal = null;
  var sessionId = null;

  // Load xterm.js
  if (typeof Terminal === 'undefined') {
    var script = document.createElement('script');
    script.src = 'https://cdn.jsdelivr.net/npm/xterm@5.3.0/lib/xterm.min.js';
    document.head.appendChild(script);

    var css = document.createElement('link');
    css.rel = 'stylesheet';
    css.href = 'https://cdn.jsdelivr.net/npm/xterm@5.3.0/css/xterm.css';
    document.head.appendChild(css);
  }

  c.data = {
    sessionId: null,
    status: 'initializing',
    error: null
  };

  function createSession() {
    $http.post('/api/x_snc_claude_mid/terminal/session/create', {
      workspaceType: 'isolated'
    }).then(function(response) {
      if (response.data.success) {
        sessionId = response.data.sessionId;
        c.data.sessionId = sessionId;
        c.data.status = response.data.status;
        initTerminal();
        startPolling();
      }
    });
  }

  function initTerminal() {
    $timeout(function() {
      terminal = new Terminal({
        cursorBlink: true,
        fontSize: 14
      });
      terminal.open(document.getElementById('terminal-' + sessionId));
      terminal.onData(function(data) {
        $http.post('/api/x_snc_claude_mid/terminal/session/' + sessionId + '/command', {
          command: data
        });
      });
    }, 500);
  }

  function startPolling() {
    $timeout(function poll() {
      $http.get('/api/x_snc_claude_mid/terminal/session/' + sessionId + '/output?clear=true')
        .then(function(response) {
          if (response.data.success && response.data.output) {
            response.data.output.forEach(function(chunk) {
              if (terminal) terminal.write(chunk.data);
            });
          }
          $timeout(poll, 1000);
        });
    }, 1000);
  }

  c.terminateSession = function() {
    $http.delete('/api/x_snc_claude_mid/terminal/session/' + sessionId);
  };

  createSession();
}
```

6. **CSS/SCSS:**

```scss
.claude-terminal-container {
  background: #1e1e1e;
  border-radius: 8px;
  padding: 0;

  .terminal-header {
    background: #2d2d30;
    padding: 12px 20px;
    color: #d4d4d4;
    display: flex;
    justify-content: space-between;
  }

  .terminal-display {
    padding: 10px;
    height: 600px;
  }

  .status-indicator {
    padding: 4px 12px;
    border-radius: 12px;
    &.status-active { background: #1e3a1e; color: #4ec9b0; }
    &.status-error { background: #5a1e1e; color: #f48771; }
  }
}
```

7. Save Widget

### Widget 2: Credential Setup

1. Click **New** (create another widget)
2. Set:
   ```
   Widget ID: claude_credential_setup
   Widget Name: Claude Credential Setup
   ```

3. Copy HTML and JS from: `servicenow/widgets/claude_credential_setup/`

4. Save Widget

---

## Step 6: Create Portal Pages (2 minutes)

1. Navigate: **Service Portal > Pages**

2. **Create Terminal Page:**
   ```
   Title: Claude Terminal
   Page ID: claude_terminal
   ```
   - Add widget: `claude_terminal`
   - Save

3. **Create Setup Page:**
   ```
   Title: API Key Setup
   Page ID: claude_credential_setup
   ```
   - Add widget: `claude_credential_setup`
   - Save

---

## Step 7: Add to Portal Navigation

1. Navigate: **Service Portal > Portals**
2. Select your portal
3. Edit **Menu**
4. Add:
   - Claude Terminal → `/sp?id=claude_terminal`
   - API Setup → `/sp?id=claude_credential_setup`

---

## Total Time: ~15 minutes

**vs. Troubleshooting XML import issues: Hours**

This manual approach is:
- ✅ Faster
- ✅ More reliable
- ✅ Easier to debug
- ✅ You understand what you're creating

---

## Quick Test After Setup

1. Go to: `/sp?id=claude_credential_setup`
2. Enter API key
3. Save
4. Go to: `/sp?id=claude_terminal`
5. Terminal should initialize!

---

**See also:**
- `servicenow/UPDATE-SET-README.md` - Detailed guide
- `WIDGET_TROUBLESHOOTING.md` - If issues occur
