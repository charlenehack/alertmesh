-- Drop the legacy notification_channels table.  In the new architecture this
-- fan-out path has been replaced by alert_routes → notification_policies →
-- notification_contacts; the dispatcher no longer reads from this table and
-- the UI no longer writes to it.  Kept the historical create/down files for
-- migration tooling, but operationally the table is dead weight.
DROP TABLE IF EXISTS notification_channels;
