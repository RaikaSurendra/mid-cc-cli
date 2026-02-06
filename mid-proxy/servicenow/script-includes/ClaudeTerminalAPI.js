// =============================================================================
// ClaudeTerminalAPI — ServiceNow Script Include (Instance-side)
// =============================================================================
// Creates JavascriptProbe payloads and inserts them into the ECC Queue for
// the MID Server to pick up.  The MID Server runs ClaudeTerminalProbe which
// makes the actual HTTP call to the Claude Terminal Service.
//
// Usage (from Scripted REST, Business Rule, or Flow):
//   var api = new ClaudeTerminalAPI();
//   var result = api.createSessionSync(userId, credentials);
//
// System Properties required:
//   x_claude.terminal.mid_server   — MID Server name (e.g. "mid-docker-host")
//   x_claude.terminal.service_url  — Service base URL (e.g. "http://claude-terminal-service:3000")
//   x_claude.terminal.auth_token   — Bearer token for the HTTP service
// =============================================================================

var ClaudeTerminalAPI = Class.create();
ClaudeTerminalAPI.prototype = {
    type: 'ClaudeTerminalAPI',

    initialize: function() {
        this.midServer  = gs.getProperty('x_claude.terminal.mid_server', '');
        this.serviceUrl = gs.getProperty('x_claude.terminal.service_url', 'http://claude-terminal-service:3000');
        this.authToken  = gs.getProperty('x_claude.terminal.auth_token', '');
        this.TIMEOUT_MS = 15000; // max wait for MID Server response
        this.POLL_MS    = 500;   // polling interval
    },

    // ── Async methods (fire-and-forget into ECC Queue) ──────────────────

    /**
     * Create a new Claude Code terminal session.
     * @param {string} userId
     * @param {object} credentials  { anthropicApiKey, githubToken }
     * @param {string} [workspaceType]
     * @returns {string} ECC Queue sys_id for tracking
     */
    createSession: function(userId, credentials, workspaceType) {
        return this._createProbe('create_session', {
            userId: userId,
            credentials: credentials,
            workspaceType: workspaceType || 'temp'
        });
    },

    /**
     * Send a command to an existing session.
     * @param {string} sessionId
     * @param {string} command
     * @returns {string} ECC Queue sys_id
     */
    sendCommand: function(sessionId, command) {
        return this._createProbe('send_command', {
            sessionId: sessionId,
            command: command
        });
    },

    /**
     * Get terminal output from a session.
     * @param {string} sessionId
     * @param {boolean} [clear=false]
     * @returns {string} ECC Queue sys_id
     */
    getOutput: function(sessionId, clear) {
        return this._createProbe('get_output', {
            sessionId: sessionId,
            clear: !!clear
        });
    },

    /**
     * Get session status.
     * @param {string} sessionId
     * @returns {string} ECC Queue sys_id
     */
    getStatus: function(sessionId) {
        return this._createProbe('get_status', {
            sessionId: sessionId
        });
    },

    /**
     * Terminate a session.
     * @param {string} sessionId
     * @returns {string} ECC Queue sys_id
     */
    terminateSession: function(sessionId) {
        return this._createProbe('terminate_session', {
            sessionId: sessionId
        });
    },

    /**
     * Resize the terminal.
     * @param {string} sessionId
     * @param {number} cols
     * @param {number} rows
     * @returns {string} ECC Queue sys_id
     */
    resizeTerminal: function(sessionId, cols, rows) {
        return this._createProbe('resize_terminal', {
            sessionId: sessionId,
            cols: cols,
            rows: rows
        });
    },

    // ── Synchronous wrappers (block until MID Server responds) ──────────

    createSessionSync: function(userId, credentials, workspaceType) {
        var eccSysId = this.createSession(userId, credentials, workspaceType);
        return this._waitForResponse(eccSysId);
    },

    sendCommandSync: function(sessionId, command) {
        var eccSysId = this.sendCommand(sessionId, command);
        return this._waitForResponse(eccSysId);
    },

    getOutputSync: function(sessionId, clear) {
        var eccSysId = this.getOutput(sessionId, clear);
        return this._waitForResponse(eccSysId);
    },

    getStatusSync: function(sessionId) {
        var eccSysId = this.getStatus(sessionId);
        return this._waitForResponse(eccSysId);
    },

    terminateSessionSync: function(sessionId) {
        var eccSysId = this.terminateSession(sessionId);
        return this._waitForResponse(eccSysId);
    },

    resizeTerminalSync: function(sessionId, cols, rows) {
        var eccSysId = this.resizeTerminal(sessionId, cols, rows);
        return this._waitForResponse(eccSysId);
    },

    // ── Internal helpers ────────────────────────────────────────────────

    /**
     * Create a JavascriptProbe ECC Queue entry for the MID Server.
     * @param {string} action  — one of the 6 supported actions
     * @param {object} params  — action-specific parameters
     * @returns {string} sys_id of the created ECC Queue record
     * @private
     */
    _createProbe: function(action, params) {
        if (!this.midServer) {
            throw new Error('x_claude.terminal.mid_server property is not configured');
        }

        var probe = new JavascriptProbe(this.midServer);
        probe.setName('ClaudeTerminalProbe');
        probe.setJavascript(
            'var probe = new ClaudeTerminalProbe();' +
            'probe.execute();'
        );

        // Pass parameters to the MID Server probe
        probe.addParameter('action',      action);
        probe.addParameter('params',      JSON.stringify(params));
        probe.addParameter('service_url', this.serviceUrl);
        probe.addParameter('auth_token',  this.authToken);

        var eccSysId = probe.create();

        gs.debug('ClaudeTerminalAPI: Created probe for action={0}, ecc_sys_id={1}',
            action, eccSysId);

        return eccSysId;
    },

    /**
     * Poll the ECC Queue output for a response from the MID Server.
     * Blocks until the response arrives or timeout is reached.
     * @param {string} eccSysId  — sys_id of the original ECC Queue input record
     * @returns {object} parsed JSON result from the MID Server
     * @private
     */
    _waitForResponse: function(eccSysId) {
        var elapsed = 0;

        while (elapsed < this.TIMEOUT_MS) {
            var gr = new GlideRecord('ecc_queue');
            gr.addQuery('queue', 'output');
            gr.addQuery('response_to', eccSysId);
            gr.addQuery('state', 'processed');
            gr.setLimit(1);
            gr.query();

            if (gr.next()) {
                var output = gr.getValue('output');
                try {
                    return JSON.parse(output);
                } catch (e) {
                    return { success: false, error: 'Failed to parse MID response: ' + e.message, raw: output };
                }
            }

            // Also check for errors
            var errGr = new GlideRecord('ecc_queue');
            errGr.addQuery('queue', 'output');
            errGr.addQuery('response_to', eccSysId);
            errGr.addQuery('state', 'error');
            errGr.setLimit(1);
            errGr.query();

            if (errGr.next()) {
                return { success: false, error: errGr.getValue('output') || 'MID Server probe failed' };
            }

            gs.sleep(this.POLL_MS);
            elapsed += this.POLL_MS;
        }

        return { success: false, error: 'Timeout waiting for MID Server response after ' + this.TIMEOUT_MS + 'ms' };
    }
};
