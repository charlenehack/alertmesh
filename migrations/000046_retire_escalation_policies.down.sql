-- Re-enable the default escalation policies and clear the comment.

UPDATE escalation_policies
   SET is_enabled = TRUE,
       description = REPLACE(
           COALESCE(description, ''),
           ' [DEPRECATED in v3 — superseded by notification.repeat_schedule.severity_chain]',
           ''
       )
 WHERE name IN (
    '默认升级 P3→P2',
    '默认升级 P2→P1',
    '默认升级 P1→P0'
);

COMMENT ON TABLE escalation_policies IS NULL;
