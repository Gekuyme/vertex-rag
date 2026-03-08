ALTER TABLE user_settings
  DROP COLUMN IF EXISTS llm_model,
  DROP COLUMN IF EXISTS llm_provider;
