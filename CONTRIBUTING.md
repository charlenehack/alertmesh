# Contributing to AlertMesh

欢迎给 AlertMesh 贡献代码！为了让 review 顺利、CI 一次过，请遵守以下约定。

## 1. 本地开发环境

| 工具 | 版本 | 说明 |
|------|------|------|
| Go | 1.22+ | `go.mod` 锁定 |
| Node.js | 20+ | 跑 `web/` 前端用 pnpm |
| PostgreSQL | 15+ | 默认依赖，本地可用 docker compose 拉起 |
| `golang-migrate` | latest | 数据库迁移工具 |
| `golangci-lint` | v2.5.0+ | 代码静态检查 |

```bash
# 一次性安装本机依赖（macOS 示例）
brew install go node pnpm postgresql golang-migrate
go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.5.0
```

## 2. 三件套：build / test / lint

每次提交前请确保：

```bash
make build    # go build ./cmd/alertmesh
make test     # go test ./... -v -cover
make lint     # golangci-lint run ./...
```

三个命令都必须 **退出码 0**。`make lint` 启用的 linter 见
[`.golangci.yml`](.golangci.yml)（errcheck / govet / staticcheck / errorlint /
gocritic / gocyclo / gosec / revive / unparam / misspell / unused / ineffassign）。
若需要 `//nolint:xxx` 豁免，**必须**在同一行末尾附说明原因，例如：

```go
//nolint:gocyclo // intentional state machine, see docs/data-sources.md
func (k *KafkaProgram) Apply(...) {...}
```

## 3. 提交规范

- 一个 commit 只解决一类问题；新增功能 + lint 修复请拆开两个 commit。
- 提交信息建议遵循 [Conventional Commits](https://www.conventionalcommits.org/)：

  ```
  feat(notification): add SMS channel
  fix(kafka): respect consumer_concurrency upper bound
  refactor(incident): extract repeat schedule into v3 struct
  docs(readme): split README into docs/
  ```

- 任何能从 commit 推出的内容（`why`、相关 issue / PR、潜在影响面）请放在
  commit body 里，便于以后回溯。

## 4. 测试期望

- **新代码必须带 `_test.go`**，至少覆盖：
  - 1 条 happy path；
  - 1 条错误分支（`fmt.Errorf` 出错的路径需要被断言）。
- 表驱动测试优先（`tests := []struct{ name string; ...; want ... }{...}`），
  CI 输出能直接定位失败用例。
- 涉及并发 / goroutine 的修改，请加 `go test -race ./...` 验证；
  典型例子见 [`internal/engine/`](internal/engine) 的 `dedup` / `routing` 测试。
- 涉及 Kafka filter / mapping 的修改，跑
  [`internal/ingestion/kafka_filter_test.go`](internal/ingestion/kafka_filter_test.go)
  全部用例并补 1 条专属测试。

新增测试包未达成的现实不阻塞 PR，但请在 PR 描述里点名"暂未补测"，方便 review
时一起讨论。

## 5. 配置 / 数据 / API 兼容性

- 不动 SQL migration 已发布的内容（往前加新文件即可，编号顺延）；如确需修改
  历史 migration，请同时给出向上 / 向下脚本与回滚说明。
- 不动 `data_sources.config` JSON 的 schema：v3 引入 `expr:` 前缀正是为了避免
  schema 变更，新功能尽量沿用此模式。
- 不破坏现有 REST API 的 request/response 形状；如必须破坏，请在 PR 描述里
  注明影响的前端页面与后端调用方。
- 不改 `.env` schema 的环境变量含义；新增 env 必须给默认值与 `.env.example`
  同步更新。

## 6. 文档同步

代码变动通常对应一份文档：

| 变动范围 | 同步更新 |
|---------|---------|
| 新增 / 修改 REST API | [`docs/permissions.md`](docs/permissions.md) 的 identity 表 |
| 新增数据源 kind / mapping 行为 | [`docs/data-sources.md`](docs/data-sources.md) |
| 修改 Incident 状态机 / 重复调度 | [`docs/lifecycle.md`](docs/lifecycle.md) |
| 修改 GORM 模型 | [`docs/data-model.md`](docs/data-model.md) |
| 引入新依赖 / 改架构 | [`docs/architecture.md`](docs/architecture.md) |
| 已交付 Roadmap 项 | 把 [`docs/roadmap.md`](docs/roadmap.md) 对应 `[ ]` 改成 `[x]` |

`README.md` 是产品门面，**只放 quick start + 索引**——长内容请进 `docs/`。

## 7. PR Checklist

提 PR 前自检：

- [ ] `make build` / `make test` / `make lint` 全部退出码 0
- [ ] 新代码带表驱动测试（happy + 1 error path）
- [ ] 涉及并发的代码加了 `-race` 验证
- [ ] 没有引入未声明的 `fmt.Errorf("...: %v", err)`（用 `%w` 包装）
- [ ] 修改了配置 / API / 数据模型的，对应文档同步更新
- [ ] commit message 符合 Conventional Commits
- [ ] PR 描述里注明影响范围与回滚方案

Reviewer 看到清单全勾就可以进入正常 review 流程。感谢贡献！
