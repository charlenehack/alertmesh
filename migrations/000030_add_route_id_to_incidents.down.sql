DROP INDEX IF EXISTS idx_incidents_route_id;
ALTER TABLE incidents DROP COLUMN IF EXISTS route_id;
