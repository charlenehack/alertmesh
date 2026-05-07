-- Per-provider AI behaviour knobs, surfaced in the admin UI under
-- 系统管理 → AI 大模型配置 (see internal/router/llm_providers.go and
-- web/src/pages/settings/LLMProviders.tsx).
--
-- language               – output language for AI analysis & chat replies.
--                          'zh'   = always 简体中文 (default)
--                          'en'   = always English
--                          'auto' = follow user's input language
-- chat_report_max_chars  – upper bound on how many chars of the previous
--                          root-cause report we re-feed into a follow-up
--                          chat turn (avoid blowing the context window of
--                          smaller models).  0 / NULL → use process default.
-- chat_history_max_turns – upper bound on how many user/assistant pairs of
--                          ai_conversations we re-feed.  0 / NULL → use
--                          process default.

ALTER TABLE llm_providers
    ADD COLUMN IF NOT EXISTS language               VARCHAR(8) NOT NULL DEFAULT 'zh',
    ADD COLUMN IF NOT EXISTS chat_report_max_chars  INT        NOT NULL DEFAULT 8000,
    ADD COLUMN IF NOT EXISTS chat_history_max_turns INT        NOT NULL DEFAULT 10;

-- Defensive constraint so a misclick in the form can't store nonsense.
ALTER TABLE llm_providers
    DROP CONSTRAINT IF EXISTS llm_providers_language_check;
ALTER TABLE llm_providers
    ADD  CONSTRAINT llm_providers_language_check
    CHECK (language IN ('zh', 'en', 'auto'));
