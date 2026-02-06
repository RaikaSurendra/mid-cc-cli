// =============================================================================
// Claude Terminal MID Widget — Client Script (AngularJS)
// =============================================================================
// Service Portal widget controller for the MID Server proxy variant.
// Uses the Scripted REST API (/api/x_claude/terminal_mid) which routes
// through the MID Server instead of the ECC Queue poller.
// =============================================================================

api.controller = function($scope, $http, $interval, $timeout) {
    var c = this;

    // ── State ───────────────────────────────────────────────────────────
    c.connected   = false;
    c.connecting  = false;
    c.sessionId   = null;
    c.command     = '';
    c.midServer   = '';
    c.outputLines = [];

    var pollInterval = null;
    var terminal     = null;
    var BASE_URL     = '/api/x_claude/terminal_mid';

    // ── xterm.js initialization ─────────────────────────────────────────
    function initTerminal() {
        if (terminal) {
            terminal.dispose();
        }

        terminal = new Terminal({
            cursorBlink: true,
            fontSize: 14,
            fontFamily: 'Menlo, Monaco, "Courier New", monospace',
            theme: {
                background: '#1e1e2e',
                foreground: '#cdd6f4',
                cursor: '#f5e0dc',
                selectionBackground: '#45475a'
            },
            rows: 30,
            cols: 120,
            scrollback: 5000
        });

        var container = document.getElementById('claude-terminal');
        if (container) {
            container.innerHTML = '';
            terminal.open(container);
            terminal.writeln('Claude Code Terminal (MID Server Proxy)');
            terminal.writeln('─'.repeat(50));
            terminal.writeln('');
        }
    }

    // ── Connect to terminal session ─────────────────────────────────────
    c.connect = function() {
        if (c.connecting || c.connected) return;

        c.connecting = true;
        initTerminal();

        terminal.writeln('\\x1b[33mConnecting via MID Server...\\x1b[0m');

        $http.post(BASE_URL + '/session', {
            credentials: {
                anthropicApiKey: c.data.apiKey || ''
            },
            workspaceType: 'temp'
        }).then(function(resp) {
            var data = resp.data;

            if (data.sessionId) {
                c.sessionId  = data.sessionId;
                c.connected  = true;
                c.connecting = false;
                c.midServer  = data.midServer || '';

                terminal.writeln('\\x1b[32mConnected! Session: ' + c.sessionId + '\\x1b[0m');
                terminal.writeln('\\x1b[90mWorkspace: ' + (data.workspacePath || 'temp') + '\\x1b[0m');
                terminal.writeln('');

                startPolling();
            } else {
                c.connecting = false;
                terminal.writeln('\\x1b[31mFailed to create session: ' +
                    (data.error || 'Unknown error') + '\\x1b[0m');
            }
        }).catch(function(err) {
            c.connecting = false;
            var msg = (err.data && err.data.error) || err.statusText || 'Connection failed';
            terminal.writeln('\\x1b[31mError: ' + msg + '\\x1b[0m');
        });
    };

    // ── Send command ────────────────────────────────────────────────────
    c.sendCommand = function() {
        if (!c.command || !c.connected || !c.sessionId) return;

        var cmd = c.command;
        c.command = '';

        terminal.writeln('\\x1b[36m$ ' + cmd + '\\x1b[0m');

        $http.post(BASE_URL + '/session/' + c.sessionId + '/command', {
            command: cmd
        }).catch(function(err) {
            var msg = (err.data && err.data.error) || 'Failed to send command';
            terminal.writeln('\\x1b[31mError: ' + msg + '\\x1b[0m');
        });
    };

    // ── Poll for output ─────────────────────────────────────────────────
    function startPolling() {
        if (pollInterval) {
            $interval.cancel(pollInterval);
        }

        pollInterval = $interval(function() {
            if (!c.connected || !c.sessionId) return;

            $http.get(BASE_URL + '/session/' + c.sessionId + '/output', {
                params: { clear: 'true' }
            }).then(function(resp) {
                var data = resp.data;

                if (data.output && data.output.length > 0) {
                    data.output.forEach(function(chunk) {
                        if (chunk.data) {
                            terminal.write(chunk.data);
                        }
                    });
                }

                // Check if session ended
                if (data.status === 'terminated' || data.status === 'error') {
                    terminal.writeln('');
                    terminal.writeln('\\x1b[33mSession ended: ' + data.status + '\\x1b[0m');
                    c.disconnect();
                }
            }).catch(function(err) {
                // Session might have been terminated
                if (err.status === 404) {
                    terminal.writeln('\\x1b[31mSession not found — disconnecting.\\x1b[0m');
                    c.disconnect();
                }
            });
        }, 2000); // Poll every 2 seconds (faster than ECC poller's 5s)
    }

    // ── Disconnect ──────────────────────────────────────────────────────
    c.disconnect = function() {
        if (pollInterval) {
            $interval.cancel(pollInterval);
            pollInterval = null;
        }

        if (c.sessionId && c.connected) {
            $http.delete(BASE_URL + '/session/' + c.sessionId).catch(function() {
                // Best-effort cleanup
            });
        }

        c.connected  = false;
        c.connecting = false;
        c.sessionId  = null;

        if (terminal) {
            terminal.writeln('');
            terminal.writeln('\\x1b[90mDisconnected.\\x1b[0m');
        }
    };

    // ── Cleanup on scope destroy ────────────────────────────────────────
    $scope.$on('$destroy', function() {
        c.disconnect();
        if (terminal) {
            terminal.dispose();
            terminal = null;
        }
    });
};
