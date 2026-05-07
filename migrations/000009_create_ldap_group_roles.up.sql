CREATE TABLE IF NOT EXISTS ldap_group_roles (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    ldap_group VARCHAR(255) NOT NULL UNIQUE,
    role_name  VARCHAR(32)  NOT NULL,
    created_at TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ  NOT NULL DEFAULT now(),
    deleted_at TIMESTAMPTZ
);
