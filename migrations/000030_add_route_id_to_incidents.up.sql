ALTER TABLE incidents
    ADD COLUMN IF NOT EXISTS route_id UUID;

CREATE INDEX IF NOT EXISTS idx_incidents_route_id ON incidents(route_id);
