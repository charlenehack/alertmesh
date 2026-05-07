INSERT INTO roles (name, description, parents, status) VALUES
    ('guest',      'Default role for new users',  '[]',            true),
    ('member',     'Standard member',             '["guest"]',     true),
    ('oncall',     'On-call responder',           '["member"]',    true),
    ('admin',      'Platform administrator',      '["oncall"]',    true),
    ('superadmin', 'Full access',                 '["admin"]',     true)
ON CONFLICT (name) DO NOTHING;
