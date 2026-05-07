-- Backfill endpoint columns on notification_contacts.
-- Required for installations whose 000027 migration ran with an earlier
-- version of the schema that lacked the webhook / IM endpoint fields.
-- All statements are idempotent.

ALTER TABLE notification_contacts
    ADD COLUMN IF NOT EXISTS webhook_url      TEXT,
    ADD COLUMN IF NOT EXISTS webhook_token    TEXT,
    ADD COLUMN IF NOT EXISTS slack_bot_token  TEXT,
    ADD COLUMN IF NOT EXISTS slack_channel_id VARCHAR(255),
    ADD COLUMN IF NOT EXISTS feishu_webhook   TEXT,
    ADD COLUMN IF NOT EXISTS feishu_secret    TEXT,
    ADD COLUMN IF NOT EXISTS dingtalk_webhook TEXT,
    ADD COLUMN IF NOT EXISTS dingtalk_secret  TEXT;
