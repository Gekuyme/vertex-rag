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
require_bin python3

timestamp="$(date +%s)"
email="cache_smoke_${timestamp}@example.com"
org_name="Cache Smoke Org ${timestamp}"
token_key="cache_token_${timestamp}_vertex"
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
owner_role_id="$(printf '%s' "$register_response" | jq -r '.user.role_id')"
if [[ -z "$token" || "$token" == "null" ]]; then
  echo "failed to register owner and get token" >&2
  exit 1
fi

cat >"$document_file" <<EOF
Cache speed smoke document.
Token: ${token_key}
This token should produce cached strict retrieval and answer.
EOF

echo "==> Upload document"
upload_response="$(curl -fsS -X POST "${API_BASE_URL}/documents/upload" \
  -H "Authorization: Bearer ${token}" \
  -F "file=@${document_file};type=text/plain" \
  -F "title=Cache Speed Document" \
  -F "allowed_role_ids=${owner_role_id}")"
document_id="$(printf '%s' "$upload_response" | jq -r '.id')"

if [[ -z "$document_id" || "$document_id" == "null" ]]; then
  echo "failed to upload document" >&2
  exit 1
fi

echo "==> Wait for ingestion ready"
for _ in $(seq 1 90); do
  status="$(curl -fsS "${API_BASE_URL}/documents" -H "Authorization: Bearer ${token}" | jq -r --arg doc_id "$document_id" '.documents[] | select(.id==$doc_id) | .status')"
  if [[ "$status" == "ready" ]]; then
    break
  fi
  if [[ "$status" == "failed" ]]; then
    echo "document ingestion failed" >&2
    exit 1
  fi
  sleep 2
done

if [[ "${status:-}" != "ready" ]]; then
  echo "document did not become ready in time" >&2
  exit 1
fi

chat_id="$(curl -fsS -X POST "${API_BASE_URL}/chats" \
  -H "Authorization: Bearer ${token}" \
  -H "Content-Type: application/json" \
  -d '{"title":"Cache speed smoke"}' | jq -r '.id')"

echo "==> Measure strict first/second response latency"
measure_json="$(python3 - "$API_BASE_URL" "$token" "$chat_id" "$token_key" <<'PY'
import json
import sys
import time
import urllib.request

api_base_url, token, chat_id, token_key = sys.argv[1:5]
url = f"{api_base_url}/chats/{chat_id}/messages"
payload = {
    "content": token_key,
    "mode": "strict",
    "top_k": 8,
    "candidate_k": 32,
}

def call_once():
    data = json.dumps(payload).encode("utf-8")
    req = urllib.request.Request(url=url, method="POST", data=data)
    req.add_header("Authorization", f"Bearer {token}")
    req.add_header("Content-Type", "application/json")
    started = time.perf_counter()
    with urllib.request.urlopen(req, timeout=120) as response:
        body = response.read()
    elapsed_ms = (time.perf_counter() - started) * 1000.0
    parsed = json.loads(body.decode("utf-8"))
    citations = parsed.get("citations") or []
    return elapsed_ms, len(citations)

first_ms, first_citations = call_once()
second_ms, second_citations = call_once()

print(json.dumps({
    "first_ms": first_ms,
    "second_ms": second_ms,
    "first_citations": first_citations,
    "second_citations": second_citations,
}))
PY
)"

first_ms="$(printf '%s' "$measure_json" | jq -r '.first_ms')"
second_ms="$(printf '%s' "$measure_json" | jq -r '.second_ms')"
first_citations="$(printf '%s' "$measure_json" | jq -r '.first_citations')"
second_citations="$(printf '%s' "$measure_json" | jq -r '.second_citations')"

if [[ "${first_citations:-0}" -le 0 || "${second_citations:-0}" -le 0 ]]; then
  echo "expected citations on both strict calls, got first=${first_citations}, second=${second_citations}" >&2
  exit 1
fi

python3 - "$first_ms" "$second_ms" <<'PY'
import sys
first_ms = float(sys.argv[1])
second_ms = float(sys.argv[2])

# Allow up to 10% noise, but expect second response to be faster (cache hit).
if second_ms > first_ms * 1.10:
    raise SystemExit(f"second response is not faster enough: first={first_ms:.1f}ms second={second_ms:.1f}ms")
PY

echo "Cache speed smoke passed: first=${first_ms}ms, second=${second_ms}ms"
