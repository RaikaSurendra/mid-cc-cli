// =============================================================================
// Claude Terminal MID Proxy — Scripted REST API
// =============================================================================
// Exposes REST endpoints on the ServiceNow instance that route requests
// through the MID Server (via JavascriptProbe) to the Claude Terminal Service.
//
// Base path: /api/x_claude/terminal_mid
//
// Endpoints:
//   POST   /session                    — Create a new terminal session
//   POST   /session/{id}/command       — Send a command
//   GET    /session/{id}/output        — Get terminal output
//   GET    /session/{id}/status        — Get session status
//   DELETE /session/{id}               — Terminate session
//   POST   /session/{id}/resize        — Resize terminal
//
// Each endpoint:
//   1. Validates the request
//   2. Calls ClaudeTerminalAPI to create a JavascriptProbe
//   3. Waits synchronously for the MID Server response
//   4. Returns the result to the caller
//
// API Name:  Claude Terminal MID Proxy
// API ID:    x_claude_terminal_mid
// =============================================================================

(function process(/* RESTAPIRequest */ request, /* RESTAPIResponse */ response) {

    var api = new ClaudeTerminalAPI();

    // ── Helper: get current user ────────────────────────────────────────
    function getCurrentUserId() {
        return gs.getUserID();
    }

    // ── Helper: send JSON response ──────────────────────────────────────
    function sendResponse(statusCode, body) {
        response.setStatus(statusCode);
        response.setContentType('application/json');
        response.setBody(body);
    }

    // ── Helper: send error ──────────────────────────────────────────────
    function sendError(statusCode, message) {
        sendResponse(statusCode, {
            success: false,
            error: message
        });
    }

    // ── Route: POST /session — Create Session ───────────────────────────
    function createSession() {
        var body = request.body.data;
        var userId = getCurrentUserId();

        if (!body || !body.credentials || !body.credentials.anthropicApiKey) {
            sendError(400, 'Missing required field: credentials.anthropicApiKey');
            return;
        }

        var result = api.createSessionSync(
            userId,
            body.credentials,
            body.workspaceType || 'temp'
        );

        if (result.success === false) {
            sendError(502, result.error || 'MID Server request failed');
            return;
        }

        sendResponse(200, result);
    }

    // ── Route: POST /session/{id}/command — Send Command ────────────────
    function sendCommand() {
        var sessionId = request.pathParams.id;
        var body = request.body.data;

        if (!sessionId) {
            sendError(400, 'Missing session ID');
            return;
        }

        if (!body || !body.command) {
            sendError(400, 'Missing required field: command');
            return;
        }

        var result = api.sendCommandSync(sessionId, body.command);

        if (result.success === false) {
            sendError(502, result.error || 'MID Server request failed');
            return;
        }

        sendResponse(200, result);
    }

    // ── Route: GET /session/{id}/output — Get Output ────────────────────
    function getOutput() {
        var sessionId = request.pathParams.id;

        if (!sessionId) {
            sendError(400, 'Missing session ID');
            return;
        }

        var clear = request.queryParams.clear === 'true';
        var result = api.getOutputSync(sessionId, clear);

        if (result.success === false) {
            sendError(502, result.error || 'MID Server request failed');
            return;
        }

        sendResponse(200, result);
    }

    // ── Route: GET /session/{id}/status — Get Status ────────────────────
    function getStatus() {
        var sessionId = request.pathParams.id;

        if (!sessionId) {
            sendError(400, 'Missing session ID');
            return;
        }

        var result = api.getStatusSync(sessionId);

        if (result.success === false) {
            sendError(502, result.error || 'MID Server request failed');
            return;
        }

        sendResponse(200, result);
    }

    // ── Route: DELETE /session/{id} — Terminate Session ──────────────────
    function terminateSession() {
        var sessionId = request.pathParams.id;

        if (!sessionId) {
            sendError(400, 'Missing session ID');
            return;
        }

        var result = api.terminateSessionSync(sessionId);

        if (result.success === false) {
            sendError(502, result.error || 'MID Server request failed');
            return;
        }

        sendResponse(200, result);
    }

    // ── Route: POST /session/{id}/resize — Resize Terminal ──────────────
    function resizeTerminal() {
        var sessionId = request.pathParams.id;
        var body = request.body.data;

        if (!sessionId) {
            sendError(400, 'Missing session ID');
            return;
        }

        if (!body || !body.cols || !body.rows) {
            sendError(400, 'Missing required fields: cols, rows');
            return;
        }

        var result = api.resizeTerminalSync(sessionId, body.cols, body.rows);

        if (result.success === false) {
            sendError(502, result.error || 'MID Server request failed');
            return;
        }

        sendResponse(200, result);
    }

    // ── Request Router ──────────────────────────────────────────────────
    var method   = request.getRequestMethod();
    var segments = request.uri.split('/').filter(function(s) { return s !== ''; });

    // Normalize: remove api/x_claude/terminal_mid prefix
    // After normalization, segments = ['session'] or ['session', '{id}', 'command'] etc.
    var path = segments.slice(segments.indexOf('session'));

    if (path[0] !== 'session') {
        sendError(404, 'Unknown endpoint');
        return;
    }

    if (path.length === 1 && method === 'POST') {
        createSession();
    } else if (path.length === 2 && method === 'GET') {
        // GET /session/{id} — treat as status
        request.pathParams = { id: path[1] };
        getStatus();
    } else if (path.length === 2 && method === 'DELETE') {
        request.pathParams = { id: path[1] };
        terminateSession();
    } else if (path.length === 3 && path[2] === 'command' && method === 'POST') {
        request.pathParams = { id: path[1] };
        sendCommand();
    } else if (path.length === 3 && path[2] === 'output' && method === 'GET') {
        request.pathParams = { id: path[1] };
        getOutput();
    } else if (path.length === 3 && path[2] === 'status' && method === 'GET') {
        request.pathParams = { id: path[1] };
        getStatus();
    } else if (path.length === 3 && path[2] === 'resize' && method === 'POST') {
        request.pathParams = { id: path[1] };
        resizeTerminal();
    } else {
        sendError(404, 'Unknown endpoint: ' + method + ' /' + path.join('/'));
    }

})(request, response);
