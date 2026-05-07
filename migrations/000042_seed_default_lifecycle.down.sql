DELETE FROM escalation_policies WHERE name IN (
    '默认升级 P3→P2',
    '默认升级 P2→P1',
    '默认升级 P1→P0'
);

DELETE FROM system_configs WHERE key IN (
    'notification.repeat_schedule',
    'incident.staleness_timeout',
    'incident.reopen_window'
);
