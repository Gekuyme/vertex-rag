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
email="retrieval_smoke_${timestamp}@example.com"
org_name="Retrieval Smoke Org ${timestamp}"
stable_token="stable_token_${timestamp}_vertex"
restricted_token="restricted_token_${timestamp}_vertex"
document_file="$(mktemp)"
restricted_file="$(mktemp)"
common_term_file="$(mktemp)"

cleanup() {
  rm -f "$document_file" "$restricted_file" "$common_term_file"
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

cat >"$document_file" <<EOF
Retrieval stability smoke document.
Token: ${stable_token}
This chunk should be returned consistently for the same query.
EOF

echo "==> Upload baseline document"
upload_response="$(curl -fsS -X POST "${API_BASE_URL}/documents/upload" \
  -H "Authorization: Bearer ${token}" \
  -F "file=@${document_file};type=text/plain" \
  -F "title=Retrieval Stability Document" \
  -F "allowed_role_ids=${owner_role_id}")"
stable_document_id="$(printf '%s' "$upload_response" | jq -r '.id')"

if [[ -z "$stable_document_id" || "$stable_document_id" == "null" ]]; then
  echo "failed to upload baseline document" >&2
  exit 1
fi

cat >"$common_term_file" <<EOF
Go string definition smoke document.
Строка в Go - это неизменяемая последовательность байтов.
Этот фрагмент должен находиться по запросу "что такое строка".
EOF

echo "==> Upload common-term document"
common_term_upload_response="$(curl -fsS -X POST "${API_BASE_URL}/documents/upload" \
  -H "Authorization: Bearer ${token}" \
  -F "file=@${common_term_file};type=text/plain" \
  -F "title=Go Strings Smoke Document" \
  -F "allowed_role_ids=${owner_role_id}")"
common_term_document_id="$(printf '%s' "$common_term_upload_response" | jq -r '.id')"

if [[ -z "$common_term_document_id" || "$common_term_document_id" == "null" ]]; then
  echo "failed to upload common-term document" >&2
  exit 1
fi

echo "==> Create restricted role"
role_response="$(curl -fsS -X POST "${API_BASE_URL}/admin/roles" \
  -H "Authorization: Bearer ${token}" \
  -H "Content-Type: application/json" \
  -d '{"name":"UnstrictRestricted","permissions":["can_manage_users","can_manage_roles","can_use_unstrict"]}')"
restricted_role_id="$(printf '%s' "$role_response" | jq -r '.id')"

if [[ -z "$restricted_role_id" || "$restricted_role_id" == "null" ]]; then
  echo "failed to create restricted role" >&2
  exit 1
fi

cat >"$restricted_file" <<EOF
Unstrict RBAC restricted smoke document.
Token: ${restricted_token}
Only restricted role should see this in citations.
EOF

echo "==> Upload restricted document"
restricted_upload_response="$(curl -fsS -X POST "${API_BASE_URL}/documents/upload" \
  -H "Authorization: Bearer ${token}" \
  -F "file=@${restricted_file};type=text/plain" \
  -F "title=Unstrict RBAC Document" \
  -F "allowed_role_ids=${restricted_role_id}")"
restricted_document_id="$(printf '%s' "$restricted_upload_response" | jq -r '.id')"

if [[ -z "$restricted_document_id" || "$restricted_document_id" == "null" ]]; then
  echo "failed to upload restricted document" >&2
  exit 1
fi

wait_document_ready() {
  local doc_id="$1"
  for _ in $(seq 1 90); do
    status="$(curl -fsS "${API_BASE_URL}/documents" -H "Authorization: Bearer ${token}" | jq -r --arg doc_id "$doc_id" '.documents[] | select(.id==$doc_id) | .status')"
    if [[ "$status" == "ready" ]]; then
      return 0
    fi
    if [[ "$status" == "failed" ]]; then
      echo "document ${doc_id} ingestion failed" >&2
      exit 1
    fi
    sleep 2
  done
  echo "document ${doc_id} did not become ready in time" >&2
  exit 1
}

echo "==> Wait documents ready"
wait_document_ready "$stable_document_id"
wait_document_ready "$common_term_document_id"
wait_document_ready "$restricted_document_id"

echo "==> Validate retrieval stability"
ids_1="$(curl -fsS -X POST "${API_BASE_URL}/admin/retrieval/debug" \
  -H "Authorization: Bearer ${token}" \
  -H "Content-Type: application/json" \
  -d "{\"query\":\"${stable_token}\",\"top_k\":8,\"candidate_k\":32}" | jq -r '.citations[0:3] | map(.chunk_id) | join(",")')"
ids_2="$(curl -fsS -X POST "${API_BASE_URL}/admin/retrieval/debug" \
  -H "Authorization: Bearer ${token}" \
  -H "Content-Type: application/json" \
  -d "{\"query\":\"${stable_token}\",\"top_k\":8,\"candidate_k\":32}" | jq -r '.citations[0:3] | map(.chunk_id) | join(",")')"
ids_3="$(curl -fsS -X POST "${API_BASE_URL}/admin/retrieval/debug" \
  -H "Authorization: Bearer ${token}" \
  -H "Content-Type: application/json" \
  -d "{\"query\":\"${stable_token}\",\"top_k\":8,\"candidate_k\":32}" | jq -r '.citations[0:3] | map(.chunk_id) | join(",")')"

if [[ -z "$ids_1" || "$ids_1" == "null" ]]; then
  echo "retrieval returned empty chunk ids" >&2
  exit 1
fi
if [[ "$ids_1" != "$ids_2" || "$ids_1" != "$ids_3" ]]; then
  echo "retrieval top chunks are unstable: ${ids_1} | ${ids_2} | ${ids_3}" >&2
  exit 1
fi

echo "==> Validate common-term retrieval"
common_term_debug="$(curl -fsS -X POST "${API_BASE_URL}/admin/retrieval/debug" \
  -H "Authorization: Bearer ${token}" \
  -H "Content-Type: application/json" \
  -d '{"query":"что такое строка","top_k":8,"candidate_k":32}')"
common_term_hits="$(printf '%s' "$common_term_debug" | jq -r --arg common_doc_id "$common_term_document_id" '[.citations[] | select(.document_id == $common_doc_id)] | length')"
if [[ "${common_term_hits:-0}" -le 0 ]]; then
  echo "common-term retrieval did not return uploaded go string document" >&2
  exit 1
fi
common_term_intent="$(printf '%s' "$common_term_debug" | jq -r '.query_intent')"
if [[ "$common_term_intent" != "definition" ]]; then
  echo "expected query_intent=definition, got ${common_term_intent}" >&2
  exit 1
fi
common_term_kind="$(printf '%s' "$common_term_debug" | jq -r --arg common_doc_id "$common_term_document_id" '.citations[] | select(.document_id == $common_doc_id) | .metadata.chunk_kind' | head -n1)"
if [[ "$common_term_kind" != "definition" ]]; then
  echo "expected retrieved common-term chunk_kind=definition, got ${common_term_kind}" >&2
  exit 1
fi

create_chat() {
  curl -fsS -X POST "${API_BASE_URL}/chats" \
    -H "Authorization: Bearer ${token}" \
    -H "Content-Type: application/json" \
    -d '{"title":"unstrict rbac smoke"}' | jq -r '.id'
}

ask_citations_len() {
  local chat_id="$1"
  local query="$2"
  curl -fsS -X POST "${API_BASE_URL}/chats/${chat_id}/messages" \
    -H "Authorization: Bearer ${token}" \
    -H "Content-Type: application/json" \
    -d "{\"content\":\"${query}\",\"mode\":\"unstrict\",\"top_k\":8,\"candidate_k\":32}"
}

echo "==> Validate unstrict RBAC (before role switch)"
chat_before="$(create_chat)"
response_before="$(ask_citations_len "$chat_before" "$restricted_token")"
citations_before="$(printf '%s' "$response_before" | jq -r '.citations | length')"
restricted_before_count="$(printf '%s' "$response_before" | jq -r --arg restricted_doc_id "$restricted_document_id" '[.citations[] | select(.document_id == $restricted_doc_id)] | length')"
if [[ "${restricted_before_count:-0}" -ne 0 ]]; then
  echo "restricted document leaked before role switch" >&2
  exit 1
fi

echo "==> Switch to restricted role"
curl -fsS -X PATCH "${API_BASE_URL}/admin/users/${user_id}/role" \
  -H "Authorization: Bearer ${token}" \
  -H "Content-Type: application/json" \
  -d "{\"role_id\":${restricted_role_id}}" >/dev/null

echo "==> Validate unstrict RBAC (after role switch)"
chat_after="$(create_chat)"
response_after="$(ask_citations_len "$chat_after" "$restricted_token")"
citations_after="$(printf '%s' "$response_after" | jq -r '.citations | length')"
restricted_after_count="$(printf '%s' "$response_after" | jq -r --arg restricted_doc_id "$restricted_document_id" '[.citations[] | select(.document_id == $restricted_doc_id)] | length')"
if [[ "${restricted_after_count:-0}" -le 0 ]]; then
  echo "expected restricted document citations after role switch, got ${restricted_after_count}" >&2
  exit 1
fi

echo "==> Restore owner role"
curl -fsS -X PATCH "${API_BASE_URL}/admin/users/${user_id}/role" \
  -H "Authorization: Bearer ${token}" \
  -H "Content-Type: application/json" \
  -d "{\"role_id\":${owner_role_id}}" >/dev/null

echo "Retrieval stability smoke passed: ids=${ids_1}; common-term hits=${common_term_hits}; intent=${common_term_intent}; kind=${common_term_kind}; unstrict total citations before=${citations_before}, after=${citations_after}, restricted before=${restricted_before_count}, restricted after=${restricted_after_count}"
