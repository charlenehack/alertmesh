# 核心数据模型

AlertMesh 的领域模型以 GORM 在 PostgreSQL 上落地。下面给出最核心的几张表的字段轮廓
（生产实现以 [`internal/model/`](../internal/model) 包下的真实定义为准）。

```go
// GORM 模型（PostgreSQL）

type Incident struct {
    ID         string         `gorm:"primaryKey;type:uuid;default:gen_random_uuid()"`
    Title      string         `gorm:"not null"`
    Severity   string         `gorm:"not null;index"` // P0/P1/P2/P3
    Status     string         `gorm:"not null;index"` // open/ack/in_progress/resolved/closed
    Source     string         `gorm:"not null"`
    Labels     datatypes.JSON `gorm:"type:jsonb"`
    GroupKey   string         `gorm:"index"`          // = AlertGroup.GroupKey
    RouteID    *string        `gorm:"type:uuid;index"` // 命中的 alert_routes.id
    AssigneeID *string
    OpenedAt   time.Time      `gorm:"autoCreateTime"`
    AckedAt    *time.Time
    ResolvedAt *time.Time
    AIStatus   string  `gorm:"default:'pending'"` // pending/running/done/failed
    AIReportID *string `gorm:"type:uuid"`
    gorm.Model
}

type Alert struct {
    ID          string         `gorm:"primaryKey;type:uuid;default:gen_random_uuid()"`
    IncidentID  string         `gorm:"not null;index"`
    Source      string         `gorm:"not null"`
    Fingerprint string         `gorm:"not null;index"`
    Labels      datatypes.JSON `gorm:"type:jsonb"`
    Annotations datatypes.JSON `gorm:"type:jsonb"`
    StartsAt    time.Time
    EndsAt      *time.Time
    Status      string `gorm:"not null"` // firing/resolved
    RawPayload  []byte `gorm:"type:jsonb"`
}

type User struct {
    ID          string  `gorm:"primaryKey;type:uuid;default:gen_random_uuid()"`
    Username    string  `gorm:"uniqueIndex;not null"`
    Email       string  `gorm:"uniqueIndex"`
    DisplayName string
    Source      string  `gorm:"not null"` // local/ldap/oidc
    ExternalID  string  `gorm:"index"`    // LDAP DN / OIDC sub
    Roles       []*Role `gorm:"many2many:user_roles"`  // 多角色
    IsActive    bool    `gorm:"default:true"`
    LastLoginAt *time.Time
    gorm.Model
}

type LLMProvider struct {
    ID          string  `gorm:"primaryKey;type:uuid;default:gen_random_uuid()"`
    Name        string  `gorm:"not null"`
    Provider    string  `gorm:"not null"` // openai/azure/ollama/anthropic
    BaseURL     string
    APIKey      string  `gorm:"not null"` // AES-256 加密存储
    Model       string  `gorm:"not null"`
    Temperature float32 `gorm:"default:0.1"`
    IsDefault   bool    `gorm:"default:false"`
    IsEnabled   bool    `gorm:"default:true"`
    gorm.Model
}

// 客户端拉取源（4.1.4）：Kafka / OpenSearch / K8s Events / 云监控的统一配置
type AlertIngestSource struct {
    ID         string         `gorm:"primaryKey;type:uuid;default:gen_random_uuid()"`
    Name       string         `gorm:"uniqueIndex;not null"`     // = RawAlert.Source
    Kind       string         `gorm:"not null;index"`           // kafka/opensearch/k8s_events/cloud_aliyun/...
    IsEnabled  bool           `gorm:"not null;default:false"`
    Connection datatypes.JSON `gorm:"type:jsonb;not null"`      // 含敏感字段，AES-256-GCM 加密落库
    Selector   datatypes.JSON `gorm:"type:jsonb;not null"`      // { match, match_re, drop, query }
    Mapping    datatypes.JSON `gorm:"type:jsonb;not null"`      // { fingerprint_from, labels, annotations, starts_at, status_from }
    Runtime    datatypes.JSON `gorm:"type:jsonb"`               // { poll_interval, watermark_field, max_per_second, ... }
    LastCursor *string                                          // OpenSearch watermark 等增量游标
    LastError  *string
    LastRunAt  *time.Time
    gorm.Model
}
```

> 这里只列了最核心的几张表。完整模型（路由、通知策略、联系人、AI 分析、Timeline、
> RBAC、审计日志、Webhook 源、Kafka mapping 等）请直接看 [`internal/model/`](../internal/model)
> 与 [`migrations/`](../migrations) 下的 SQL 文件。
