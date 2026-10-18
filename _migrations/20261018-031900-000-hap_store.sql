-- Migration: hap_store
-- Created: 2026-10-18T03:19:00Z


-- HomeKit pairing state, kept with the app's data so pairings survive the
-- container being recreated.
CREATE TABLE IF NOT EXISTS hap_store (
    key TEXT PRIMARY KEY,
    value BLOB NOT NULL
);
