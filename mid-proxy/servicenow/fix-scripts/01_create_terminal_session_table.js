// =============================================================================
// Fix Script: Create x_claude_terminal_session table
// =============================================================================
// Run this in: System Definition → Fix Scripts → New
// Name: Claude Terminal - Create Session Table
//
// Creates the terminal session table with all columns, choices, and indexes.
// Safe to re-run — checks for existing table before creating.
// =============================================================================

(function() {
    var TABLE_NAME = 'x_claude_terminal_session';
    var TABLE_LABEL = 'Claude Terminal Session';

    // ── Check if table already exists ───────────────────────────────────
    if (gs.tableExists(TABLE_NAME)) {
        gs.info('Table ' + TABLE_NAME + ' already exists. Skipping creation.');
        gs.print('Table ' + TABLE_NAME + ' already exists. Skipping creation.');
        return;
    }

    gs.info('Creating table: ' + TABLE_NAME);

    // ── Create the table ────────────────────────────────────────────────
    var tc = new GlideTableCreator(TABLE_NAME, TABLE_LABEL);
    tc.setApplication('x_claude_terminal');
    tc.setExtendable(false);
    tc.setCreateAccessControls(true);
    tc.update();

    gs.info('Table ' + TABLE_NAME + ' created successfully.');

    // ── Add columns ─────────────────────────────────────────────────────

    // session_id — String(40), mandatory, unique, display
    var sessionId = new GlideRecord('sys_dictionary');
    sessionId.initialize();
    sessionId.setValue('name', TABLE_NAME);
    sessionId.setValue('element', 'session_id');
    sessionId.setValue('column_label', 'Session ID');
    sessionId.setValue('internal_type', 'string');
    sessionId.setValue('max_length', 40);
    sessionId.setValue('mandatory', true);
    sessionId.setValue('unique', true);
    sessionId.setValue('display', true);
    sessionId.insert();

    // user — Reference to sys_user, mandatory
    var user = new GlideRecord('sys_dictionary');
    user.initialize();
    user.setValue('name', TABLE_NAME);
    user.setValue('element', 'user');
    user.setValue('column_label', 'User');
    user.setValue('internal_type', 'reference');
    user.setValue('reference', 'sys_user');
    user.setValue('mandatory', true);
    user.insert();

    // status — Choice, mandatory, default "initializing"
    var status = new GlideRecord('sys_dictionary');
    status.initialize();
    status.setValue('name', TABLE_NAME);
    status.setValue('element', 'status');
    status.setValue('column_label', 'Status');
    status.setValue('internal_type', 'choice');
    status.setValue('mandatory', true);
    status.setValue('default_value', 'initializing');
    status.insert();

    // command_queue — JSON(4000)
    var cmdQueue = new GlideRecord('sys_dictionary');
    cmdQueue.initialize();
    cmdQueue.setValue('name', TABLE_NAME);
    cmdQueue.setValue('element', 'command_queue');
    cmdQueue.setValue('column_label', 'Command Queue');
    cmdQueue.setValue('internal_type', 'json');
    cmdQueue.setValue('max_length', 4000);
    cmdQueue.insert();

    // output_buffer — JSON(65000)
    var outputBuf = new GlideRecord('sys_dictionary');
    outputBuf.initialize();
    outputBuf.setValue('name', TABLE_NAME);
    outputBuf.setValue('element', 'output_buffer');
    outputBuf.setValue('column_label', 'Output Buffer');
    outputBuf.setValue('internal_type', 'json');
    outputBuf.setValue('max_length', 65000);
    outputBuf.insert();

    // workspace_path — String(255)
    var wsPath = new GlideRecord('sys_dictionary');
    wsPath.initialize();
    wsPath.setValue('name', TABLE_NAME);
    wsPath.setValue('element', 'workspace_path');
    wsPath.setValue('column_label', 'Workspace Path');
    wsPath.setValue('internal_type', 'string');
    wsPath.setValue('max_length', 255);
    wsPath.insert();

    // workspace_type — Choice, default "isolated"
    var wsType = new GlideRecord('sys_dictionary');
    wsType.initialize();
    wsType.setValue('name', TABLE_NAME);
    wsType.setValue('element', 'workspace_type');
    wsType.setValue('column_label', 'Workspace Type');
    wsType.setValue('internal_type', 'choice');
    wsType.setValue('default_value', 'isolated');
    wsType.insert();

    // last_activity — Glide Date/Time, mandatory
    var lastActivity = new GlideRecord('sys_dictionary');
    lastActivity.initialize();
    lastActivity.setValue('name', TABLE_NAME);
    lastActivity.setValue('element', 'last_activity');
    lastActivity.setValue('column_label', 'Last Activity');
    lastActivity.setValue('internal_type', 'glide_date_time');
    lastActivity.setValue('mandatory', true);
    lastActivity.insert();

    // mid_server — String(100)
    var midServer = new GlideRecord('sys_dictionary');
    midServer.initialize();
    midServer.setValue('name', TABLE_NAME);
    midServer.setValue('element', 'mid_server');
    midServer.setValue('column_label', 'MID Server');
    midServer.setValue('internal_type', 'string');
    midServer.setValue('max_length', 100);
    midServer.insert();

    // error_message — String(1000)
    var errorMsg = new GlideRecord('sys_dictionary');
    errorMsg.initialize();
    errorMsg.setValue('name', TABLE_NAME);
    errorMsg.setValue('element', 'error_message');
    errorMsg.setValue('column_label', 'Error Message');
    errorMsg.setValue('internal_type', 'string');
    errorMsg.setValue('max_length', 1000);
    errorMsg.insert();

    gs.info('All columns added to ' + TABLE_NAME);

    // ── Add choices for "status" field ──────────────────────────────────
    var statusChoices = [
        { value: 'initializing', label: 'Initializing', order: 100 },
        { value: 'active',       label: 'Active',       order: 200 },
        { value: 'idle',         label: 'Idle',         order: 300 },
        { value: 'terminated',   label: 'Terminated',   order: 400 },
        { value: 'error',        label: 'Error',        order: 500 }
    ];

    statusChoices.forEach(function(choice) {
        var ch = new GlideRecord('sys_choice');
        ch.initialize();
        ch.setValue('name', TABLE_NAME);
        ch.setValue('element', 'status');
        ch.setValue('value', choice.value);
        ch.setValue('label', choice.label);
        ch.setValue('sequence', choice.order);
        ch.setValue('language', 'en');
        ch.insert();
    });

    gs.info('Status choices added.');

    // ── Add choices for "workspace_type" field ──────────────────────────
    var wsChoices = [
        { value: 'isolated',   label: 'Isolated (Temporary)', order: 100 },
        { value: 'persistent', label: 'Persistent',           order: 200 }
    ];

    wsChoices.forEach(function(choice) {
        var ch = new GlideRecord('sys_choice');
        ch.initialize();
        ch.setValue('name', TABLE_NAME);
        ch.setValue('element', 'workspace_type');
        ch.setValue('value', choice.value);
        ch.setValue('label', choice.label);
        ch.setValue('sequence', choice.order);
        ch.setValue('language', 'en');
        ch.insert();
    });

    gs.info('Workspace type choices added.');

    // ── Create indexes ──────────────────────────────────────────────────
    // Index: user + status (composite)
    var idx1 = new GlideRecord('sys_index');
    idx1.initialize();
    idx1.setValue('table', TABLE_NAME);
    idx1.setValue('index_col_1', 'user');
    idx1.setValue('index_col_2', 'status');
    idx1.setValue('unique_index', false);
    idx1.insert();

    // Index: session_id (unique)
    var idx2 = new GlideRecord('sys_index');
    idx2.initialize();
    idx2.setValue('table', TABLE_NAME);
    idx2.setValue('index_col_1', 'session_id');
    idx2.setValue('unique_index', true);
    idx2.insert();

    gs.info('Indexes created.');

    gs.print('SUCCESS: Table ' + TABLE_NAME + ' created with all columns, choices, and indexes.');
})();
