#!/usr/bin/env bash

set -euo pipefail

API_BASE_URL="${API_BASE_URL:-http://localhost:8080}"
PASSWORD="${SMOKE_PASSWORD:-Password123!}"

require_bin() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "missing required binary: $1" >&2
    exit 1
  fi
}

require_bin curl
require_bin jq

timestamp="$(date +%s)"
email="acl_smoke_${timestamp}@example.com"
org_name="ACL Smoke Org ${timestamp}"
token_key="acl_token_${timestamp}_vertex"
document_file="$(mktemp)"

cleanup() {
  rm -f "$document_file"
}
trap cleanup EXIT

echo "==> Register owner"
register_response="$(curl -fsS -X POST "${API_BASE_URL}/auth/register_owner" \
  -H "Content-Type: application/json" \
  -d "{\"organization_name\":\"${org_name}\",\"email\":\"${email}\",\"password\":\"${PASSWORD}\"}")"

token="$(printf '%s' "$register_response" | jq -r '.access_token')"
user_id="$(printf '%s' "$register_response" | jq -r '.user.id')"
owner_role_id="$(printf '%s' "$register_response" | jq -r '.user.role_id')"

if [[ -z "$token" || "$token" == "null" ]]; then
  echo "failed to register owner and get token" >&2
  exit 1
fi

echo "==> Create restricted role"
role_response="$(curl -fsS -X POST "${API_BASE_URL}/admin/roles" \
  -H "Authorization: Bearer ${token}" \
  -H "Content-Type: application/json" \
  -d '{"name":"RestrictedReader","permissions":["can_manage_users","can_manage_roles"]}')"
restricted_role_id="$(printf '%s' "$role_response" | jq -r '.id')"

if [[ -z "$restricted_role_id" || "$restricted_role_id" == "null" ]]; then
  echo "failed to create role" >&2
  exit 1
fi

cat >"$document_file" <<EOF
Internal ACL smoke knowledge base document.
Verification token: ${token_key}
Only users with RestrictedReader role should retrieve this chunk.
EOF

echo "==> Upload document with restricted role access"
upload_response="$(curl -fsS -X POST "${API_BASE_URL}/documents/upload" \
  -H "Authorization: Bearer ${token}" \
  -F "file=@${document_file};type=text/plain" \
  -F "title=ACL Restricted Smoke Document" \
  -F "allowed_role_ids=${restricted_role_id}")"
document_id="$(printf '%s' "$upload_response" | jq -r '.id')"

if [[ -z "$document_id" || "$document_id" == "null" ]]; then
  echo "failed to upload document" >&2
  exit 1
fi

echo "==> Wait for ingestion ready"
for _ in $(seq 1 90); do
  status="$(curl -fsS "${API_BASE_URL}/documents" \
    -H "Authorization: Bearer ${token}" | jq -r --arg doc_id "$document_id" '.documents[] | select(.id==$doc_id) | .status')"

  if [[ "$status" == "ready" ]]; then
    break
  fi
  if [[ "$status" == "failed" ]]; then
    echo "document ingestion failed" >&2
    exit 1
  fi
  sleep 2
done

if [[ "$status" != "ready" ]]; then
  echo "document did not become ready in time (status=${status:-unknown})" >&2
  exit 1
fi

create_chat() {
  curl -fsS -X POST "${API_BASE_URL}/chats" \
    -H "Authorization: Bearer ${token}" \
    -H "Content-Type: application/json" \
    -d '{"title":"ACL smoke check"}' | jq -r '.id'
}

ask_citations_len() {
  local chat_id="$1"
  curl -fsS -X POST "${API_BASE_URL}/chats/${chat_id}/messages" \
    -H "Authorization: Bearer ${token}" \
    -H "Content-Type: application/json" \
    -d "{\"content\":\"${token_key}\",\"mode\":\"strict\",\"top_k\":8,\"candidate_k\":32}" | jq -r '.citations | length'
}

echo "==> Verify no access before role switch"
chat_before="$(create_chat)"
citations_before="$(ask_citations_len "$chat_before")"
if [[ "$citations_before" -ne 0 ]]; then
  echo "expected 0 citations before role switch, got ${citations_before}" >&2
  exit 1
fi

echo "==> Switch owner to restricted role"
curl -fsS -X PATCH "${API_BASE_URL}/admin/users/${user_id}/role" \
  -H "Authorization: Bearer ${token}" \
  -H "Content-Type: application/json" \
  -d "{\"role_id\":${restricted_role_id}}" >/dev/null

echo "==> Verify access after role switch"
chat_after="$(create_chat)"
citations_after="$(ask_citations_len "$chat_after")"
if [[ "$citations_after" -le 0 ]]; then
  echo "expected citations after role switch, got ${citations_after}" >&2
  exit 1
fi

echo "==> Restore owner role"
curl -fsS -X PATCH "${API_BASE_URL}/admin/users/${user_id}/role" \
  -H "Authorization: Bearer ${token}" \
  -H "Content-Type: application/json" \
  -d "{\"role_id\":${owner_role_id}}" >/dev/null

echo "ACL smoke passed: before=${citations_before}, after=${citations_after}"
