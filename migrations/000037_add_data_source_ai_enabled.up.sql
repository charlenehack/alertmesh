-- 所有数据源类型均可开启 AI 分析。
-- 列默认为 false 以确保现有数据源不会被自动启用。
ALTER TABLE data_sources
    ADD COLUMN IF NOT EXISTS ai_enabled BOOLEAN NOT NULL DEFAULT false;

-- 删除旧的 kind 白名单约束（如果存在），允许所有数据源开启 ai_enabled
ALTER TABLE data_sources
    DROP CONSTRAINT IF EXISTS data_sources_ai_enabled_kind_chk;
