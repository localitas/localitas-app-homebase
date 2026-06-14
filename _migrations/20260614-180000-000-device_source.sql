ALTER TABLE devices ADD COLUMN source TEXT NOT NULL DEFAULT '';
ALTER TABLE devices ADD COLUMN source_id TEXT NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS idx_devices_source ON devices(source);
CREATE UNIQUE INDEX IF NOT EXISTS idx_devices_source_id ON devices(source, source_id) WHERE source != '';
