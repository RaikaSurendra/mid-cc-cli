# Fix Scripts — Execution Order

Run these in **System Definition → Fix Scripts** on your ServiceNow instance.
Execute in order — each script is idempotent (safe to re-run).

| # | Script | Purpose |
|---|--------|---------|
| 1 | `01_create_terminal_session_table.js` | Creates `x_claude_terminal_session` table with columns, choices, indexes |
| 2 | `02_create_credentials_table.js` | Creates `x_claude_credentials` table with password2 encrypted fields |
| 3 | `03_create_system_properties.js` | Creates MID proxy system properties (mid_server, service_url, auth_token) |
| 4 | `04_create_acls.js` | Creates row-level ACLs (user ownership isolation + admin override) |

## After Running

1. Verify tables: Navigate to `x_claude_terminal_session.list` and `x_claude_credentials.list`
2. Verify properties: **System Properties** → search `x_claude.terminal`
3. Update the auth token property with your actual service token
4. Proceed to deploy Script Includes, REST API, and Widget from `../script-includes/`, `../scripted-rest/`, `../widgets/`
