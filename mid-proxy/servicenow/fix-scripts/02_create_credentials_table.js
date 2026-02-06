// =============================================================================
// Fix Script: Create x_claude_credentials table
// =============================================================================
// Run this in: System Definition → Fix Scripts → New
// Name: Claude Terminal - Create Credentials Table
//
// Creates the credentials table for storing encrypted API keys per user.
// Uses password2 field type for AES-128 encryption at rest.
// Safe to re-run — checks for existing table before creating.
// =============================================================================

(function() {
    var TABLE_NAME = 'x_claude_credentials';
    var TABLE_LABEL = 'Claude Credentials';

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

    // user — Reference to sys_user, mandatory, unique, display
    var user = new GlideRecord('sys_dictionary');
    user.initialize();
    user.setValue('name', TABLE_NAME);
    user.setValue('element', 'user');
    user.setValue('column_label', 'User');
    user.setValue('internal_type', 'reference');
    user.setValue('reference', 'sys_user');
    user.setValue('mandatory', true);
    user.setValue('unique', true);
    user.setValue('display', true);
    user.insert();

    // anthropic_api_key — Password2 (encrypted at rest), mandatory
    var apiKey = new GlideRecord('sys_dictionary');
    apiKey.initialize();
    apiKey.setValue('name', TABLE_NAME);
    apiKey.setValue('element', 'anthropic_api_key');
    apiKey.setValue('column_label', 'Anthropic API Key');
    apiKey.setValue('internal_type', 'password2');
    apiKey.setValue('max_length', 255);
    apiKey.setValue('mandatory', true);
    apiKey.insert();

    // github_token — Password2 (encrypted at rest), optional
    var ghToken = new GlideRecord('sys_dictionary');
    ghToken.initialize();
    ghToken.setValue('name', TABLE_NAME);
    ghToken.setValue('element', 'github_token');
    ghToken.setValue('column_label', 'GitHub Token');
    ghToken.setValue('internal_type', 'password2');
    ghToken.setValue('max_length', 255);
    ghToken.insert();

    // last_used — Glide Date/Time
    var lastUsed = new GlideRecord('sys_dictionary');
    lastUsed.initialize();
    lastUsed.setValue('name', TABLE_NAME);
    lastUsed.setValue('element', 'last_used');
    lastUsed.setValue('column_label', 'Last Used');
    lastUsed.setValue('internal_type', 'glide_date_time');
    lastUsed.insert();

    // last_validated — Glide Date/Time
    var lastValidated = new GlideRecord('sys_dictionary');
    lastValidated.initialize();
    lastValidated.setValue('name', TABLE_NAME);
    lastValidated.setValue('element', 'last_validated');
    lastValidated.setValue('column_label', 'Last Validated');
    lastValidated.setValue('internal_type', 'glide_date_time');
    lastValidated.insert();

    // validation_status — Choice, default "untested"
    var valStatus = new GlideRecord('sys_dictionary');
    valStatus.initialize();
    valStatus.setValue('name', TABLE_NAME);
    valStatus.setValue('element', 'validation_status');
    valStatus.setValue('column_label', 'Validation Status');
    valStatus.setValue('internal_type', 'choice');
    valStatus.setValue('default_value', 'untested');
    valStatus.insert();

    gs.info('All columns added to ' + TABLE_NAME);

    // ── Add choices for "validation_status" field ────────────────────────
    var choices = [
        { value: 'valid',    label: 'Valid',    order: 100 },
        { value: 'invalid',  label: 'Invalid',  order: 200 },
        { value: 'untested', label: 'Untested', order: 300 }
    ];

    choices.forEach(function(choice) {
        var ch = new GlideRecord('sys_choice');
        ch.initialize();
        ch.setValue('name', TABLE_NAME);
        ch.setValue('element', 'validation_status');
        ch.setValue('value', choice.value);
        ch.setValue('label', choice.label);
        ch.setValue('sequence', choice.order);
        ch.setValue('language', 'en');
        ch.insert();
    });

    gs.info('Validation status choices added.');

    // ── Create index ────────────────────────────────────────────────────
    // Unique index on user (one credential record per user)
    var idx = new GlideRecord('sys_index');
    idx.initialize();
    idx.setValue('table', TABLE_NAME);
    idx.setValue('index_col_1', 'user');
    idx.setValue('unique_index', true);
    idx.insert();

    gs.info('Index created.');

    gs.print('SUCCESS: Table ' + TABLE_NAME + ' created with all columns, choices, and indexes.');
})();
