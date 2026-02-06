// =============================================================================
// ClaudeTerminalProbe — MID Server Script Include (runs ON the MID Server)
// =============================================================================
// Extends AbstractAjaxProcessor / AProbe.  When the MID Server picks up a
// JavascriptProbe from the ECC Queue, it executes this script.
//
// The probe reads parameters set by ClaudeTerminalAPI (instance-side),
// makes an HTTP call to the Claude Terminal Service, and writes the
// result back to the ECC Queue output.
//
// Supported actions:
//   create_session, send_command, get_output, get_status,
//   terminate_session, resize_terminal
//
// This replaces the custom Go ECC Poller binary with MID Server-native
// JavaScript execution, reducing latency from ~5-10s to ~3-7s.
// =============================================================================

var ClaudeTerminalProbe = Class.create();
ClaudeTerminalProbe.prototype = {
    type: 'ClaudeTerminalProbe',

    /**
     * Main entry point — called by the JavascriptProbe framework on the MID Server.
     */
    execute: function() {
        var action     = probe.getParameter('action');
        var paramsJson = probe.getParameter('params');
        var serviceUrl = probe.getParameter('service_url');
        var authToken  = probe.getParameter('auth_token');

        if (!action || !serviceUrl) {
            this._setError('Missing required parameters: action and service_url');
            return;
        }

        var params;
        try {
            params = JSON.parse(paramsJson);
        } catch (e) {
            this._setError('Failed to parse params JSON: ' + e.message);
            return;
        }

        ms.log('ClaudeTerminalProbe: Executing action=' + action +
               ' serviceUrl=' + serviceUrl);

        try {
            var result = this._dispatch(action, params, serviceUrl, authToken);
            probe.setOutput(JSON.stringify(result));
        } catch (e) {
            this._setError('Probe execution failed: ' + e.message);
        }
    },

    /**
     * Route to the correct handler based on action.
     * @private
     */
    _dispatch: function(action, params, serviceUrl, authToken) {
        switch (action) {
            case 'create_session':
                return this._createSession(params, serviceUrl, authToken);
            case 'send_command':
                return this._sendCommand(params, serviceUrl, authToken);
            case 'get_output':
                return this._getOutput(params, serviceUrl, authToken);
            case 'get_status':
                return this._getStatus(params, serviceUrl, authToken);
            case 'terminate_session':
                return this._terminateSession(params, serviceUrl, authToken);
            case 'resize_terminal':
                return this._resizeTerminal(params, serviceUrl, authToken);
            default:
                throw new Error('Unknown action: ' + action);
        }
    },

    // ── Action handlers ─────────────────────────────────────────────────

    _createSession: function(params, serviceUrl, authToken) {
        var body = {
            userId: params.userId,
            credentials: params.credentials,
            workspaceType: params.workspaceType || 'temp'
        };

        return this._httpRequest(
            'POST',
            serviceUrl + '/api/session/create',
            body,
            authToken,
            params.userId
        );
    },

    _sendCommand: function(params, serviceUrl, authToken) {
        var body = {
            command: params.command
        };

        return this._httpRequest(
            'POST',
            serviceUrl + '/api/session/' + params.sessionId + '/command',
            body,
            authToken,
            params.userId
        );
    },

    _getOutput: function(params, serviceUrl, authToken) {
        var url = serviceUrl + '/api/session/' + params.sessionId + '/output';
        if (params.clear) {
            url += '?clear=true';
        }

        return this._httpRequest('GET', url, null, authToken, params.userId);
    },

    _getStatus: function(params, serviceUrl, authToken) {
        return this._httpRequest(
            'GET',
            serviceUrl + '/api/session/' + params.sessionId + '/status',
            null,
            authToken,
            params.userId
        );
    },

    _terminateSession: function(params, serviceUrl, authToken) {
        return this._httpRequest(
            'DELETE',
            serviceUrl + '/api/session/' + params.sessionId,
            null,
            authToken,
            params.userId
        );
    },

    _resizeTerminal: function(params, serviceUrl, authToken) {
        var body = {
            cols: params.cols,
            rows: params.rows
        };

        return this._httpRequest(
            'POST',
            serviceUrl + '/api/session/' + params.sessionId + '/resize',
            body,
            authToken,
            params.userId
        );
    },

    // ── HTTP helper using MID Server's built-in HTTPRequest ─────────────

    /**
     * Make an HTTP request from the MID Server to the Claude Terminal Service.
     * Uses the MID Server's built-in HTTPRequest (Apache HttpClient wrapper).
     *
     * @param {string} method    — GET, POST, DELETE, PATCH
     * @param {string} url       — Full URL to the service endpoint
     * @param {object|null} body — Request body (JSON-serializable)
     * @param {string} authToken — Bearer token for Authorization header
     * @param {string} [userId]  — X-User-ID header for session ownership
     * @returns {object} parsed JSON response
     * @private
     */
    _httpRequest: function(method, url, body, authToken, userId) {
        var request = new HTTPRequest(url);
        request.setRequestHeader('Content-Type', 'application/json');
        request.setRequestHeader('Accept', 'application/json');

        if (authToken) {
            request.setRequestHeader('Authorization', 'Bearer ' + authToken);
        }

        if (userId) {
            request.setRequestHeader('X-User-ID', userId);
        }

        var response;
        var bodyStr = body ? JSON.stringify(body) : null;

        switch (method) {
            case 'GET':
                response = request.get();
                break;
            case 'POST':
                response = request.post(bodyStr);
                break;
            case 'DELETE':
                response = request.del();
                break;
            default:
                throw new Error('Unsupported HTTP method: ' + method);
        }

        var statusCode = response.getStatusCode();
        var responseBody = response.getBody();

        ms.log('ClaudeTerminalProbe: ' + method + ' ' + url +
               ' → ' + statusCode);

        if (statusCode < 200 || statusCode >= 300) {
            throw new Error('HTTP ' + statusCode + ': ' + responseBody);
        }

        try {
            return JSON.parse(responseBody);
        } catch (e) {
            return { raw: responseBody };
        }
    },

    /**
     * Set error output on the probe.
     * @private
     */
    _setError: function(message) {
        ms.log('ClaudeTerminalProbe ERROR: ' + message);
        probe.setOutput(JSON.stringify({
            success: false,
            error: message
        }));
        probe.setError(message);
    }
};
