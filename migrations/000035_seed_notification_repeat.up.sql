-- Seed two new system_configs rows used by the notification dispatcher.
--
--   notification.repeat_interval — Go duration string (e.g. "1h", "30m").
--     When an open incident keeps receiving alerts, the dispatcher silently
--     appends them; if more than this interval has passed since the last
--     successful notification, a new "[REPEAT]" notification is fired so
--     the on-call team is reminded the issue is still ongoing.
--
--   system.web_base_url — public URL prefix of the AlertMesh web UI.
--     Used to build clickable "查看完整报告" links inside notifications
--     (especially the AI-followup notification).  Optional: when empty
--     the link is omitted.
--
-- Both rows use ON CONFLICT DO NOTHING so re-running the migration is safe
-- and never overwrites operator-set values.

INSERT INTO system_configs (key, value, description) VALUES
    ('notification.repeat_interval', '1h',
     'Go duration string. Re-notify cooldown for ongoing open incidents.'),
    ('system.web_base_url', '',
     'Public URL prefix of the AlertMesh web UI, e.g. https://alertmesh.example.com')
ON CONFLICT (key) DO NOTHING;
