# Roadmap

> 本文档对应原 README §12。已交付项基于代码与 migration 实际状态勾选；尚未上线
> 的能力保留 `[ ]` 待办。改动 Roadmap 条目时请保持「不增不删」原则——只翻状态、
> 不改范围，避免文档与实现脱节。

## Phase 1 — 核心平台（MVP）

- [x] 多源告警接入：Alertmanager v1 webhook / Alertmanager v2 PostableAlerts /
  通用 Webhook（[`internal/ingestion/alertmanager.go`](../internal/ingestion/alertmanager.go) /
  [`alertmanager_v2.go`](../internal/ingestion/alertmanager_v2.go) /
  [`webhook.go`](../internal/ingestion/webhook.go)）
- [x] envconfig 配置体系 + zerolog 日志 + GORM
- [x] 规则引擎内存实现：去重 / 聚合 / 路由 / 抑制（[`internal/engine/`](../internal/engine)）
- [x] Incident 生命周期管理 + Timeline（[`internal/incident/`](../internal/incident)）
- [x] gorbac RBAC + Endpoint 自动同步 + 本地账号 + JWT
- [x] `/api/v1/user/info` 返回 identity 权限列表
- [x] 基础通知渠道：钉钉 / 飞书 / Slack / 邮件 / 语音 / SMS
  （[`internal/notification/`](../internal/notification)，9 个 channel 文件）
- [x] 系统配置 Web UI（LLM Key / 渠道 / 规则）
- [x] Web UI：告警看板 + Incident 详情（含 `Alert.annotations` + 完整 `Alert.labels`
  渲染）
- [x] 通用 Webhook RFC 9421 签名校验中间件 + `webhook_sources` 表 + Web UI
  一次性私钥下发 / 轮换（migration 000032 + [`internal/router/middleware/webhook_signature.go`](../internal/router/middleware/webhook_signature.go)）
- [x] AI 大模型供应商 Web UI（系统管理 → AI 大模型配置，仅管理员）：CRUD + 设为
  默认 + 测试连接，api_key AES-256-GCM 加密、列表恒返回 `******`（migration
  000016 + 000033 + [`internal/router/llm_providers.go`](../internal/router/llm_providers.go)）

## Phase 2 — AI 智能分析

- [x] langchaingo ReAct Agent + 5 个 Tool（[`internal/ai/tools/`](../internal/ai/tools)：
  `metrics` / `logs` / `sysinfo` / `changes` / `runbook`，由 `registry.go` 注册）
- [x] PostgreSQL LISTEN/NOTIFY 任务队列（[`internal/ai/orchestrator.go`](../internal/ai/orchestrator.go)）
- [x] AI 分析报告 WebSocket 流式输出（[`internal/ai/callback.go`](../internal/ai/callback.go)
  + [`internal/router/ai.go`](../internal/router/ai.go)）
- [x] 多轮追问对话（Memory，[`internal/ai/memory.go`](../internal/ai/memory.go)）

## Phase 3 — 扩展接入 + 可选组件

**4.1.4 Ingest Source 通用底座**：

- [x] `data_sources` 表 + GORM 模型 + AES-256-GCM 加密（沿用既有 `data_sources`
  表承载 `kind=kafka|opensearch|elastic|kubernetes|prometheus`，通过 migration
  000043 / 000045 / 000046 / 000047 持续演进）
- [x] selector / mapper：Kafka 走 `expr-lang` + `gjson`（[`internal/ingestion/kafka_filter.go`](../internal/ingestion/kafka_filter.go)），
  OpenSearch / Elastic 走 DSL + JSONPath
- [x] `pg_notify('data_source_event')` 热加载 + debounce + 5 分钟兜底 floor
  （[`internal/ingestion/kafka_manager.go`](../internal/ingestion/kafka_manager.go)
  + [`internal/realtime/pglisten.go`](../internal/realtime/pglisten.go)）
- [x] `/api/v1/data-sources` CRUD + `test-message` dry-run（identity = `dataSource*`，
  admin-only）
