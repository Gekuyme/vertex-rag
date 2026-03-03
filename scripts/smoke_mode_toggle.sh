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
email="mode_toggle_${timestamp}@example.com"
org_name="Mode Toggle Org ${timestamp}"
stream_file="$(mktemp)"

cleanup() {
  rm -f "$stream_file"
}
trap cleanup EXIT

echo "==> Register owner"
register_response="$(curl -fsS -X POST "${API_BASE_URL}/auth/register_owner" \
  -H "Content-Type: application/json" \
  -d "{\"organization_name\":\"${org_name}\",\"email\":\"${email}\",\"password\":\"${PASSWORD}\"}")"

token="$(printf '%s' "$register_response" | jq -r '.access_token')"
if [[ -z "$token" || "$token" == "null" ]]; then
  echo "failed to register owner and get token" >&2
  exit 1
fi

echo "==> Set default mode to unstrict"
curl -fsS -X PATCH "${API_BASE_URL}/me/settings" \
  -H "Authorization: Bearer ${token}" \
  -H "Content-Type: application/json" \
  -d '{"default_mode":"unstrict"}' >/dev/null

chat_id="$(curl -fsS -X POST "${API_BASE_URL}/chats" \
  -H "Authorization: Bearer ${token}" \
  -H "Content-Type: application/json" \
  -d '{"title":"Mode toggle smoke"}' | jq -r '.id')"

long_question="Сделай подробный план из 20 пунктов по автоматизации документооборота в компании и добавь практические шаги внедрения."

echo "==> Start streaming request without explicit mode"
curl -sS -N -X POST "${API_BASE_URL}/chats/${chat_id}/messages/stream" \
  -H "Authorization: Bearer ${token}" \
  -H "Content-Type: application/json" \
  -d "{\"content\":\"${long_question}\"}" >"${stream_file}" &
stream_pid=$!

sleep 0.2

echo "==> Toggle default mode to strict while stream is in progress"
curl -fsS -X PATCH "${API_BASE_URL}/me/settings" \
  -H "Authorization: Bearer ${token}" \
  -H "Content-Type: application/json" \
  -d '{"default_mode":"strict"}' >/dev/null

wait "${stream_pid}"

first_mode="$(awk '
  /^event: / { event=$2 }
  /^data: / {
    if (event == "done") {
      gsub(/^data: /, "", $0)
      print $0
      exit
    }
  }
' "${stream_file}" | jq -r '.mode')"

if [[ "$first_mode" != "unstrict" ]]; then
  echo "expected in-flight request mode=unstrict, got ${first_mode}" >&2
  exit 1
fi

echo "==> Next request without explicit mode must use strict"
second_response="$(curl -fsS -X POST "${API_BASE_URL}/chats/${chat_id}/messages" \
  -H "Authorization: Bearer ${token}" \
  -H "Content-Type: application/json" \
  -d '{"content":"Проверь текущий режим ответа"}')"
second_mode="$(printf '%s' "$second_response" | jq -r '.mode')"

if [[ "$second_mode" != "strict" ]]; then
  echo "expected next request mode=strict after toggle, got ${second_mode}" >&2
  exit 1
fi

echo "Mode toggle smoke passed: first=${first_mode}, second=${second_mode}"
