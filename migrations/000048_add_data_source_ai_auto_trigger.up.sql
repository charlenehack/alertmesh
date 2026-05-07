-- Per-source opt-in: enqueue an ai_tasks row when an incident is created.
-- Default false so log incidents only consume LLM tokens after the operator
-- clicks "触发 AI 分析" unless this flag is explicitly enabled.
ALTER TABLE data_sources
    ADD COLUMN IF NOT EXISTS ai_auto_trigger BOOLEAN NOT NULL DEFAULT false;

ALTER TABLE data_sources
    DROP CONSTRAINT IF EXISTS data_sources_ai_auto_trigger_chk;
ALTER TABLE data_sources
    ADD CONSTRAINT data_sources_ai_auto_trigger_chk
    CHECK (NOT ai_auto_trigger OR ai_enabled);
