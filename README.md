# AlertMesh

> 多源告警聚合 · 智能根因分析 · 全链路事件管理平台

AlertMesh 是一个**多源告警聚合与智能分发平台**，定位为**被动接收器**：

- 统一接收来自 Alertmanager、Prometheus、Webhook、Kafka、K8s Events、OpenSearch /
  Elastic、云监控等**已触发的告警**；
- **不执行** PromQL / KQL 规则评估——规则引擎仅处理入站告警的 labels / annotations；
- 参考 Alertmanager 的内存状态模型实现去重与聚合，**默认无需 Redis / Kafka
  作为基础设施**，预留扩展接口；
- 借助 [langchaingo](https://github.com/tmc/langchaingo) 驱动 AI 对 Incident 进行
  自动根因分析；
- 使用 [gorbac](https://github.com/mikespook/gorbac) 实现 RBAC，**接口权限即按钮
  权限**，前后端复用同一份 identity。

```mermaid
flowchart LR
    SRC["多源告警\nAlertmanager/K8s/RDS/Webhook/Kafka"]
    --> ING["接入归一化\nRawAlert"]
    --> ENG["规则引擎\n内存: 去重→路由→聚合→抑制→升级"]
    --> AG["AlertGroup"]
    --> INC["Incident Service\n生命周期·SLA·Timeline"]
    --> AI["AI Orchestrator\nlangchaingo ReAct Agent"]
    --> OUT["通知·工单·审批·Web UI"]
```

## 核心能力

| 能力 | 说明 |
|------|------|
| **多源接入** | Alertmanager v1/v2、Prometheus 直推、通用 Webhook（RFC 9421 签名）、Kafka、K8s Events、OpenSearch / Elastic、云监控 |
| **告警归一化** | 各来源格式统一映射为 `RawAlert`，Kafka mapping 支持 gjson 路径与 `expr:` 表达式双语法 |
| **规则引擎** | 内存实现去重、聚合、抑制、路由、升级，无外部依赖 |
| **Incident 生命周期 v3** | Open → Ack → In Progress → Resolved → Closed，1/3/5min 线性递增 + 1h 升级阶梯 P3→P0 |
| **消息通知策略 v3** | dispatcher 三段式（resolveRecipients → groupByChannelTarget → dispatchBuckets），P0 默认走 SMS / 语音 |
| **AI 根因分析** | langchaingo ReAct Agent，自动调用 metrics / logs / sysinfo / changes / runbook 5 个 Tool，WebSocket 流式推送 |
| **统一权限** | gorbac RBAC，接口权限 = 按钮权限，identity 自动同步到 `endpoints` |
| **认证** | 本地账号、LDAP（含 group → role 映射）、OIDC / SSO |

## 部署

### 方式一：Docker Compose（推荐）

**前提：** 服务器已安装 Docker >= 24 和 Docker Compose >= 2.20。

```bash
# 1. 拉取代码
git clone <repo-url> /opt/alertmesh
cd /opt/alertmesh

# 2. 生成加密密钥并配置环境变量
cp .env.example .env
openssl rand -base64 32   # 复制输出值填入 ALERTMESH_ENCRYPTION_KEY
vim .env
# 必须修改的两项：
#   ALERTMESH_DATABASE_DSN=postgres://alertmesh:强密码@postgres:5432/alertmesh?sslmode=disable
#   ALERTMESH_ENCRYPTION_KEY=<上面 openssl 命令的输出>

# 3. 一键启动（自动拉起 PostgreSQL，首次启动自动执行数据库迁移）
docker compose -f deploy/docker/docker-compose.yml up -d

# 4. 查看启动日志（确认无报错，并获取 admin 初始密码）
docker compose -f deploy/docker/docker-compose.yml logs -f alertmesh

# 5. 验证服务健康
curl http://localhost:8080/healthz
# 返回 {"status":"ok"} 表示启动成功

# 6. 访问 Web UI
# http://<服务器IP>:8080
# 默认账号：admin / 密码见上面日志输出
```

> 数据库迁移由后端启动时**自动执行**，无需手动运行任何 migrate 命令。

---

### 方式二：裸机手动部署（Ubuntu / Debian）

#### 1. 安装系统依赖

```bash
apt-get update && apt-get install -y curl git openssl

# ── 安装 Go 1.22+ ──────────────────────────────────────────────────
wget https://go.dev/dl/go1.22.5.linux-amd64.tar.gz
tar -C /usr/local -xzf go1.22.5.linux-amd64.tar.gz
echo 'export PATH=$PATH:/usr/local/go/bin' >> /etc/profile
source /etc/profile
go version   # 确认输出 go1.22.x

# ── 安装 Node.js 20+ ───────────────────────────────────────────────
curl -fsSL https://deb.nodesource.com/setup_20.x | bash -
apt-get install -y nodejs
node -v   # 确认输出 v20.x

# ── 安装 PostgreSQL 15+ ────────────────────────────────────────────
apt-get install -y postgresql
systemctl enable postgresql && systemctl start postgresql
```

#### 2. 初始化数据库

```bash
sudo -u postgres psql << 'SQL'
CREATE USER alertmesh WITH PASSWORD '替换为强密码';
CREATE DATABASE alertmesh OWNER alertmesh;
\q
SQL

# 验证连接
psql -U alertmesh -h localhost -d alertmesh -c "SELECT 1;"
```

#### 3. 拉取代码

```bash
git clone <repo-url> /opt/alertmesh
cd /opt/alertmesh
```

#### 4. 配置环境变量

```bash
cp .env.example .env
vim .env
```

`.env` 关键配置项：

```dotenv
# ── 必填 ──────────────────────────────────────────────────────────
ALERTMESH_DATABASE_DSN=postgres://alertmesh:替换为强密码@localhost:5432/alertmesh?sslmode=disable
ALERTMESH_ENCRYPTION_KEY=<执行 openssl rand -base64 32 的输出>   # ⚠️ 生成后不可更改

# ── 基础配置 ──────────────────────────────────────────────────────
ALERTMESH_SERVER_PORT=8080       # HTTP 监听端口
ALERTMESH_LOG_LEVEL=info         # info / debug / warn
ALERTMESH_LOG_FORMAT=json        # json（生产） / pretty（调试）
ALERTMESH_AI_WORKERS=2           # AI 分析并发 worker 数

# ── 可选：AI 根因分析数据源 ────────────────────────────────────────
# ALERTMESH_PROMETHEUS_URL=http://prometheus:9090
# ALERTMESH_OPENSEARCH_URL=http://opensearch:9200

# ── 可选：Redis 缓存 ───────────────────────────────────────────────
# ALERTMESH_REDIS_ENABLED=true
# ALERTMESH_REDIS_ADDR=localhost:6379

# ── 可选：Nginx 配置下发（运维操作功能） ───────────────────────────
# NGINX_WORK_DIR=/opt/alertmesh/work
# ALERTMESH_ANSIBLE_USER=root
# ALERTMESH_ANSIBLE_PASSWORD=ssh密码
# ALERTMESH_ANSIBLE_NGINX_BIN=/usr/sbin/nginx
```

#### 5. 编译后端

```bash
cd /opt/alertmesh
go build -trimpath -ldflags "-s -w" -o alertmesh ./cmd/alertmesh/
ls -lh alertmesh   # 确认二进制文件存在
```

#### 6. 构建前端

```bash
cd /opt/alertmesh/web
npm install
npx vite build        # ⚠️ 不要用 npm run build，会做 TS 类型检查可能报错
ls dist/              # 确认 index.html 存在
cd /opt/alertmesh
```

#### 7. 配置 systemd 服务（开机自启）

```bash
# 创建专用运行用户
useradd -r -s /sbin/nologin alertmesh
mkdir -p /var/log/alertmesh
chown -R alertmesh:alertmesh /opt/alertmesh /var/log/alertmesh

# 创建 systemd 服务文件
cat > /etc/systemd/system/alertmesh.service << 'EOF'
[Unit]
Description=AlertMesh Alert Platform
After=network.target postgresql.service

[Service]
Type=simple
User=alertmesh
WorkingDirectory=/opt/alertmesh
EnvironmentFile=/opt/alertmesh/.env
ExecStart=/opt/alertmesh/alertmesh
Restart=on-failure
RestartSec=5s
StandardOutput=journal
StandardError=journal

[Install]
WantedBy=multi-user.target
EOF

# 启动服务
systemctl daemon-reload
systemctl enable alertmesh
systemctl start alertmesh

# 查看启动状态
systemctl status alertmesh
journalctl -u alertmesh -f   # 实时日志（获取 admin 初始密码）
```

#### 8. 验证服务

```bash
# 后端健康检查
curl http://localhost:8080/healthz
# 预期：{"status":"ok"}

# 查看监听端口
ss -tlnp | grep 8080
```

---

### Nginx 反向代理配置

生产环境推荐在 alertmesh 前挂 Nginx，统一处理静态文件、HTTPS 和 WebSocket：

```nginx
server {
    listen 80;
    server_name your-domain.com;
    # HTTPS 时将 80 重定向：
    # return 301 https://$host$request_uri;

    # 前端静态文件
    root /opt/alertmesh/web/dist;
    index index.html;

    # SPA 路由 fallback（必须，否则刷新页面 404）
    location / {
        try_files $uri $uri/ /index.html;
    }

    # API 反向代理
    location /api/ {
        proxy_pass http://127.0.0.1:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_read_timeout 300s;
    }

    # WebSocket（容器终端 + SSE 流式推送，必须单独配置）
    location /ws {
        proxy_pass http://127.0.0.1:8080;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_set_header Host $host;
        proxy_read_timeout 3600s;
    }
}
```

```bash
nginx -t && systemctl reload nginx
```

---

### 升级（后续版本更新）

```bash
cd /opt/alertmesh

# 1. 拉取最新代码
git pull

# 2. 重新编译后端
go build -trimpath -ldflags "-s -w" -o alertmesh ./cmd/alertmesh/

# 3. 重新构建前端
cd web && npm install && npx vite build && cd ..

# 4. 重启服务（数据库迁移自动执行）
systemctl restart alertmesh
systemctl status alertmesh
```

---

### 注意事项

| 事项 | 说明 |
|------|------|
| **ENCRYPTION_KEY 不可更改** | 生产部署后必须妥善保管，修改后所有加密数据将无法解密 |
| **数据库备份** | 建议配置 PostgreSQL 定时备份：`pg_dump alertmesh > backup.sql` |
| **防火墙** | 对外只开放 80/443，后端端口（8080）不要直接暴露到公网 |
| **首次登录密码** | 启动日志中会输出 admin 初始密码，登录后立即在「个人设置」中修改 |
| **前端构建命令** | 务必用 `npx vite build`，不要用 `npm run build`（会做 TS 类型检查） |

## 文档索引

| 主题 | 文档 |
|------|------|
| 整体架构 / HA / 部署拓扑 / 技术选型 | [docs/architecture.md](docs/architecture.md) |
| 告警消息归一化、路由、聚合、抑制、升级 | [docs/data-flow.md](docs/data-flow.md) |
| 告警生命周期 v3 与收敛策略 | [docs/lifecycle.md](docs/lifecycle.md) |
| 数据源详解（Prometheus / Webhook / Kafka / OpenSearch / Elastic / K8s） | [docs/data-sources.md](docs/data-sources.md) |
| 权限模型（gorbac + endpoint 自动同步 + LDAP 组映射） | [docs/permissions.md](docs/permissions.md) |
| AI 编排（langchaingo / ReAct Agent / 5 个 Tool / Memory） | [docs/ai.md](docs/ai.md) |
| GORM 数据模型 | [docs/data-model.md](docs/data-model.md) |
| 目录结构与单进程启动流程 | [docs/directory.md](docs/directory.md) |
| Roadmap（已交付项已勾选） | [docs/roadmap.md](docs/roadmap.md) |

新加入项目？请先看 [`ARCHITECTURE.md`](ARCHITECTURE.md)（一页纸索引），再按需
深入；动手前请阅读 [`CONTRIBUTING.md`](CONTRIBUTING.md)。

## License

MIT License © 2026 AlertMesh Contributors
