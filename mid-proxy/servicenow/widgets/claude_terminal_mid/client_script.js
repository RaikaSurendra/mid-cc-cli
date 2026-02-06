/**
 * Claude Terminal Widget - MID Proxy Version
 *
 * Client-side controller for the Service Portal widget.
 * Uses the MID Proxy Scripted REST API instead of direct ECC Queue writes.
 *
 * Differences from original widget:
 *   - API calls go through /api/x_claude/terminal_mid/* (MID proxy)
 *   - No direct ECC Queue manipulation
 *   - Slightly higher latency per call (~2-3s MID Server roundtrip)
 *   - Built-in retry on 504 (MID timeout)
 *
 * Dependencies:
 *   - xterm.js (loaded via CDN or Service Portal dependency)
 *   - xterm-addon-fit
 */
api.controller = function($scope, $http, $interval, $timeout) {
    'use strict';

    var c = this;
    var terminal = null;
    var fitAddon = null;

    // State
    c.sessionId = null;
    c.status = 'disconnected';
    c.error = null;
    c.polling = null;
    c.pollInterval = 500; // Start at 500ms (MID proxy is slower than direct)
    c.maxPollInterval = 5000;
    c.minPollInterval = 200;
    c.consecutiveEmpty = 0;

    // API base path (MID Proxy Scripted REST API)
    var API_BASE = '/api/x_claude/terminal_mid';

    /**
     * Initialize xterm.js terminal
     */
    c.initTerminal = function() {
        var termContainer = document.getElementById('terminal-container');
        if (!termContainer) return;

        terminal = new Terminal({
            cursorBlink: true,
            fontSize: 14,
            fontFamily: 'Menlo, Monaco, "Courier New", monospace',
            theme: {
                background: '#1e1e1e',
                foreground: '#d4d4d4',
                cursor: '#d4d4d4'
            },
            cols: 120,
            rows: 30
        });

        fitAddon = new FitAddon.FitAddon();
        terminal.loadAddon(fitAddon);
        terminal.open(termContainer);
        fitAddon.fit();

        // Handle user input
        terminal.onData(function(data) {
            if (c.sessionId && c.status === 'active') {
                c.sendCommand(data);
            }
        });

        // Handle terminal resize
        terminal.onResize(function(size) {
            if (c.sessionId && c.status === 'active') {
                c.resizeTerminal(size.cols, size.rows);
            }
        });

        // Window resize handler
        window.addEventListener('resize', function() {
            if (fitAddon) fitAddon.fit();
        });

        terminal.writeln('Claude Terminal (MID Proxy Mode)');
        terminal.writeln('Type your Anthropic API key and press Connect to start.');
        terminal.writeln('');
    };

    /**
     * Create a new session via MID Server proxy.
     */
    c.createSession = function() {
        if (!c.data.apiKey) {
            c.error = 'Please enter your Anthropic API key';
            return;
        }

        c.status = 'connecting';
        c.error = null;
        terminal.writeln('Connecting via MID Server proxy...');

        $http.post(API_BASE + '/session', {
            anthropicApiKey: c.data.apiKey,
            workspaceType: 'isolated'
        }).then(function(resp) {
            if (resp.data && resp.data.sessionId) {
                c.sessionId = resp.data.sessionId;
                c.status = 'active';
                terminal.writeln('Session created: ' + c.sessionId);
                terminal.writeln('');
                c.startPolling();
            } else {
                c.status = 'error';
                c.error = 'Unexpected response from MID Server';
                terminal.writeln('ERROR: Unexpected response');
            }
        }).catch(function(err) {
            c.status = 'error';
            if (err.status === 504) {
                c.error = 'MID Server did not respond in time. Is it running?';
            } else {
                c.error = err.data ? err.data.error : 'Connection failed';
            }
            terminal.writeln('ERROR: ' + c.error);
        });
    };

    /**
     * Send a command to the session.
     */
    c.sendCommand = function(command) {
        if (!c.sessionId) return;

        $http.post(API_BASE + '/session/' + c.sessionId + '/command', {
            command: command
        }).then(function() {
            // Reset poll interval for fast output retrieval
            c.pollInterval = c.minPollInterval;
            c.consecutiveEmpty = 0;
        }).catch(function(err) {
            if (err.status === 504) {
                // MID timeout - command may still have been delivered
                console.warn('MID timeout on send_command, will retry output poll');
            } else {
                terminal.writeln('\r\nERROR sending command: ' + (err.data ? err.data.error : 'unknown'));
            }
        });
    };

    /**
     * Poll for output from the session.
     * Uses adaptive polling: fast when there's output, slow when idle.
     */
    c.startPolling = function() {
        if (c.polling) return;

        var poll = function() {
            if (!c.sessionId || c.status !== 'active') {
                c.stopPolling();
                return;
            }

            $http.get(API_BASE + '/session/' + c.sessionId + '/output', {
                params: { clear: 'true' }
            }).then(function(resp) {
                if (resp.data && resp.data.output && resp.data.output.length > 0) {
                    // Write output to terminal
                    resp.data.output.forEach(function(chunk) {
                        terminal.write(chunk.data);
                    });

                    // Fast polling when we have output
                    c.consecutiveEmpty = 0;
                    c.pollInterval = c.minPollInterval;
                } else {
                    // Adaptive backoff when no output
                    c.consecutiveEmpty++;
                    if (c.consecutiveEmpty > 5) {
                        c.pollInterval = Math.min(c.pollInterval * 1.5, c.maxPollInterval);
                    }
                }

                // Check session status
                if (resp.data && resp.data.status === 'terminated') {
                    c.status = 'disconnected';
                    terminal.writeln('\r\nSession terminated.');
                    c.stopPolling();
                    return;
                }

                // Schedule next poll
                c.polling = $timeout(poll, c.pollInterval);

            }).catch(function(err) {
                if (err.status === 504) {
                    // MID timeout - retry with backoff
                    c.pollInterval = Math.min(c.pollInterval * 2, c.maxPollInterval);
                    c.polling = $timeout(poll, c.pollInterval);
                } else {
                    terminal.writeln('\r\nERROR: Lost connection to session');
                    c.status = 'error';
                    c.stopPolling();
                }
            });
        };

        // Start first poll
        c.polling = $timeout(poll, c.pollInterval);
    };

    /**
     * Stop polling for output.
     */
    c.stopPolling = function() {
        if (c.polling) {
            $timeout.cancel(c.polling);
            c.polling = null;
        }
    };

    /**
     * Resize the terminal on the server side.
     */
    c.resizeTerminal = function(cols, rows) {
        if (!c.sessionId) return;

        $http.post(API_BASE + '/session/' + c.sessionId + '/resize', {
            cols: cols,
            rows: rows
        }).catch(function() {
            // Resize failures are non-critical
            console.warn('Failed to resize terminal');
        });
    };

    /**
     * Terminate the current session.
     */
    c.disconnect = function() {
        if (!c.sessionId) return;

        c.stopPolling();
        terminal.writeln('\r\nDisconnecting...');

        $http.delete(API_BASE + '/session/' + c.sessionId).then(function() {
            terminal.writeln('Session terminated.');
            c.sessionId = null;
            c.status = 'disconnected';
        }).catch(function() {
            // Force local cleanup even if server call fails
            c.sessionId = null;
            c.status = 'disconnected';
            terminal.writeln('Disconnected (server may still be cleaning up).');
        });
    };

    // Initialize terminal on load
    $timeout(function() {
        c.initTerminal();
    }, 100);

    // Cleanup on scope destroy
    $scope.$on('$destroy', function() {
        c.stopPolling();
        if (terminal) terminal.dispose();
    });
};
