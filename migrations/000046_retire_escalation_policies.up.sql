-- v3 removes the standalone ack_timeout escalation chain.  The lifecycle
-- ladder previously expressed by escalation_policies (P3→P2/1h, P2→P1/2h,
-- P1→P0/4h) is now folded into notification.repeat_schedule.severity_chain
-- and driven by per-tier dwell timers.
--
-- We DO NOT DROP the table: operators who hand-rolled custom escalation
-- rows would lose them silently, and the model is still loaded by the
-- /alert/escalation-policies UI page (now showing the rows as deprecated).
-- The runtime simply stops reading from this table — see
-- internal/engine/pipeline.go (loadEscalations removed) and
-- cmd/alertmesh/main.go (StartEscalator removed).

UPDATE escalation_policies
   SET is_enabled = FALSE,
       description = COALESCE(description, '') || ' [DEPRECATED in v3 — superseded by notification.repeat_schedule.severity_chain]'
 WHERE name IN (
    '默认升级 P3→P2',
    '默认升级 P2→P1',
    '默认升级 P1→P0'
);

COMMENT ON TABLE escalation_policies IS
    'DEPRECATED in v3: incident lifecycle escalation has been folded into notification.repeat_schedule.severity_chain (driven by per-tier dwell timers in incident/service.go). The table is retained so existing custom rows are not silently lost; the runtime no longer reads from it.';
