#!/usr/bin/env bash

set -euo pipefail

API_BASE_URL="${API_BASE_URL:-http://localhost:8080}"
AUTH_TOKEN="${AUTH_TOKEN:-}"
OWNER_EMAIL="${OWNER_EMAIL:-smoke@vertex.local}"
OWNER_PASSWORD="${OWNER_PASSWORD:-Password123!}"
QUERY_SET="${QUERY_SET:-full}"
CASE_SET="${CASE_SET:-full}"
LLM_RPM_LIMIT="${LLM_RPM_LIMIT:-0}"
REQUEST_DELAY_SECONDS="${REQUEST_DELAY_SECONDS:-0}"
LOW_QUOTA_MODE="${LOW_QUOTA_MODE:-false}"

require_bin() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "missing required binary: $1" >&2
    exit 1
  fi
}

require_bin curl
require_bin jq
require_bin python3

results_file="$(mktemp)"
cleanup() {
  rm -f "$results_file"
}
trap cleanup EXIT

if [[ "$LOW_QUOTA_MODE" == "true" ]]; then
  if [[ "$QUERY_SET" == "full" ]]; then
    QUERY_SET="micro"
  fi
  if [[ "$CASE_SET" == "full" ]]; then
    CASE_SET="micro"
  fi
  if [[ "$LLM_RPM_LIMIT" == "0" ]]; then
    LLM_RPM_LIMIT=10
  fi
fi

apply_rate_limit() {
  local delay="$REQUEST_DELAY_SECONDS"
  if [[ "$LLM_RPM_LIMIT" =~ ^[0-9]+$ ]] && [[ "$LLM_RPM_LIMIT" -gt 0 ]]; then
    delay="$(python3 - "$LLM_RPM_LIMIT" <<'PY'
import sys
rpm = int(sys.argv[1])
print(f"{60.0 / rpm:.2f}")
PY
)"
  fi
  if [[ "$delay" != "0" && "$delay" != "0.0" && "$delay" != "0.00" ]]; then
    sleep "$delay"
  fi
}

token="$AUTH_TOKEN"
if [[ -z "$token" ]]; then
  echo "==> Login as ${OWNER_EMAIL}"
  login_response="$(curl -sS -X POST "${API_BASE_URL}/auth/login" \
    -H "Content-Type: application/json" \
    -d "{\"email\":\"${OWNER_EMAIL}\",\"password\":\"${OWNER_PASSWORD}\"}")"

  token="$(printf '%s' "$login_response" | jq -r '.access_token // empty')"
  if [[ -z "$token" ]]; then
    error_message="$(printf '%s' "$login_response" | jq -r '.error // empty')"
    if [[ -z "$error_message" ]]; then
      error_message="failed to login and get token"
    fi
    echo "${error_message}" >&2
    echo "Tip: run with AUTH_TOKEN=... or OWNER_EMAIL/OWNER_PASSWORD for the target org." >&2
    exit 1
  fi
else
  echo "==> Using AUTH_TOKEN from environment"
fi

run_case() {
  local mode="$1"
  local profile="$2"
  local top_k="$3"
  local candidate_k="$4"
  local query="$5"

  local chat_id
  chat_id="$(curl -fsS -X POST "${API_BASE_URL}/chats" \
    -H "Authorization: Bearer ${token}" \
    -H "Content-Type: application/json" \
    -d "{\"title\":\"matrix ${mode}/${profile}: ${query}\"}" | jq -r '.id')"

  if [[ -z "$chat_id" || "$chat_id" == "null" ]]; then
    echo "failed to create chat for ${mode}/${profile}/${query}" >&2
    exit 1
  fi

  python3 - "$API_BASE_URL" "$token" "$chat_id" "$mode" "$profile" "$top_k" "$candidate_k" "$query" <<'PY'
import json
import sys
import time
import urllib.request

api_base_url, token, chat_id, mode, profile, top_k, candidate_k, query = sys.argv[1:9]
payload = {
    "content": query,
    "mode": mode,
    "top_k": int(top_k),
    "candidate_k": int(candidate_k),
}
url = f"{api_base_url}/chats/{chat_id}/messages"

req = urllib.request.Request(url=url, method="POST", data=json.dumps(payload).encode("utf-8"))
req.add_header("Authorization", f"Bearer {token}")
req.add_header("Content-Type", "application/json")

started = time.perf_counter()
with urllib.request.urlopen(req, timeout=180) as response:
    parsed = json.loads(response.read().decode("utf-8"))
elapsed_ms = int((time.perf_counter() - started) * 1000)

assistant = parsed.get("assistant_message") or {}
content = " ".join((assistant.get("content") or "").split())
status = "ok"
quality = "unknown"
if content == "Недостаточно данных в базе знаний.":
    status = "fallback"
elif not content:
    status = "empty"
expected_keywords = {
    "что такое строки": ["строк", "байт", "utf"],
    "что такое горутины": ["гору", "поток", "go"],
    "что такое каналы": ["канал", "синхрон", "данн"],
    "что такое срезы": ["срез", "массив", "len"],
    "что такое rune": ["rune", "unicode", "символ"],
    "чем срез отличается от массива": ["срез", "массив"],
    "как запустить горутину": ["go", "гору"],
    "как создать канал": ["chan", "канал", "make"],
    "как получить подстроку в go": ["подстрок", "срез", "строк"],
    "можно ли изменять строку в go": ["неизмен", "строк"],
    "почему len строки может отличаться от числа символов": ["len", "байт", "символ"],
    "для чего нужен range в go": ["range", "цикл", "итер"],
}
normalized_content = content.lower()
keywords = expected_keywords.get(query, [])
if status == "ok":
    if keywords:
        matched = sum(1 for keyword in keywords if keyword in normalized_content)
        quality = "good" if matched > 0 else "bad"
        if matched == 0:
            status = "bad_quality"
    else:
        quality = "unchecked"
else:
    quality = status

preview = content[:140]
citations = parsed.get("citations") or []
doc_titles = []
for citation in citations[:3]:
    title = (citation.get("doc_title") or "").strip()
    if title and title not in doc_titles:
        doc_titles.append(title)

print(json.dumps({
    "mode": mode,
    "profile": profile,
    "top_k": int(top_k),
    "candidate_k": int(candidate_k),
    "query": query,
    "elapsed_ms": elapsed_ms,
    "status": status,
    "quality": quality,
    "citations": len(citations),
    "docs": doc_titles,
    "preview": preview,
}, ensure_ascii=False))
PY
}

