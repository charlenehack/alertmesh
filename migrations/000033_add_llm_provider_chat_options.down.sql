ALTER TABLE llm_providers
    DROP CONSTRAINT IF EXISTS llm_providers_language_check;

ALTER TABLE llm_providers
    DROP COLUMN IF EXISTS language,
    DROP COLUMN IF EXISTS chat_report_max_chars,
    DROP COLUMN IF EXISTS chat_history_max_turns;
