UPDATE roles
SET permissions = COALESCE(
  (
    SELECT jsonb_agg(permission ORDER BY sort_key)
    FROM (
      SELECT value AS permission, ordinality AS sort_key
      FROM jsonb_array_elements_text(roles.permissions) WITH ORDINALITY
      WHERE value <> 'can_use_unstrict'
    ) filtered
  ),
  '[]'::jsonb
)
WHERE permissions ? 'can_use_unstrict';
