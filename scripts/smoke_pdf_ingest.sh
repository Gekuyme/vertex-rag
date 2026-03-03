#!/usr/bin/env bash

set -euo pipefail

API_BASE_URL="${API_BASE_URL:-http://localhost:8080}"
PASSWORD="${SMOKE_PASSWORD:-Password123!}"
ENV_FILE="${ENV_FILE:-.env}"

require_bin() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "missing required binary: $1" >&2
    exit 1
  fi
}

require_bin curl
require_bin jq
require_bin python3
require_bin docker

timestamp="$(date +%s)"
email="pdf_smoke_${timestamp}@example.com"
org_name="PDF Smoke Org ${timestamp}"
token_key="pdf_token_${timestamp}_vertex"
pdf_file="$(mktemp -t vertex-pdf-XXXXXX.pdf)"

cleanup() {
  rm -f "$pdf_file"
}
trap cleanup EXIT

compose_cmd=(docker compose --env-file "${ENV_FILE}" -f deploy/compose/docker-compose.yml)

echo "==> Register owner"
register_response="$(curl -fsS -X POST "${API_BASE_URL}/auth/register_owner" \
  -H "Content-Type: application/json" \
  -d "{\"organization_name\":\"${org_name}\",\"email\":\"${email}\",\"password\":\"${PASSWORD}\"}")"

token="$(printf '%s' "$register_response" | jq -r '.access_token')"
org_id="$(printf '%s' "$register_response" | jq -r '.user.org_id')"
owner_role_id="$(printf '%s' "$register_response" | jq -r '.user.role_id')"

if [[ -z "$token" || "$token" == "null" ]]; then
  echo "failed to register owner and get token" >&2
  exit 1
fi

echo "==> Generate sample PDF"
python3 - "$pdf_file" "$token_key" <<'PY'
import pathlib
import sys

output = pathlib.Path(sys.argv[1])
token = sys.argv[2]
text = f"Vertex PDF smoke token: {token}"

objects = []
objects.append(b"<< /Type /Catalog /Pages 2 0 R >>")
objects.append(b"<< /Type /Pages /Kids [3 0 R] /Count 1 >>")
objects.append(
    b"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 5 0 R >> >> /Contents 4 0 R >>"
)
stream = f"BT /F1 18 Tf 50 740 Td ({text}) Tj ET".encode("latin-1")
objects.append(b"<< /Length " + str(len(stream)).encode("ascii") + b" >>\nstream\n" + stream + b"\nendstream")
objects.append(b"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>")

buf = bytearray()
buf.extend(b"%PDF-1.4\n")
offsets = [0]
for index, obj in enumerate(objects, start=1):
    offsets.append(len(buf))
    buf.extend(f"{index} 0 obj\n".encode("ascii"))
    buf.extend(obj)
    buf.extend(b"\nendobj\n")

xref_offset = len(buf)
buf.extend(f"xref\n0 {len(offsets)}\n".encode("ascii"))
buf.extend(b"0000000000 65535 f \n")
for offset in offsets[1:]:
    buf.extend(f"{offset:010d} 00000 n \n".encode("ascii"))
buf.extend(
    f"trailer\n<< /Size {len(offsets)} /Root 1 0 R >>\nstartxref\n{xref_offset}\n%%EOF\n".encode("ascii")
)
output.write_bytes(buf)
PY

echo "==> Upload PDF"
upload_response="$(curl -fsS -X POST "${API_BASE_URL}/documents/upload" \
  -H "Authorization: Bearer ${token}" \
  -F "file=@${pdf_file};type=application/pdf" \
  -F "title=PDF Smoke Document" \
  -F "allowed_role_ids=${owner_role_id}")"
document_id="$(printf '%s' "$upload_response" | jq -r '.id')"

if [[ -z "$document_id" || "$document_id" == "null" ]]; then
  echo "failed to upload PDF document" >&2
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

if [[ "${status:-}" != "ready" ]]; then
  echo "document did not become ready in time" >&2
  exit 1
fi

echo "==> Verify chunks and embeddings in DB"
chunk_count="$("${compose_cmd[@]}" exec -T postgres psql -U "${POSTGRES_USER:-vertex}" -d "${POSTGRES_DB:-vertex_rag}" -t -A -c "SELECT COUNT(*) FROM document_chunks WHERE org_id='${org_id}'::uuid AND document_id='${document_id}'::uuid;" | tr -d '[:space:]')"
embedding_count="$("${compose_cmd[@]}" exec -T postgres psql -U "${POSTGRES_USER:-vertex}" -d "${POSTGRES_DB:-vertex_rag}" -t -A -c "SELECT COUNT(*) FROM document_chunks WHERE org_id='${org_id}'::uuid AND document_id='${document_id}'::uuid AND embedding IS NOT NULL;" | tr -d '[:space:]')"

if [[ "${chunk_count:-0}" -le 0 ]]; then
  echo "expected chunk_count > 0, got ${chunk_count}" >&2
  exit 1
fi
if [[ "${embedding_count:-0}" -le 0 ]]; then
  echo "expected embedding_count > 0, got ${embedding_count}" >&2
  exit 1
fi

echo "==> Verify strict chat answer has citations"
chat_id="$(curl -fsS -X POST "${API_BASE_URL}/chats" \
  -H "Authorization: Bearer ${token}" \
  -H "Content-Type: application/json" \
  -d '{"title":"PDF smoke chat"}' | jq -r '.id')"

answer_response="$(curl -fsS -X POST "${API_BASE_URL}/chats/${chat_id}/messages" \
  -H "Authorization: Bearer ${token}" \
  -H "Content-Type: application/json" \
  -d "{\"content\":\"${token_key}\",\"mode\":\"strict\",\"top_k\":8,\"candidate_k\":32}")"
citations_len="$(printf '%s' "$answer_response" | jq -r '.citations | length')"

if [[ "${citations_len:-0}" -le 0 ]]; then
  echo "expected strict citations > 0, got ${citations_len}" >&2
  exit 1
fi

echo "PDF smoke passed: ready with embeddings (${embedding_count}/${chunk_count}), strict citations=${citations_len}"
