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
email="grounding_smoke_${timestamp}@example.com"
org_name="Grounding Smoke Org ${timestamp}"
role_name="GroundingRestricted"
doc_a_file="$(mktemp)"
doc_b_file="$(mktemp)"

cleanup() {
  rm -f "$doc_a_file" "$doc_b_file"
}
trap cleanup EXIT

echo "==> Register owner"
register_response="$(curl -fsS -X POST "${API_BASE_URL}/auth/register_owner" \
  -H "Content-Type: application/json" \
  -d "{\"organization_name\":\"${org_name}\",\"email\":\"${email}\",\"password\":\"${PASSWORD}\"}")"

token="$(printf '%s' "$register_response" | jq -r '.access_token')"
owner_role_id="$(printf '%s' "$register_response" | jq -r '.user.role_id')"
if [[ -z "$token" || "$token" == "null" ]]; then
  echo "failed to register owner and get token" >&2
  exit 1
fi

echo "==> Create secondary role"
role_response="$(curl -fsS -X POST "${API_BASE_URL}/admin/roles" \
  -H "Authorization: Bearer ${token}" \
  -H "Content-Type: application/json" \
  -d "{\"name\":\"${role_name}\",\"permissions\":[\"can_manage_users\",\"can_manage_roles\"]}")"
secondary_role_id="$(printf '%s' "$role_response" | jq -r '.id')"
if [[ -z "$secondary_role_id" || "$secondary_role_id" == "null" ]]; then
  echo "failed to create role" >&2
  exit 1
fi

cat >"$doc_a_file" <<'EOF'
## SSO
SSO is enabled for all enterprise tenants in this environment.
EOF

cat >"$doc_b_file" <<'EOF'
## SSO
SSO is disabled for all enterprise tenants in this environment.
EOF

upload_doc() {
  local file="$1"
  local title="$2"
  curl -fsS -X POST "${API_BASE_URL}/documents/upload" \
    -H "Authorization: Bearer ${token}" \
    -F "file=@${file};type=text/plain" \
    -F "title=${title}" \
    -F "allowed_role_ids=${owner_role_id}" \
    -F "allowed_role_ids=${secondary_role_id}" | jq -r '.id'
}

echo "==> Upload contradiction documents"
doc_a_id="$(upload_doc "$doc_a_file" "Grounding SSO Enabled")"
doc_b_id="$(upload_doc "$doc_b_file" "Grounding SSO Disabled")"

wait_document_ready() {
  local doc_id="$1"
  local status=""
  for _ in $(seq 1 90); do
    status="$(curl -fsS "${API_BASE_URL}/documents" \
      -H "Authorization: Bearer ${token}" | jq -r --arg doc_id "$doc_id" '.documents[] | select(.id==$doc_id) | .status')"
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
wait_document_ready "$doc_a_id"
wait_document_ready "$doc_b_id"

echo "==> Validate grounding metadata from retrieval debug"
debug_response="$(curl -fsS -X POST "${API_BASE_URL}/admin/retrieval/debug" \
  -H "Authorization: Bearer ${token}" \
  -H "Content-Type: application/json" \
  -d '{"query":"sso","top_k":8,"candidate_k":20,"pipeline_version":"v2"}')"

confidence="$(printf '%s' "$debug_response" | jq -r '.grounding.confidence')"
coverage_ratio="$(printf '%s' "$debug_response" | jq -r '.grounding.coverage_ratio')"
source_agreement="$(printf '%s' "$debug_response" | jq -r '.grounding.source_agreement')"
contradiction_count="$(printf '%s' "$debug_response" | jq -r '.grounding.contradictions | length')"
first_contradiction="$(printf '%s' "$debug_response" | jq -r '.grounding.contradictions[0].type // empty')"
evidence_span="$(printf '%s' "$debug_response" | jq -r '.citations[0].evidence_span // empty')"

if [[ -z "$confidence" || "$confidence" == "null" ]]; then
  echo "missing grounding.confidence" >&2
  exit 1
fi
if [[ -z "$coverage_ratio" || "$coverage_ratio" == "null" ]]; then
  echo "missing grounding.coverage_ratio" >&2
  exit 1
fi
if [[ -z "$source_agreement" || "$source_agreement" == "null" ]]; then
  echo "missing grounding.source_agreement" >&2
  exit 1
fi
if [[ "${contradiction_count:-0}" -le 0 ]]; then
  echo "expected contradiction signal in grounding metadata" >&2
  exit 1
fi
if [[ "$first_contradiction" != "boolean_conflict" && "$first_contradiction" != "policy_conflict" ]]; then
  echo "unexpected contradiction type: ${first_contradiction}" >&2
  exit 1
fi
if [[ -z "$evidence_span" ]]; then
  echo "expected evidence_span in citations" >&2
  exit 1
fi

echo "Grounding debug smoke passed: confidence=${confidence}, coverage=${coverage_ratio}, agreement=${source_agreement}, contradiction=${first_contradiction}"
