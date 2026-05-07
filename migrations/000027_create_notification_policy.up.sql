-- Notification policies (告警通知策略)
CREATE TABLE IF NOT EXISTS notification_policies (
    id          UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    name        VARCHAR(255) NOT NULL,
    severities  JSONB        NOT NULL DEFAULT '[]',
    description TEXT,
    contact_ids JSONB        NOT NULL DEFAULT '[]',
    group_ids   JSONB        NOT NULL DEFAULT '[]',
    is_enabled  BOOLEAN      NOT NULL DEFAULT true,
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ  NOT NULL DEFAULT now(),
    deleted_at  TIMESTAMPTZ,
    UNIQUE (name)
);

-- Notification contacts (联系人)
-- Secret fields (webhook_token, slack_bot_token, feishu_secret, dingtalk_secret)
-- are AES-256-GCM encrypted at rest by the application layer.
CREATE TABLE IF NOT EXISTS notification_contacts (
    id                UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    name              VARCHAR(255) NOT NULL,
    email             VARCHAR(255),
    phone             VARCHAR(32),
    webhook_url       TEXT,
    webhook_token     TEXT,
    slack_bot_token   TEXT,
    slack_channel_id  VARCHAR(255),
    feishu_webhook    TEXT,
    feishu_secret     TEXT,
    dingtalk_webhook  TEXT,
    dingtalk_secret   TEXT,
    created_at        TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ  NOT NULL DEFAULT now(),
    deleted_at        TIMESTAMPTZ,
    UNIQUE (name)
);

-- Notification contact groups (联系人组)
CREATE TABLE IF NOT EXISTS notification_contact_groups (
    id          UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    name        VARCHAR(255) NOT NULL,
    description TEXT,
    contact_ids JSONB        NOT NULL DEFAULT '[]',
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ  NOT NULL DEFAULT now(),
    deleted_at  TIMESTAMPTZ,
    UNIQUE (name)
);
