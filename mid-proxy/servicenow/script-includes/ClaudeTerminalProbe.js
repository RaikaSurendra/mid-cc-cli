/**
 * ClaudeTerminalProbe - MID Server Script Include
 *
 * Runs ON the MID Server when it picks up an ECC Queue item with
 * topic=ClaudeTerminalMIDProxy. Makes HTTP calls to the Claude Terminal
 * HTTP Service running on the same network.
 *
 * ServiceNow Setup:
 *   1. Create MID Server Script Include:
 *      - Name: ClaudeTerminalProbe
 *      - Script: (paste this file)
 *      - Active: true
 *
 *   2. System Properties (sys_properties):
 *      - x_claude.terminal.service_url = http://claude-terminal-service:3000
 *      - x_claude.terminal.auth_token  = mid-llm-cli-dev-token-2026
 *
 * ECC Queue Payload Format (JSON string in probe parameters):
 *   {
 *     "action": "create_session|send_command|get_output|get_status|terminate_session|resize_terminal",
 *     "session_id": "uuid (for existing sessions)",
 *     "user_id": "sys_user.user_name",
 *     "data": { ... action-specific payload }
 *   }
 */

// Entry point - called by MID Server when probe executes
var ClaudeTerminalProbe = Class.create();
ClaudeTerminalProbe.prototype = Object.extendsObject(AProbe, {

    /**
     * Main execution method called by the MID Server probe framework.
     * Reads parameters from the probe, routes to the correct action handler,
     * and sets the result output.
     */
    execute: function() {
        var startTime = new Date().getTime();

        try {
            // Read probe parameters
            var action = this.getParameter('action');
            var sessionId = this.getParameter('session_id') || '';
            var userId = this.getParameter('user_id') || '';
            var payload = this.getParameter('payload') || '{}';
            var serviceURL = this.getParameter('service_url') || 'http://claude-terminal-service:3000';
            var authToken = this.getParameter('auth_token') || '';

            if (!action) {
                this.setError('Missing required parameter: action');
                return;
            }

            this.log('Processing action: ' + action + ' for user: ' + userId);

            var result;

            switch (action) {
                case 'create_session':
                    result = this._createSession(serviceURL, authToken, payload);
                    break;

                case 'send_command':
                    result = this._sendCommand(serviceURL, authToken, sessionId, userId, payload);
                    break;

                case 'get_output':
                    result = this._getOutput(serviceURL, authToken, sessionId, userId, payload);
                    break;

                case 'get_status':
                    result = this._getStatus(serviceURL, authToken, sessionId, userId);
                    break;

                case 'terminate_session':
                    result = this._terminateSession(serviceURL, authToken, sessionId, userId);
                    break;

                case 'resize_terminal':
                    result = this._resizeTerminal(serviceURL, authToken, sessionId, userId, payload);
                    break;

                default:
                    this.setError('Unknown action: ' + action);
                    return;
            }

            var elapsed = new Date().getTime() - startTime;
            this.log('Action ' + action + ' completed in ' + elapsed + 'ms');

            // Set probe result
            this.setOutput(JSON.stringify({
                success: true,
                action: action,
                data: result,
                elapsed_ms: elapsed
            }));

        } catch (e) {
            var elapsed = new Date().getTime() - startTime;
            this.log('ERROR: Action failed after ' + elapsed + 'ms: ' + e.message);
            this.setError(JSON.stringify({
                success: false,
                error: e.message,
                elapsed_ms: elapsed
            }));
        }
    },

    /**
     * POST /api/session/create
     */
    _createSession: function(serviceURL, authToken, payload) {
        var data = JSON.parse(payload);

        if (!data.userId) {
            throw new Error('Missing userId in payload');
        }
        if (!data.credentials || !data.credentials.anthropicApiKey) {
            throw new Error('Missing credentials.anthropicApiKey in payload');
        }

        var body = JSON.stringify({
            userId: data.userId,
            credentials: data.credentials,
            workspaceType: data.workspaceType || 'isolated'
        });

        return this._httpRequest('POST', serviceURL + '/api/session/create', authToken, '', body);
    },

    /**
     * POST /api/session/{id}/command
     */
    _sendCommand: function(serviceURL, authToken, sessionId, userId, payload) {
        this._validateSession(sessionId);
        var data = JSON.parse(payload);

        if (!data.command) {
            throw new Error('Missing command in payload');
        }

        var body = JSON.stringify({ command: data.command });
        return this._httpRequest('POST', serviceURL + '/api/session/' + sessionId + '/command', authToken, userId, body);
    },

    /**
     * GET /api/session/{id}/output
     */
    _getOutput: function(serviceURL, authToken, sessionId, userId, payload) {
        this._validateSession(sessionId);
        var data = JSON.parse(payload);
        var clear = data.clear === true ? 'true' : 'false';

        return this._httpRequest('GET', serviceURL + '/api/session/' + sessionId + '/output?clear=' + clear, authToken, userId, null);
    },

    /**
     * GET /api/session/{id}/status
     */
    _getStatus: function(serviceURL, authToken, sessionId, userId) {
        this._validateSession(sessionId);
        return this._httpRequest('GET', serviceURL + '/api/session/' + sessionId + '/status', authToken, userId, null);
    },

    /**
     * DELETE /api/session/{id}
     */
    _terminateSession: function(serviceURL, authToken, sessionId, userId) {
        this._validateSession(sessionId);
        return this._httpRequest('DELETE', serviceURL + '/api/session/' + sessionId, authToken, userId, null);
    },

    /**
     * POST /api/session/{id}/resize
     */
    _resizeTerminal: function(serviceURL, authToken, sessionId, userId, payload) {
        this._validateSession(sessionId);
        var data = JSON.parse(payload);

        if (!data.cols || !data.rows) {
            throw new Error('Missing cols or rows in payload');
        }

        var body = JSON.stringify({ cols: data.cols, rows: data.rows });
        return this._httpRequest('POST', serviceURL + '/api/session/' + sessionId + '/resize', authToken, userId, body);
    },

    /**
     * Generic HTTP request helper using MID Server's built-in HTTP client.
     */
    _httpRequest: function(method, url, authToken, userId, body) {
        var httpClient = new Packages.org.apache.commons.httpclient.HttpClient();
        httpClient.getHttpConnectionManager().getParams().setConnectionTimeout(30000);
        httpClient.getHttpConnectionManager().getParams().setSoTimeout(30000);

        var httpMethod;
        if (method === 'GET') {
            httpMethod = new Packages.org.apache.commons.httpclient.methods.GetMethod(url);
        } else if (method === 'POST') {
            httpMethod = new Packages.org.apache.commons.httpclient.methods.PostMethod(url);
            if (body) {
                httpMethod.setRequestEntity(
                    new Packages.org.apache.commons.httpclient.methods.StringRequestEntity(
                        body, 'application/json', 'UTF-8'
                    )
                );
            }
        } else if (method === 'DELETE') {
            httpMethod = new Packages.org.apache.commons.httpclient.methods.DeleteMethod(url);
        }

        // Set headers
        httpMethod.setRequestHeader('Content-Type', 'application/json');
        httpMethod.setRequestHeader('Accept', 'application/json');

        if (authToken) {
            httpMethod.setRequestHeader('Authorization', 'Bearer ' + authToken);
        }
        if (userId) {
            httpMethod.setRequestHeader('X-User-ID', userId);
        }

        try {
            var statusCode = httpClient.executeMethod(httpMethod);
            var responseBody = httpMethod.getResponseBodyAsString();

            if (statusCode >= 200 && statusCode < 300) {
                return JSON.parse(responseBody);
            } else {
                throw new Error('HTTP ' + statusCode + ': ' + responseBody);
            }
        } finally {
            httpMethod.releaseConnection();
        }
    },

    _validateSession: function(sessionId) {
        if (!sessionId) {
            throw new Error('Missing session_id');
        }
    },

    log: function(msg) {
        ms.log('ClaudeTerminalProbe: ' + msg);
    },

    setOutput: function(output) {
        result.output = output;
    },

    setError: function(error) {
        result.error = error;
    },

    type: 'ClaudeTerminalProbe'
});
