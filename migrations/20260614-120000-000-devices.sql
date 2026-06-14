CREATE TABLE IF NOT EXISTS devices (
    id TEXT PRIMARY KEY,
    node_id INTEGER NOT NULL UNIQUE,
    name TEXT NOT NULL,
    device_type TEXT NOT NULL DEFAULT '',
    room TEXT NOT NULL DEFAULT '',
    vendor TEXT NOT NULL DEFAULT '',
    model TEXT NOT NULL DEFAULT '',
    clusters TEXT NOT NULL DEFAULT '[]',
    online INTEGER NOT NULL DEFAULT 0,
    virtual INTEGER NOT NULL DEFAULT 0,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS device_state_log (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    device_id TEXT NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    cluster TEXT NOT NULL,
    attribute TEXT NOT NULL,
    value TEXT NOT NULL,
    recorded_at INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_device_state_log_device ON device_state_log(device_id, recorded_at DESC);
