-- Allow alert_routes rows to be inserted without an explicit matchers value.
-- An empty JSON array means "match all alerts", which is the canonical shape
-- of the operator-defined catch-all (兜底) route.  Without this default,
-- direct INSERTs from external SQL tooling would still trip the NOT NULL
-- constraint even though the Go API layer already normalises nil → '[]'.
ALTER TABLE alert_routes
    ALTER COLUMN matchers SET DEFAULT '[]'::jsonb;

UPDATE alert_routes SET matchers = '[]'::jsonb WHERE matchers IS NULL;
