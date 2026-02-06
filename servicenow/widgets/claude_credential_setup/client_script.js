function ClaudeCredentialSetupController($scope, $http, spUtil) {
  var c = this;

  c.data = {
    anthropicApiKey: '',
    githubToken: '',
    hasExisting: false,
    lastUsed: null,
    validationStatus: 'untested',
    saving: false,
    testing: false,
    deleting: false,
    saved: false,
    error: null
  };

  /**
   * Initialize - load existing credentials info
   */
  c.init = function() {
    loadCredentialInfo();
  };

  /**
   * Load existing credential information (not the actual keys)
   */
  function loadCredentialInfo() {
    $http({
      method: 'GET',
      url: '/api/now/claude/credentials/info'
    }).then(function(response) {
      if (response.data.success && response.data.hasCredentials) {
        c.data.hasExisting = true;
        c.data.lastUsed = response.data.lastUsed;
        c.data.validationStatus = response.data.validationStatus;
      }
    }, function(error) {
      console.error('Failed to load credential info:', error);
    });
  }

  /**
   * Save credentials
   */
  c.saveCredentials = function() {
    if (!c.data.anthropicApiKey) {
      c.data.error = 'Anthropic API Key is required';
      return;
    }

    c.data.saving = true;
    c.data.error = null;

    $http({
      method: 'POST',
      url: '/api/now/claude/credentials/save',
      data: {
        anthropicApiKey: c.data.anthropicApiKey,
        githubToken: c.data.githubToken
      }
    }).then(function(response) {
      if (response.data.success) {
        c.data.saved = true;
        c.data.hasExisting = true;
        spUtil.addInfoMessage('Credentials saved successfully');

        // Clear the form
        c.data.anthropicApiKey = '';
        c.data.githubToken = '';

        // Reload credential info
        loadCredentialInfo();
      } else {
        c.data.error = response.data.error || 'Failed to save credentials';
      }
      c.data.saving = false;
    }, function(error) {
      console.error('Failed to save credentials:', error);
      c.data.error = error.data ? error.data.error : 'Failed to save credentials';
      c.data.saving = false;
    });
  };

  /**
   * Test credentials
   */
  c.testCredentials = function() {
    if (!c.data.anthropicApiKey) {
      c.data.error = 'Anthropic API Key is required';
      return;
    }

    c.data.testing = true;
    c.data.error = null;

    $http({
      method: 'POST',
      url: '/api/now/claude/credentials/test',
      data: {
        anthropicApiKey: c.data.anthropicApiKey
      }
    }).then(function(response) {
      if (response.data.success) {
        spUtil.addInfoMessage('API Key is valid!');
        c.data.validationStatus = 'valid';
      } else {
        c.data.error = 'API Key validation failed: ' + (response.data.error || 'Invalid key');
        c.data.validationStatus = 'invalid';
      }
      c.data.testing = false;
    }, function(error) {
      console.error('Failed to test credentials:', error);
      c.data.error = 'Failed to test API Key: ' + (error.data ? error.data.error : 'Unknown error');
      c.data.validationStatus = 'invalid';
      c.data.testing = false;
    });
  };

  /**
   * Delete credentials
   */
  c.deleteCredentials = function() {
    if (!confirm('Are you sure you want to delete your stored credentials?')) {
      return;
    }

    c.data.deleting = true;
    c.data.error = null;

    $http({
      method: 'DELETE',
      url: '/api/now/claude/credentials/delete'
    }).then(function(response) {
      if (response.data.success) {
        spUtil.addInfoMessage('Credentials deleted successfully');
        c.data.hasExisting = false;
        c.data.lastUsed = null;
        c.data.validationStatus = 'untested';
      } else {
        c.data.error = response.data.error || 'Failed to delete credentials';
      }
      c.data.deleting = false;
    }, function(error) {
      console.error('Failed to delete credentials:', error);
      c.data.error = error.data ? error.data.error : 'Failed to delete credentials';
      c.data.deleting = false;
    });
  };

  // Initialize
  c.init();
}
