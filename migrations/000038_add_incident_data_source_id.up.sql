-- 给 incident 打上"来自哪个数据源"的标签，用于 AI 分析白名单：
--   ai 触发时回查 data_sources.kind / ai_enabled 决定是否真的入队 LLM 任务。
-- 历史 incident 没有该列，会保持 NULL，在 service.shouldRunAI 中视为 disabled。
ALTER TABLE incidents
    ADD COLUMN IF NOT EXISTS data_source_id UUID;

CREATE INDEX IF NOT EXISTS idx_incidents_data_source
    ON incidents (data_source_id);
