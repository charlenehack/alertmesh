-- nginx_server_groups: 生产/灰度等 Nginx 目标服务器分组
CREATE TABLE IF NOT EXISTS nginx_server_groups (
    id         BIGSERIAL    PRIMARY KEY,
    name       VARCHAR(64)  NOT NULL,           -- e.g. "生产", "灰度"
    env        VARCHAR(32)  NOT NULL,           -- "prod" | "gray"
    servers    JSONB        NOT NULL DEFAULT '[]', -- [{ip, label, port}]
    created_at TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

-- 默认插入两条记录
INSERT INTO nginx_server_groups (name, env, servers) VALUES
    ('生产', 'prod', '[]'),
    ('灰度', 'gray', '[]')
ON CONFLICT DO NOTHING;
