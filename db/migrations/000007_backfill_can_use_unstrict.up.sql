UPDATE roles
SET permissions = (
  SELECT jsonb_agg(permission ORDER BY sort_key)
  FROM (
    SELECT permission, MIN(sort_key) AS sort_key
    FROM (
      SELECT value AS permission, ordinality AS sort_key
      FROM jsonb_array_elements_text(roles.permissions) WITH ORDINALITY
      UNION ALL
      SELECT 'can_use_unstrict' AS permission, 1000000 AS sort_key
    ) merged
    GROUP BY permission
  ) deduped
)
WHERE (
  permissions ? 'can_toggle_web_search'
  OR name IN ('Owner', 'Admin')
)
AND NOT permissions ? 'can_use_unstrict';
