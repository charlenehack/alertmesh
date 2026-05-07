-- Revert lifecycle v3.

ALTER TABLE incidents
    DROP COLUMN IF EXISTS severity_started_at,
    DROP COLUMN IF EXISTS repeat_seq_index;

UPDATE system_configs
   SET value = '[{"after":"0s","interval":"30m","tag":"[REPEAT]"},{"after":"3h","interval":"2h","tag":"[ATTENTION]","escalate":true},{"after":"24h","interval":"6h","tag":"[REPEAT]"}]',
       description = 'JSON ladder for progressive re-notification of ongoing incidents.'
 WHERE key = 'notification.repeat_schedule';
