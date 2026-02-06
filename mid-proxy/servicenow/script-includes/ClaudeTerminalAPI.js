/**
 * ClaudeTerminalAPI - Server-side Script Include
 *
 * Runs ON the ServiceNow instance. Triggers MID Server probes
 * to communicate with the Claude Terminal HTTP Service.
 *
 * This replaces the direct ECC Queue manipulation used in the
 * original setup. Instead, it uses the standard JavascriptProbe
 * framework to route commands through the MID Server.
 *
 * ServiceNow Setup:
 *   1. Create Script Include:
 *      - Name: ClaudeTerminalAPI
 *      - Client callable: false
 *      - Active: true
 *      - Script: (paste this file)
 *
 *   2. System Properties (sys_properties):
 *      - x_claude.terminal.mid_server  = k8s-mid-proxy-01
 *      - x_claude.terminal.service_url = http://claude-terminal-service:3000
 *      - x_claude.terminal.auth_token  = mid-llm-cli-dev-token-2026
 *
 * Usage from Server Scripts / Business Rules:
 *   var api = new ClaudeTerminalAPI();
 *   var result = api.createSession('john.doe', 'sk-ant-...', '', 'isolated');
 *   var sessionId = result.sessionId;
 *   api.sendCommand(sessionId, 'john.doe', 'help\n');
 *   var output = api.getOutput(sessionId, 'john.doe', true);
 */

