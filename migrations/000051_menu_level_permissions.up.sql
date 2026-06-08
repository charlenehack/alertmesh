-- 菜单级权限收敛：清空旧的细粒度 role_endpoints，待服务重启后 StoreRouter
-- 自动注册新的菜单级 endpoints，管理员保持全权（ACL 层管理员直接放行），
-- 其余角色的 role_endpoints 已清空，可在 UI 中重新配置。

-- 1. 清空所有角色与旧细粒度权限点的绑定
DELETE FROM role_endpoints;

-- 2. 清空旧细粒度权限点（服务重启后 StoreRouter 自动写入新的菜单级权限点）
DELETE FROM endpoints;
