-- Insert default admin user (username: admin, password: admin123)
-- Password hash is bcrypt cost=10 of "admin123"
INSERT INTO users (username, email, display_name, password_hash, source, is_active)
VALUES (
    'admin',
    'admin@alertmesh.local',
    'Administrator',
    '$2a$10$jmY20by67GA2LxPJPk64W./RisaoMGeEM.zHT4xRlWGOCRf0yQUGe',
    'local',
    true
)
ON CONFLICT (username) DO NOTHING;

-- Assign admin role to the admin user
INSERT INTO user_roles (user_id, role_id)
SELECT u.id, r.id
FROM users u, roles r
WHERE u.username = 'admin' AND r.name = 'admin'
ON CONFLICT DO NOTHING;
