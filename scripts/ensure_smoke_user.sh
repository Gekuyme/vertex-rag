#!/usr/bin/env bash

set -euo pipefail

SMOKE_USER_EMAIL="${SMOKE_USER_EMAIL:-smoke@vertex.local}"
SMOKE_USER_PASSWORD="${SMOKE_USER_PASSWORD:-Password123!}"
SMOKE_ORG_NAME="${SMOKE_ORG_NAME:-Vertex Demo}"
SMOKE_ROLE_NAME="${SMOKE_ROLE_NAME:-Owner}"

echo "==> Hash password for smoke user"
password_hash="$(cd apps/api && go run ./cmd/hashpassword "${SMOKE_USER_PASSWORD}")"

if [[ -z "$password_hash" ]]; then
  echo "failed to generate password hash" >&2
  exit 1
fi

echo "==> Upsert smoke user ${SMOKE_USER_EMAIL} in org ${SMOKE_ORG_NAME}"
docker compose --env-file .env -f deploy/compose/docker-compose.yml -f deploy/compose/docker-compose.dev.yml exec -T postgres psql -U vertex -d vertex_rag <<SQL
WITH target_org AS (
  SELECT id FROM organizations WHERE name = '${SMOKE_ORG_NAME}' LIMIT 1
),
target_role AS (
  SELECT r.id, r.org_id
  FROM roles r
  JOIN target_org o ON o.id = r.org_id
  WHERE r.name = '${SMOKE_ROLE_NAME}'
  LIMIT 1
)
INSERT INTO users (org_id, email, password_hash, role_id, status)
SELECT org_id, '${SMOKE_USER_EMAIL}', '${password_hash}', id, 'active'
FROM target_role
ON CONFLICT (email)
DO UPDATE SET
  org_id = EXCLUDED.org_id,
  password_hash = EXCLUDED.password_hash,
  role_id = EXCLUDED.role_id,
  status = 'active',
  updated_at = NOW();
SQL

echo "Smoke user is ready:"
echo "  email=${SMOKE_USER_EMAIL}"
echo "  password=${SMOKE_USER_PASSWORD}"