- [x] Web UI「数据源」按 kind 拆分组件（Kafka / Kubernetes / Prometheus /
  OpenSearch / Elastic 各自抽屉，`web/src/pages/datasources/`）+ Tabs + 自适应
  宽度

**具体 Connector**：

- [x] Kafka 消费接入：segmentio/kafka-go，SASL/TLS 凭证 DB 加密，
  `max_per_second` 触发 `Reader.Pause`，per-row `consumer_concurrency` `[1,32]`
  起 N workers 共享 GroupID + 引擎并发安全（dedup atomic / routing regex 预编译 /
  per-group_key 锁）。mapping 双语法（gjson 路径 / `expr:` 表达式）+ 内置函数
  `strip_query` / `normalize_path` / `regex_replace` / `coalesce`。详见
  [data-sources.md §5](data-sources.md#5-kafka-数据源filter-表达式--字段映射)
- [ ] K8s Watch 接入（`ALERTMESH_K8S_ENABLED=true`，client-go SharedInformer +
  workqueue 限流），仅有占位文件（[`internal/ingestion/k8s.go`](../internal/ingestion/k8s.go)），
  尚未实现 sub-kind：
  - [ ] `k8s_pod_restart`：Pod Informer + `RestartCount` delta + Events/Logs/Node
    enrichment
  - [ ] `k8s_node`：Node Informer + 状态翻转检测
  - [ ] `k8s_event`：Event watcher 兜底
- [x] OpenSearch / ES 数据源注册 + 凭证 + UI 配置（migration 000047 `kind=elastic`）。
  进程内 **poller 拉取 → Pipeline** 仍缺（见 [log-alert-denoising.md](log-alert-denoising.md) 路线 B）；
  **路线 A**：OpenSearch/Elastic/Kibana **Alerting → RFC 9421 Webhook** 已支持
  `webhook_sources.mapping`（gjson 路径）+ [`internal/router/alert.go`](../internal/router/alert.go) 动态适配器。
- [ ] 云 RDS 慢查询接入（阿里云 / 腾讯云，复用 cloud connector，
  [`internal/ingestion/cloud.go`](../internal/ingestion/cloud.go) 仅占位）
- [ ] 云监控 Webhook 适配（AWS CloudWatch，走 §4.1.2 RFC 9421 入站）

**消息通知策略 v3**（已交付）：

- [x] dispatcher 三段式重构：`resolveRecipients → groupByChannelTarget →
  dispatchBuckets`（[`internal/notification/dispatcher.go`](../internal/notification/dispatcher.go)）
- [x] 1min/3min/5min 线性递增 + 持续 1 小时 P3→P2→P1→P0 升级阶梯（migration
  000035 + 000045 seed + [`internal/incident/service.go`](../internal/incident/service.go)
  `repeatScheduleEntry` / `pickRepeatRung` / `escalateBySchedule` v3 实现）
- [x] P0 才电话 + SMS 默认策略（`severities` 过滤 + IM/邮件合并 @联系人）
- [x] migration 000046 退役旧 `escalation_policies`（lifecycle v3 接管）

**可选基础设施**：

- [ ] Redis 可选启用（`ALERTMESH_REDIS_ENABLED=true`）—— 含 Webhook nonce 去重
  切换到 Redis SETNX。当前仅有 [`internal/store/redis.go`](../internal/store/redis.go)
  TODO 占位。

## Phase 4 — 企业能力

- [ ] LDAP / OIDC 认证 + LDAP 组映射角色（model 已就绪：[`internal/auth/ldap.go`](../internal/auth/ldap.go)
  + [`internal/auth/oidc.go`](../internal/auth/oidc.go)）
- [ ] memberlist 集群模式（多节点 HA）
- [ ] 工单集成（Jira / 飞书多维表格）
- [ ] 审批流
- [ ] Oncall 排班 + 升级策略（schema 已存在 `oncall_schedules`）
- [ ] SLA 报表 + MTTA/MTTR

## Phase 5 — 高级 AI 能力

- [ ] 历史 RCA 知识库（向量检索增强）
- [ ] 告警相关性分析与自动合并
- [ ] 自动修复 Action（需审批后执行）
