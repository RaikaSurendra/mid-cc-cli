/**
 * Claude Terminal MID Proxy - Scripted REST API
 *
 * Exposes REST endpoints on the ServiceNow instance that route through
 * the MID Server to the Claude Terminal HTTP Service.
 *
 * ServiceNow Setup:
 *   1. Create Scripted REST API:
 *      - Name: Claude Terminal MID Proxy
 *      - API ID: x_claude_terminal_mid
 *      - API namespace: x_claude
 *      - Active: true
 *
 *   2. Create Resources (see below for each endpoint)
 *
 * All endpoints require an authenticated ServiceNow session.
 * The user's sys_user.user_name is used as the userId for session ownership.
 */

// ============================================================================
// Resource: POST /api/x_claude/terminal_mid/session
// Name: Create Session
// HTTP Method: POST
// ============================================================================
(function createSession(request, response) {
    var body = request.body.data;
    var userId = gs.getUserName();

    if (!body.anthropicApiKey) {
        response.setStatus(400);
        response.setBody({ error: 'Missing anthropicApiKey' });
        return;
    }

    var api = new ClaudeTerminalAPI();
    var probeRef = api.createSession(
        userId,
        body.anthropicApiKey,
        body.githubToken || '',
        body.workspaceType || 'isolated'
    );

    // Wait for MID Server to process (synchronous for REST response)
    var result = api.getProbeResult(probeRef.ecc_sys_id, 15000);

    if (result) {
        response.setStatus(200);
        response.setBody(result);
    } else {
        response.setStatus(504);
        response.setBody({
            error: 'MID Server did not respond within timeout',
            ecc_sys_id: probeRef.ecc_sys_id
        });
    }
})(request, response);


// ============================================================================
// Resource: POST /api/x_claude/terminal_mid/session/{session_id}/command
// Name: Send Command
// HTTP Method: POST
// Relative path: /session/{session_id}/command
// ============================================================================
(function sendCommand(request, response) {
    var sessionId = request.pathParams.session_id;
    var body = request.body.data;
    var userId = gs.getUserName();

    if (!body.command) {
        response.setStatus(400);
        response.setBody({ error: 'Missing command' });
        return;
    }

    var api = new ClaudeTerminalAPI();
    var probeRef = api.sendCommand(sessionId, userId, body.command);

    var result = api.getProbeResult(probeRef.ecc_sys_id, 10000);

    if (result) {
        response.setStatus(200);
        response.setBody(result);
    } else {
        response.setStatus(504);
        response.setBody({ error: 'MID Server timeout', ecc_sys_id: probeRef.ecc_sys_id });
    }
})(request, response);


// ============================================================================
// Resource: GET /api/x_claude/terminal_mid/session/{session_id}/output
// Name: Get Output
// HTTP Method: GET
// Relative path: /session/{session_id}/output
// Query params: clear=true|false
// ============================================================================
(function getOutput(request, response) {
    var sessionId = request.pathParams.session_id;
    var userId = gs.getUserName();
    var clear = request.queryParams.clear == 'true';

    var api = new ClaudeTerminalAPI();
    var probeRef = api.getOutput(sessionId, userId, clear);

    var result = api.getProbeResult(probeRef.ecc_sys_id, 10000);

    if (result) {
        response.setStatus(200);
        response.setBody(result);
    } else {
        response.setStatus(504);
        response.setBody({ error: 'MID Server timeout', ecc_sys_id: probeRef.ecc_sys_id });
    }
})(request, response);


// ============================================================================
// Resource: GET /api/x_claude/terminal_mid/session/{session_id}/status
// Name: Get Status
// HTTP Method: GET
// Relative path: /session/{session_id}/status
// ============================================================================
(function getStatus(request, response) {
    var sessionId = request.pathParams.session_id;
    var userId = gs.getUserName();

    var api = new ClaudeTerminalAPI();
    var probeRef = api.getStatus(sessionId, userId);

    var result = api.getProbeResult(probeRef.ecc_sys_id, 10000);

    if (result) {
        response.setStatus(200);
        response.setBody(result);
    } else {
        response.setStatus(504);
        response.setBody({ error: 'MID Server timeout', ecc_sys_id: probeRef.ecc_sys_id });
    }
})(request, response);


// ============================================================================
// Resource: DELETE /api/x_claude/terminal_mid/session/{session_id}
// Name: Terminate Session
// HTTP Method: DELETE
// Relative path: /session/{session_id}
// ============================================================================
(function terminateSession(request, response) {
    var sessionId = request.pathParams.session_id;
    var userId = gs.getUserName();

    var api = new ClaudeTerminalAPI();
    var probeRef = api.terminateSession(sessionId, userId);

    var result = api.getProbeResult(probeRef.ecc_sys_id, 10000);

    if (result) {
        response.setStatus(200);
        response.setBody(result);
    } else {
        response.setStatus(504);
        response.setBody({ error: 'MID Server timeout', ecc_sys_id: probeRef.ecc_sys_id });
    }
})(request, response);


// ============================================================================
// Resource: POST /api/x_claude/terminal_mid/session/{session_id}/resize
// Name: Resize Terminal
// HTTP Method: POST
// Relative path: /session/{session_id}/resize
// ============================================================================
(function resizeTerminal(request, response) {
    var sessionId = request.pathParams.session_id;
    var body = request.body.data;
    var userId = gs.getUserName();

    if (!body.cols || !body.rows) {
        response.setStatus(400);
        response.setBody({ error: 'Missing cols or rows' });
        return;
    }

    var api = new ClaudeTerminalAPI();
    var probeRef = api.resizeTerminal(sessionId, userId, body.cols, body.rows);

    var result = api.getProbeResult(probeRef.ecc_sys_id, 10000);

    if (result) {
        response.setStatus(200);
        response.setBody(result);
    } else {
        response.setStatus(504);
        response.setBody({ error: 'MID Server timeout', ecc_sys_id: probeRef.ecc_sys_id });
    }
})(request, response);
