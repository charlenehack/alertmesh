# 目录结构

参考 [langchaingo](https://github.com/tmc/langchaingo) 的项目风格：`internal/` 下按
领域功能扁平分包，每个包职责单一、命名即含义，不嵌套 `service/router/provider`
等分层目录。**只有一个入口 `cmd/alertmesh/`**。

```
alertmesh/
│
├── cmd/alertmesh/                  # 唯一入口，单二进制
│   ├── main.go                     # rootCtx → InitApp(cfg) → Server / Pipeline / Orchestrator
│   ├── app.go                      # App 聚合体（Server/DB/Orchestrator/Pipeline/...）
│   ├── providers.go                # 手写 wire provider
│   ├── wire.go / wire_gen.go       # google/wire 注入图
│
├── internal/                       # 所有私有业务代码
│   │
│   ├── ai/                         # langchaingo AI 编排（agent / orchestrator / callback / memory + tools/）
│   ├── auth/                       # JWT / RBAC（gorbac）/ Local / LDAP / OIDC / RSA / wire_crypto
│   ├── config/                     # envconfig + AES-256-GCM 加解密
│   ├── engine/                     # 规则引擎（pipeline / dedup / aggregation / inhibition / routing / escalation）
│   ├── httputil/                   # restful response 包装、字段脱敏
│   ├── incident/                   # Incident 生命周期 service / repository / timeline / staleness reaper
│   ├── ingestion/                  # 接入归一化 + Kafka manager + Kafka mapping (expr-lang) + 云适配
│   ├── label/                      # identity 权限常量集中地（incident / alert_center / ai / system / ingestSource …）
│   ├── model/                      # GORM 领域模型（incident / alert / route / policy / contact / llm_provider / …）
│   ├── notification/               # dispatcher + 渠道（dingtalk / feishu / slack / email / sms / voice）+ 联系人/模板
│   ├── realtime/                   # PG LISTEN → WebSocket 实时事件分发（hub / event / listener / pglisten）
│   ├── router/                     # go-restful 路由 + 中间件
│   │   ├── middleware/             #   auth.go / acl.go / audit.go / webhook_signature.go
│   │   ├── ai.go                   #   AI 触发 / 报告 / WebSocket / chat
│   │   ├── alert.go                #   POST /api/v1/alerts/*（标准入站）
│   │   ├── alert_center.go         #   路由 / 通知策略 / 联系人 / 静默 / 抑制 / 升级 / 模板
│   │   ├── data_sources.go         #   data_sources（Kafka / OpenSearch / Elastic / K8s / Prometheus）CRUD + 测试
│   │   ├── incident.go             #   Incident CRUD + 状态流转
│   │   ├── llm_providers.go        #   /llm-providers CRUD + set-default + test
│   │   ├── realtime.go             #   /api/v1/realtime/ws（topic 订阅）
│   │   ├── router.go               #   汇总 + StoreRouter 自动同步 endpoint
│   │   ├── system.go               #   登录 / 用户 / 角色 / 系统配置
│   │   └── webhook_sources.go      #   webhook_sources CRUD + rotate（一次性私钥下发）
│   ├── store/                      # postgres / redis / migrate（golang-migrate 嵌入式）
│   ├── sysconfig/                  # system_settings 服务（含 Bootstrap、加密、JWT/RSA 密钥）
│   ├── ticketing/                  # 工单集成（Jira / 飞书多维表格）
│   └── version/                    # 版本号常量与 String()
│
├── pkg/                            # 可导出公共包
│   ├── logger/                     # zerolog 初始化
│   └── metrics/                    # 自身 Prometheus 指标 + /healthz handler
│
├── web/                            # 前端 (React + TypeScript + Ant Design)
├── deploy/
│   ├── docker/                     # Dockerfile / docker-compose
│   ├── k8s/                        # Kubernetes 清单
│   └── helm/                       # Helm Chart
├── migrations/                     # golang-migrate SQL 文件
├── .golangci.yml                   # 统一 lint 基线
├── .env.example                    # 环境变量示例（提交 git）
└── README.md
```

## 单进程启动流程

`cmd/alertmesh/main.go` 是唯一入口，骨架如下（实际代码以仓库为准，避免文档漂移）：

1. `config.Load()` —— envconfig + .env 自动加载。
2. `logger.Init(cfg.LogLevel)` —— zerolog 全局初始化。
3. `rootCtx, rootCancel := context.WithCancel(...)` —— 应用级生命周期 context。
4. `InitApp(rootCtx, cfg)` —— google/wire 注入 DB / RBAC / Pipeline / Incident
   Service / AI Orchestrator / Realtime Hub / Router / HTTP Server。
5. `Orchestrator.StartWorkerPool(rootCtx)` + `Pipeline.StartReloadListener(rootCtx)`
   + `realtime.Start(rootCtx, …)` —— 启动后台 goroutine。
6. `incident.StartStalenessReaper(rootCtx, …)` —— Incident 闲置自动 resolve。
7. `ingestion.StartKafka(rootCtx, …)` —— `data_sources` 行驱动的 Kafka 消费 fleet
   （无 env 开关）；`cfg.K8sEnabled` 为真时启动 K8s Informer。
8. `app.Server.ListenAndServe()` —— HTTP 服务进入主循环。
9. SIGTERM / SIGINT 触发 `rootCancel()` + `Orchestrator.Stop()` + `Pipeline.Stop()`
   + `Server.Shutdown(15s)`，所有挂在 `rootCtx` 上的 goroutine（dispatcher、AI
   follow-up、Kafka reader、PG LISTEN、reaper）随之退出。

完整入口请直接看 [`cmd/alertmesh/main.go`](../cmd/alertmesh/main.go) —— 文档不再
内联 Go 代码片段，避免与实现漂移。