print_header() {
  printf "%-12s %-9s %-10s %-5s %-10s %-6s %-9s %-32s %s\n" "query_type" "mode" "profile" "k" "candidate" "ms" "status" "docs" "preview"
  printf "%-12s %-9s %-10s %-5s %-10s %-6s %-9s %-32s %s\n" "------------" "---------" "----------" "-----" "----------" "------" "---------" "--------------------------------" "-------"
}

print_row() {
  local json="$1"
  local query query_type mode profile top_k candidate_k elapsed_ms status docs preview
  query="$(printf '%s' "$json" | jq -r '.query')"
  query_type="$(classify_query_type "$query")"
  mode="$(printf '%s' "$json" | jq -r '.mode')"
  profile="$(printf '%s' "$json" | jq -r '.profile')"
  top_k="$(printf '%s' "$json" | jq -r '.top_k')"
  candidate_k="$(printf '%s' "$json" | jq -r '.candidate_k')"
  elapsed_ms="$(printf '%s' "$json" | jq -r '.elapsed_ms')"
  status="$(printf '%s' "$json" | jq -r '.status')"
  docs="$(printf '%s' "$json" | jq -r '.docs | join(", ")')"
  preview="$(printf '%s' "$json" | jq -r '.preview')"

  printf "%-12s %-9s %-10s %-5s %-10s %-6s %-9s %-32.32s %s\n" \
    "$query_type" "$mode" "$profile" "$top_k" "$candidate_k" "$elapsed_ms" "$status" "$docs" "$preview"
}

classify_query_type() {
  local query="$1"
  case "$query" in
    "что такое "*)
      echo "definition"
      ;;
    "чем "*|"в чем разница "*)
      echo "comparison"
      ;;
    "как "*)
      echo "procedure"
      ;;
    "можно ли "*|"почему "*)
      echo "constraint"
      ;;
    *)
      echo "general"
      ;;
  esac
}

if [[ "$QUERY_SET" == "micro" ]]; then
  declare -a queries=(
    "что такое строки"
    "как создать канал"
    "чем срез отличается от массива"
  )
elif [[ "$QUERY_SET" == "lite" ]]; then
  declare -a queries=(
    "что такое строки"
    "что такое горутины"
    "что такое каналы"
    "как создать канал"
    "чем срез отличается от массива"
    "можно ли изменять строку в go"
  )
else
  declare -a queries=(
    "что такое строки"
    "что такое горутины"
    "что такое каналы"
    "что такое срезы"
    "что такое rune"
    "чем срез отличается от массива"
    "как запустить горутину"
    "как создать канал"
    "как получить подстроку в go"
    "можно ли изменять строку в go"
    "почему len строки может отличаться от числа символов"
    "для чего нужен range в go"
  )
fi

if [[ "$CASE_SET" == "micro" ]]; then
  declare -a cases=(
    "strict balanced 6 20"
    "unstrict balanced 6 20"
  )
elif [[ "$CASE_SET" == "lite" ]]; then
  declare -a cases=(
    "strict balanced 8 32"
    "unstrict balanced 8 32"
    "unstrict thinking 12 48"
  )
else
  declare -a cases=(
    "strict fast 4 12"
    "strict balanced 8 32"
    "strict thinking 12 48"
    "unstrict fast 4 12"
    "unstrict balanced 8 32"
    "unstrict thinking 12 48"
  )
fi

