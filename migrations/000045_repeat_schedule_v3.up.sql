-- Incident lifecycle v3: linear-increase repeat sequence + dwell-driven escalation.
--
-- v2 stored a flat array of {after, interval, tag, escalate?} rungs and chose
-- "the highest after still ≤ elapsed".  v3 replaces this with a global object
-- whose schedule.severity_chain owns the escalation ladder; intervals are no
-- longer constants per-tier but a per-incident sequence index walking
-- interval_sequence_minutes (then linearly stepping by interval_step_minutes
-- up to interval_max_minutes).
--
-- New columns on incidents:
--   repeat_seq_index    — counter incremented after every successful re-notify;
--                         reset to 0 when severity escalates (so the new tier
--                         starts at the dense head of the sequence again).
--   severity_started_at — anchor used by the dwell-based escalator; set on
--                         create/reopen and on every escalation.

ALTER TABLE incidents
    ADD COLUMN IF NOT EXISTS repeat_seq_index    INT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS severity_started_at TIMESTAMPTZ;

UPDATE incidents
   SET severity_started_at = COALESCE(severity_started_at, opened_at)
 WHERE severity_started_at IS NULL;

-- Seed value rewritten as the v3 object.  Idempotent: only overwrites if the
-- key already exists (it does after migration 000042).  The legacy array
-- shape is detected and rejected by the parser, so any operator who hand-
-- edited the JSON to the v2 form will fall back to "no schedule = no
-- repeats" with a single WARN log line.
UPDATE system_configs
   SET value = '{
  "version": 3,
  "interval_sequence_minutes": [1, 3, 5],
  "interval_step_minutes": 2,
  "interval_max_minutes": 30,
  "severity_chain": [
    {"severity": "P3", "dwell": "1h", "tag": "[REPEAT]"},
    {"severity": "P2", "dwell": "1h", "tag": "[ATTENTION]"},
    {"severity": "P1", "dwell": "1h", "tag": "[ATTENTION]"},
    {"severity": "P0", "dwell": null, "tag": "[CRITICAL]"}
  ]
}',
       description = 'v3: linear-increase interval sequence + dwell-driven severity escalation; P0 is terminal.'
 WHERE key = 'notification.repeat_schedule';
