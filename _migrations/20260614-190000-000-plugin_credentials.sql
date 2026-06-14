CREATE TABLE IF NOT EXISTS plugin_credentials (
    plugin_name TEXT PRIMARY KEY,
    vault_public_id TEXT NOT NULL,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
);