echo "==> Query matrix"
print_header
for query in "${queries[@]}"; do
  for case_def in "${cases[@]}"; do
    apply_rate_limit
    # shellcheck disable=SC2086
    json="$(run_case ${case_def} "$query")"
    printf '%s\n' "$json" >>"$results_file"
    print_row "$json"
  done
done

echo
echo "==> Summary"
python3 - "$results_file" <<'PY'
import json
import sys
from collections import defaultdict

path = sys.argv[1]
rows = []
with open(path, "r", encoding="utf-8") as fh:
    for line in fh:
        line = line.strip()
        if not line:
            continue
        row = json.loads(line)
        query = row.get("query", "")
        if query.startswith("что такое "):
            query_type = "definition"
        elif query.startswith("чем ") or query.startswith("в чем разница "):
            query_type = "comparison"
        elif query.startswith("как "):
            query_type = "procedure"
        elif query.startswith("можно ли ") or query.startswith("почему "):
            query_type = "constraint"
        else:
            query_type = "general"
        row["query_type"] = query_type
        rows.append(row)

if not rows:
    print("no results")
    raise SystemExit(0)

stats = defaultdict(lambda: {"count": 0, "ok": 0, "fallback": 0, "empty": 0, "bad_quality": 0, "elapsed_sum": 0})
mode_stats = defaultdict(lambda: {"count": 0, "ok": 0, "fallback": 0, "empty": 0, "bad_quality": 0, "elapsed_sum": 0})
profile_stats = defaultdict(lambda: {"count": 0, "ok": 0, "fallback": 0, "empty": 0, "bad_quality": 0, "elapsed_sum": 0})
for row in rows:
    bucket = stats[row["query_type"]]
    bucket["count"] += 1
    bucket["elapsed_sum"] += row["elapsed_ms"]
    bucket[row["status"]] += 1

    mode_bucket = mode_stats[row["mode"]]
    mode_bucket["count"] += 1
    mode_bucket["elapsed_sum"] += row["elapsed_ms"]
    mode_bucket[row["status"]] += 1

    profile_bucket = profile_stats[row["profile"]]
    profile_bucket["count"] += 1
    profile_bucket["elapsed_sum"] += row["elapsed_ms"]
    profile_bucket[row["status"]] += 1

print(f"{'query_type':12} {'count':>5} {'ok':>5} {'fallback':>9} {'bad_q':>7} {'empty':>6} {'avg_ms':>8}")
print(f"{'-'*12} {'-'*5} {'-'*5} {'-'*9} {'-'*7} {'-'*6} {'-'*8}")
for query_type in sorted(stats.keys()):
    bucket = stats[query_type]
    avg_ms = int(bucket["elapsed_sum"] / max(bucket["count"], 1))
    print(f"{query_type:12} {bucket['count']:5d} {bucket['ok']:5d} {bucket['fallback']:9d} {bucket['bad_quality']:7d} {bucket['empty']:6d} {avg_ms:8d}")

print()
print(f"{'mode':12} {'count':>5} {'ok':>5} {'fallback':>9} {'bad_q':>7} {'empty':>6} {'avg_ms':>8}")
print(f"{'-'*12} {'-'*5} {'-'*5} {'-'*9} {'-'*7} {'-'*6} {'-'*8}")
for mode in sorted(mode_stats.keys()):
    bucket = mode_stats[mode]
    avg_ms = int(bucket["elapsed_sum"] / max(bucket["count"], 1))
    print(f"{mode:12} {bucket['count']:5d} {bucket['ok']:5d} {bucket['fallback']:9d} {bucket['bad_quality']:7d} {bucket['empty']:6d} {avg_ms:8d}")

print()
print(f"{'profile':12} {'count':>5} {'ok':>5} {'fallback':>9} {'bad_q':>7} {'empty':>6} {'avg_ms':>8}")
print(f"{'-'*12} {'-'*5} {'-'*5} {'-'*9} {'-'*7} {'-'*6} {'-'*8}")
for profile in sorted(profile_stats.keys()):
    bucket = profile_stats[profile]
    avg_ms = int(bucket["elapsed_sum"] / max(bucket["count"], 1))
    print(f"{profile:12} {bucket['count']:5d} {bucket['ok']:5d} {bucket['fallback']:9d} {bucket['bad_quality']:7d} {bucket['empty']:6d} {avg_ms:8d}")

print()
print("Worst by latency:")
for row in sorted(rows, key=lambda item: item["elapsed_ms"], reverse=True)[:5]:
    print(f"- {row['elapsed_ms']}ms | {row['mode']}/{row['profile']} | {row['query']} | {row['status']}")

print()
print("Worst by quality:")
bad_rows = [row for row in rows if row["status"] != "ok"]
if not bad_rows:
    print("- no fallback/empty cases")
else:
    for row in bad_rows[:10]:
        print(f"- {row['mode']}/{row['profile']} | {row['query']} | {row['status']} | docs={', '.join(row.get('docs') or [])}")
PY

echo
echo "Tip: run 'make logs-api | rg chat_flow' in another terminal to correlate server-side stage timings."
