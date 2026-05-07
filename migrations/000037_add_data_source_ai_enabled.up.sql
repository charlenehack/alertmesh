-- AI 分析白名单：仅 Kafka / OpenSearch 等日志类数据源支持开启 AI 分析；
-- Prometheus / K8s 告警本身就是已确认的指标事实，不应再耗费 LLM token。
-- 列默认为 false 以确保现有数据源不会被自动启用。
ALTER TABLE data_sources
    ADD COLUMN IF NOT EXISTS ai_enabled BOOLEAN NOT NULL DEFAULT false;

-- DB 兜底：业务层会先校验，CHECK 约束确保即便绕过 router 直接写库
-- 也不会出现 prometheus/k8s 数据源 ai_enabled=true 的异常组合。
ALTER TABLE data_sources
    DROP CONSTRAINT IF EXISTS data_sources_ai_enabled_kind_chk;
ALTER TABLE data_sources
    ADD CONSTRAINT data_sources_ai_enabled_kind_chk
    CHECK (ai_enabled = false OR kind IN ('kafka','opensearch'));
