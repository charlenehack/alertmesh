INSERT INTO roles (name, description, parents, status) VALUES
    ('管理员', '系统管理员，拥有全部权限',     '[]', true),
    ('开发',   '开发人员，可查看和配置服务',   '[]', true),
    ('运维',   '运维人员，可执行运维操作',     '[]', true),
    ('测试',   '测试人员，可查看告警和数据',   '[]', true),
    ('其它',   '默认角色，仅基础访问',         '[]', true)
ON CONFLICT (name) DO NOTHING;