var ClaudeTerminalAPI = Class.create();
ClaudeTerminalAPI.prototype = {

    initialize: function() {
        this.midServerName = gs.getProperty('x_claude.terminal.mid_server', 'k8s-mid-proxy-01');
        this.serviceURL = gs.getProperty('x_claude.terminal.service_url', 'http://claude-terminal-service:3000');
        this.authToken = gs.getProperty('x_claude.terminal.auth_token', '');
    },

    /**
     * Create a new Claude terminal session.
     *
     * @param {string} userId - ServiceNow user name
     * @param {string} anthropicApiKey - Anthropic API key
     * @param {string} githubToken - GitHub token (optional)
     * @param {string} workspaceType - 'isolated' or 'persistent'
     * @returns {object} - { sessionId, status, workspacePath } or error
     */
    createSession: function(userId, anthropicApiKey, githubToken, workspaceType) {
        var payload = JSON.stringify({
            userId: userId,
            credentials: {
                anthropicApiKey: anthropicApiKey,
                githubToken: githubToken || ''
            },
            workspaceType: workspaceType || 'isolated'
        });

        return this._executeProbe('create_session', '', userId, payload);
    },

    /**
     * Send a command to an existing session.
     *
     * @param {string} sessionId - Session UUID
     * @param {string} userId - Session owner
     * @param {string} command - Command to execute
     * @returns {object} - { success: true } or error
     */
    sendCommand: function(sessionId, userId, command) {
        var payload = JSON.stringify({ command: command });
        return this._executeProbe('send_command', sessionId, userId, payload);
    },

    /**
     * Get output from a session.
     *
     * @param {string} sessionId - Session UUID
     * @param {string} userId - Session owner
     * @param {boolean} clear - Whether to clear the buffer after reading
     * @returns {object} - { sessionId, output: [...], status }
     */
    getOutput: function(sessionId, userId, clear) {
        var payload = JSON.stringify({ clear: clear === true });
        return this._executeProbe('get_output', sessionId, userId, payload);
    },

    /**
     * Get status of a session.
     *
     * @param {string} sessionId - Session UUID
     * @param {string} userId - Session owner
     * @returns {object} - { sessionId, userId, status, ... }
     */
    getStatus: function(sessionId, userId) {
        return this._executeProbe('get_status', sessionId, userId, '{}');
    },

    /**
     * Terminate a session.
     *
     * @param {string} sessionId - Session UUID
     * @param {string} userId - Session owner
     * @returns {object} - { message: "session terminated" }
     */
    terminateSession: function(sessionId, userId) {
        return this._executeProbe('terminate_session', sessionId, userId, '{}');
    },

    /**
     * Resize terminal dimensions.
     *
     * @param {string} sessionId - Session UUID
     * @param {string} userId - Session owner
     * @param {number} cols - Number of columns
     * @param {number} rows - Number of rows
     * @returns {object} - { success: true }
     */
    resizeTerminal: function(sessionId, userId, cols, rows) {
        var payload = JSON.stringify({ cols: cols, rows: rows });
        return this._executeProbe('resize_terminal', sessionId, userId, payload);
    },

    /**
     * Execute a JavascriptProbe on the MID Server.
     * This writes to the ECC Queue and waits for the MID Server to pick it up,
     * execute ClaudeTerminalProbe, and return the result.
     *
     * @param {string} action - The action to perform
     * @param {string} sessionId - Session UUID (empty for create)
     * @param {string} userId - ServiceNow user
     * @param {string} payload - JSON payload string
     * @returns {object} - Parsed response from the probe
     */
    _executeProbe: function(action, sessionId, userId, payload) {
        var probe = new JavascriptProbe(this.midServerName);
        probe.setName('ClaudeTerminalProbe');

        // Set probe parameters (these become available via probe.getParameter() on MID)
        probe.addParameter('action', action);
        probe.addParameter('session_id', sessionId);
        probe.addParameter('user_id', userId);
        probe.addParameter('payload', payload);
        probe.addParameter('service_url', this.serviceURL);
        probe.addParameter('auth_token', this.authToken);

        // Set the probe script to use our registered MID Server Script Include
        probe.setJavascript(
            'var probe = new ClaudeTerminalProbe();' +
            'probe.execute();'
        );

        // Create the probe (writes to ECC Queue)
        var eccSysId = probe.create();

        gs.debug('ClaudeTerminalAPI: Probe created for action=' + action +
                 ' ecc_sys_id=' + eccSysId + ' mid=' + this.midServerName);

        // Return the ECC sys_id so callers can track async results
        return {
            ecc_sys_id: eccSysId,
            action: action,
            mid_server: this.midServerName,
            status: 'queued'
        };
    },

    /**
     * Check the result of a previously submitted probe.
     * Looks for the output ECC Queue item matching the input sys_id.
     *
     * @param {string} eccSysId - The sys_id returned by _executeProbe
     * @param {number} maxWaitMs - Maximum time to wait (default: 10000ms)
     * @returns {object|null} - Parsed probe result or null if not ready
     */
    getProbeResult: function(eccSysId, maxWaitMs) {
        maxWaitMs = maxWaitMs || 10000;
        var startTime = new Date().getTime();

        while (new Date().getTime() - startTime < maxWaitMs) {
            var gr = new GlideRecord('ecc_queue');
            gr.addQuery('response_to', eccSysId);
            gr.addQuery('queue', 'output');
            gr.query();

            if (gr.next()) {
                var output = gr.getValue('payload');
                try {
                    var parsed = JSON.parse(output);
                    if (parsed.data) {
                        return parsed.data;
                    }
                    return parsed;
                } catch (e) {
                    return { raw: output };
                }
            }

            // Wait 500ms before checking again
            gs.sleep(500);
        }

        return null; // Timed out
    },

    /**
     * Synchronous helper that creates a session and waits for the result.
     * Useful for scripted REST endpoints.
     *
     * @param {string} userId
     * @param {string} anthropicApiKey
     * @param {string} githubToken
     * @param {string} workspaceType
     * @param {number} timeoutMs - Max wait time (default: 15000)
     * @returns {object} - Session info or error
     */
    createSessionSync: function(userId, anthropicApiKey, githubToken, workspaceType, timeoutMs) {
        var probeRef = this.createSession(userId, anthropicApiKey, githubToken, workspaceType);
        return this.getProbeResult(probeRef.ecc_sys_id, timeoutMs || 15000);
    },

    /**
     * Synchronous helper that sends a command and waits for the result.
     */
    sendCommandSync: function(sessionId, userId, command, timeoutMs) {
        var probeRef = this.sendCommand(sessionId, userId, command);
        return this.getProbeResult(probeRef.ecc_sys_id, timeoutMs || 10000);
    },

    /**
     * Synchronous helper that gets output and waits for the result.
     */
    getOutputSync: function(sessionId, userId, clear, timeoutMs) {
        var probeRef = this.getOutput(sessionId, userId, clear);
        return this.getProbeResult(probeRef.ecc_sys_id, timeoutMs || 10000);
    },

    type: 'ClaudeTerminalAPI'
};
