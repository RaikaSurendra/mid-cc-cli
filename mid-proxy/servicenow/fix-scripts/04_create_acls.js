// =============================================================================
// Fix Script: Create ACLs for Claude Terminal tables
// =============================================================================
// Run this in: System Definition → Fix Scripts → New
// Name: Claude Terminal - Create Access Controls
//
// Creates row-level ACLs so users can only read/write their own records.
// Admin gets full access. Credentials table is extra-restricted.
// Safe to re-run — checks for existing ACLs before creating.
// =============================================================================

(function() {

    // ── Helper: create an ACL if it doesn't exist ───────────────────────
    function createACL(table, operation, role, script, condition) {
        var aclName = table + '.' + operation;

        var gr = new GlideRecord('sys_security_acl');
        gr.addQuery('name', aclName);
        gr.addQuery('operation', operation);
        gr.query();

        if (gr.next()) {
            gs.print('SKIP: ACL ' + aclName + ' (' + operation + ') already exists.');
            return;
        }

        var acl = new GlideRecord('sys_security_acl');
        acl.initialize();
        acl.setValue('name', aclName);
        acl.setValue('operation', operation);
        acl.setValue('type', 'record');
        acl.setValue('active', true);
        acl.setValue('admin_overrides', true);

        if (script) {
            acl.setValue('script', script);
            acl.setValue('advanced', true);
        }

        if (condition) {
            acl.setValue('condition', condition);
        }

        var aclSysId = acl.insert();

        // Add role requirement
        if (role && aclSysId) {
            var aclRole = new GlideRecord('sys_security_acl_role');
            aclRole.initialize();
            aclRole.setValue('sys_security_acl', aclSysId);
            aclRole.setValue('sys_user_role', getRoleSysId(role));
            aclRole.insert();
        }

        gs.print('CREATED: ACL ' + aclName + ' (' + operation + ')' +
                 (role ? ' with role: ' + role : ''));
    }

    // ── Helper: get role sys_id by name ─────────────────────────────────
    function getRoleSysId(roleName) {
        var gr = new GlideRecord('sys_user_role');
        gr.addQuery('name', roleName);
        gr.query();
        if (gr.next()) {
            return gr.getUniqueValue();
        }
        return '';
    }

    // ── User ownership script (user can only see their own records) ─────
    var ownershipScript =
        'answer = gs.hasRole("admin") || ' +
        'current.getValue("user") == gs.getUserID();';

    // ── x_claude_terminal_session ACLs ──────────────────────────────────
    gs.info('Creating ACLs for x_claude_terminal_session...');

    // Read: user can read own sessions, admin can read all
    createACL('x_claude_terminal_session', 'read', null, ownershipScript);

    // Write: user can update own sessions, admin can update all
    createACL('x_claude_terminal_session', 'write', null, ownershipScript);

    // Create: any authenticated user
    createACL('x_claude_terminal_session', 'create', 'snc_internal');

    // Delete: admin only
    createACL('x_claude_terminal_session', 'delete', 'admin');

    // ── x_claude_credentials ACLs ───────────────────────────────────────
    gs.info('Creating ACLs for x_claude_credentials...');

    // Read: user can only read own credentials, admin can read all
    createACL('x_claude_credentials', 'read', null, ownershipScript);

    // Write: user can only update own credentials, admin can update all
    createACL('x_claude_credentials', 'write', null, ownershipScript);

    // Create: any authenticated user (first-time credential setup)
    createACL('x_claude_credentials', 'create', 'snc_internal');

    // Delete: admin only (prevent accidental credential deletion)
    createACL('x_claude_credentials', 'delete', 'admin');

    gs.print('SUCCESS: All ACLs for Claude Terminal tables processed.');
})();
