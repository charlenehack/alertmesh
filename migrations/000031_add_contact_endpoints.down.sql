ALTER TABLE notification_contacts
    DROP COLUMN IF EXISTS webhook_url,
    DROP COLUMN IF EXISTS webhook_token,
    DROP COLUMN IF EXISTS slack_bot_token,
    DROP COLUMN IF EXISTS slack_channel_id,
    DROP COLUMN IF EXISTS feishu_webhook,
    DROP COLUMN IF EXISTS feishu_secret,
    DROP COLUMN IF EXISTS dingtalk_webhook,
    DROP COLUMN IF EXISTS dingtalk_secret;
