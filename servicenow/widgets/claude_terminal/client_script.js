function ClaudeTerminalController($scope, $http, $timeout, spUtil, $window) {
  var c = this;
  var terminal = null;
  var sessionId = null;
  var pollingInterval = 1000; // Start with 1 second
  var maxPollingInterval = 5000; // Max 5 seconds
  var minPollingInterval = 100; // Min 100ms when active
  var pollingTimer = null;
  var isActive = false;
  var ambSubscription = null;

  // Load xterm.js if not already loaded
  if (typeof Terminal === 'undefined') {
    var xtermScript = document.createElement('script');
    xtermScript.src = 'https://cdn.jsdelivr.net/npm/xterm@5.3.0/lib/xterm.min.js';
    document.head.appendChild(xtermScript);

    var xtermCSS = document.createElement('link');
    xtermCSS.rel = 'stylesheet';
    xtermCSS.href = 'https://cdn.jsdelivr.net/npm/xterm@5.3.0/css/xterm.css';
    document.head.appendChild(xtermCSS);
  }

  c.data = {
    sessionId: null,
    status: 'initializing',
    statusClass: 'status-initializing',
    error: null,
    needsCredentials: false
  };

  /**
   * Initialize terminal session
   */
  c.init = function() {
    console.log('Initializing Claude Terminal...');
    createSession();
  };

  /**
   * Create a new terminal session
   */
  function createSession() {
    $http({
      method: 'POST',
      url: '/api/now/claude/terminal/session/create',
      data: {
        workspaceType: 'isolated'
      }
    }).then(function(response) {
      if (response.data.success) {
        sessionId = response.data.sessionId;
        c.data.sessionId = sessionId;
        c.data.status = response.data.status;
        updateStatusClass();

        // Initialize xterm.js terminal
        initializeTerminal();

        // Subscribe to AMB notifications
        subscribeToAMB();

        // Start polling for output
        startPolling();
      } else {
        handleError(response.data.error);
      }
    }, function(error) {
      console.error('Failed to create session:', error);
      if (error.status === 401 || (error.data && error.data.error && error.data.error.includes('credentials'))) {
        c.data.needsCredentials = true;
      } else {
        handleError(error.data ? error.data.error : 'Failed to create session');
      }
    });
  }

  /**
   * Initialize xterm.js terminal
   */
  function initializeTerminal() {
    $timeout(function() {
      var terminalElement = document.getElementById('terminal-' + sessionId);
      if (!terminalElement) {
        console.error('Terminal element not found');
        return;
      }

      terminal = new Terminal({
        cursorBlink: true,
        fontSize: 14,
        fontFamily: 'Menlo, Monaco, "Courier New", monospace',
        theme: {
          background: '#1e1e1e',
          foreground: '#d4d4d4'
        },
        cols: 80,
        rows: 24
      });

      terminal.open(terminalElement);

      // Handle user input
      terminal.onData(function(data) {
        sendCommand(data);
        isActive = true;
        pollingInterval = minPollingInterval; // Speed up polling when user is typing
      });

      // Handle resize
      var resizeObserver = new ResizeObserver(function() {
        if (terminal && terminalElement) {
          var dims = terminal._core._renderService.dimensions;
          if (dims) {
            resizeTerminal(dims.actualCellWidth, dims.actualCellHeight);
          }
        }
      });
      resizeObserver.observe(terminalElement);

      console.log('Terminal initialized');
    }, 500);
  }

  /**
   * Subscribe to AMB notifications
   */
  function subscribeToAMB() {
    var channel = 'claude.terminal.' + sessionId;
    console.log('Subscribing to AMB channel:', channel);

    if (window.AMB) {
      ambSubscription = window.AMB.subscribe(channel, function(message) {
        console.log('AMB notification received:', message);
        // When we get a notification, immediately poll for output
        pollOutput();
      });
    }
  }

  /**
   * Send command to session
   */
  function sendCommand(command) {
    $http({
      method: 'POST',
      url: '/api/now/claude/terminal/session/' + sessionId + '/command',
      data: {
        command: command
      }
    }).then(function(response) {
      if (!response.data.success) {
        console.error('Failed to send command:', response.data.error);
      }
    }, function(error) {
      console.error('Failed to send command:', error);
    });
  }

  /**
   * Start polling for output
   */
  function startPolling() {
    pollingTimer = $timeout(function poll() {
      pollOutput();
      pollingTimer = $timeout(poll, pollingInterval);
    }, pollingInterval);
  }

  /**
   * Poll for output
   */
  function pollOutput() {
    $http({
      method: 'GET',
      url: '/api/now/claude/terminal/session/' + sessionId + '/output?clear=true'
    }).then(function(response) {
      if (response.data.success && response.data.output && response.data.output.length > 0) {
        // Write output to terminal
        response.data.output.forEach(function(chunk) {
          if (terminal) {
            terminal.write(chunk.data);
          }
        });

        // Reset polling to min interval since we got output
        isActive = true;
        pollingInterval = minPollingInterval;
      } else {
        // No output, gradually increase polling interval
        if (!isActive) {
          pollingInterval = Math.min(pollingInterval * 1.2, maxPollingInterval);
        }
        isActive = false;
      }

      // Update status
      if (response.data.status) {
        c.data.status = response.data.status;
        updateStatusClass();
      }
    }, function(error) {
      console.error('Failed to poll output:', error);
    });
  }

  /**
   * Resize terminal
   */
  function resizeTerminal(cols, rows) {
    $http({
      method: 'POST',
      url: '/api/now/claude/terminal/session/' + sessionId + '/resize',
      data: {
        cols: cols,
        rows: rows
      }
    }).then(function(response) {
      if (!response.data.success) {
        console.error('Failed to resize terminal:', response.data.error);
      }
    }, function(error) {
      console.error('Failed to resize terminal:', error);
    });
  }

  /**
   * Terminate session
   */
  c.terminateSession = function() {
    if (!confirm('Are you sure you want to terminate this session?')) {
      return;
    }

    if (pollingTimer) {
      $timeout.cancel(pollingTimer);
    }

    if (ambSubscription && window.AMB) {
      window.AMB.unsubscribe(ambSubscription);
    }

    $http({
      method: 'DELETE',
      url: '/api/now/claude/terminal/session/' + sessionId
    }).then(function(response) {
      spUtil.addInfoMessage('Session terminated successfully');
      $window.location.reload();
    }, function(error) {
      console.error('Failed to terminate session:', error);
      spUtil.addErrorMessage('Failed to terminate session');
    });
  };

  /**
   * Retry session creation
   */
  c.retry = function() {
    c.data.error = null;
    c.data.needsCredentials = false;
    createSession();
  };

  /**
   * Handle errors
   */
  function handleError(errorMessage) {
    c.data.error = errorMessage;
    c.data.status = 'error';
    updateStatusClass();
  }

  /**
   * Update status class for styling
   */
  function updateStatusClass() {
    var statusMap = {
      'initializing': 'status-initializing',
      'active': 'status-active',
      'idle': 'status-idle',
      'terminated': 'status-terminated',
      'error': 'status-error'
    };
    c.data.statusClass = statusMap[c.data.status] || 'status-unknown';
  }

  /**
   * Cleanup on destroy
   */
  $scope.$on('$destroy', function() {
    console.log('Cleaning up terminal...');
    if (pollingTimer) {
      $timeout.cancel(pollingTimer);
    }
    if (ambSubscription && window.AMB) {
      window.AMB.unsubscribe(ambSubscription);
    }
    if (terminal) {
      terminal.dispose();
    }
  });

  // Initialize
  c.init();
}
