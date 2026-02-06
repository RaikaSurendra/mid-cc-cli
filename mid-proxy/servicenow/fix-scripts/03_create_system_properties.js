// =============================================================================
// Fix Script: Create System Properties for MID Proxy
// =============================================================================
// Run this in: System Definition → Fix Scripts → New
// Name: Claude Terminal - Create MID Proxy System Properties
//
// Creates the three system properties required by ClaudeTerminalAPI.js
// to connect to the MID Server and Claude Terminal Service.
// Safe to re-run — checks for existing properties before creating.
// =============================================================================

(function() {
    var properties = [
        {
            name: 'x_claude.terminal.mid_server',
            description: 'Name of the MID Server used for Claude Terminal proxy. Must match the MID Server record name in ServiceNow.',
            value: 'mid-docker-proxy',
            type: 'string'
        },
        {
            name: 'x_claude.terminal.service_url',
            description: 'Base URL of the Claude Terminal HTTP Service, as reachable from the MID Server (e.g. Docker network hostname).',
            value: 'http://claude-terminal-service:3000',
            type: 'string'
        },
        {
            name: 'x_claude.terminal.auth_token',
            description: 'Bearer token for authenticating with the Claude Terminal HTTP Service. Must match the API_AUTH_TOKEN env var on the service.',
            value: '',
            type: 'password'
        }
    ];

    properties.forEach(function(prop) {
        var gr = new GlideRecord('sys_properties');
        gr.addQuery('name', prop.name);
        gr.query();

        if (gr.next()) {
            gs.info('Property ' + prop.name + ' already exists (current value preserved). Skipping.');
            gs.print('SKIP: Property ' + prop.name + ' already exists.');
            return;
        }

        var newProp = new GlideRecord('sys_properties');
        newProp.initialize();
        newProp.setValue('name', prop.name);
        newProp.setValue('description', prop.description);
        newProp.setValue('value', prop.value);
        newProp.setValue('type', prop.type);
        newProp.setValue('read_roles', 'admin');
        newProp.setValue('write_roles', 'admin');
        newProp.insert();

        gs.info('Created property: ' + prop.name);
        gs.print('CREATED: ' + prop.name + ' = ' + (prop.type === 'password' ? '****' : prop.value));
    });

    gs.print('SUCCESS: All MID Proxy system properties processed.');
})();
