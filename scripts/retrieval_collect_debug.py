#!/usr/bin/env python3

import argparse
import json
import os
import sys
import time
import urllib.error
import urllib.parse
import urllib.request
from pathlib import Path
from typing import Any


def load_json(path: str) -> Any:
    with open(path, "r", encoding="utf-8") as handle:
        return json.load(handle)


def request_json(url: str, method: str, payload: dict[str, Any] | None = None, token: str | None = None) -> dict[str, Any]:
    data = None
    if payload is not None:
        data = json.dumps(payload).encode("utf-8")
    request = urllib.request.Request(url=url, method=method, data=data)
    request.add_header("Content-Type", "application/json")
    if token:
        request.add_header("Authorization", f"Bearer {token}")

    try:
        with urllib.request.urlopen(request, timeout=180) as response:
            raw = response.read().decode("utf-8")
    except urllib.error.HTTPError as exc:
        raw = exc.read().decode("utf-8", errors="replace")
        raise SystemExit(f"{method} {url} failed: {exc.code} {raw}") from exc

    parsed = json.loads(raw)
    if not isinstance(parsed, dict):
        raise SystemExit(f"expected JSON object from {method} {url}")
    return parsed


def login(api_base_url: str, email: str, password: str) -> str:
    response = request_json(
        f"{api_base_url}/auth/login",
        "POST",
        {"email": email, "password": password},
    )
    token = str(response.get("access_token", "")).strip()
    if not token:
        raise SystemExit("failed to obtain access token from /auth/login")
    return token


def ensure_dir(path: str) -> Path:
    directory = Path(path)
    directory.mkdir(parents=True, exist_ok=True)
    return directory


def save_json(path: Path, payload: dict[str, Any]) -> None:
    with path.open("w", encoding="utf-8") as handle:
        json.dump(payload, handle, ensure_ascii=False, indent=2)
        handle.write("\n")


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description="Collect throttled /admin/retrieval/debug results for offline eval."
    )
    parser.add_argument("--cases", required=True, help="Path to eval cases JSON file")
    parser.add_argument("--output-dir", required=True, help="Directory to write collected results")
    parser.add_argument("--api-base-url", default=os.environ.get("API_BASE_URL", "http://localhost:8080"))
    parser.add_argument("--auth-token", default=os.environ.get("AUTH_TOKEN", ""))
    parser.add_argument("--owner-email", default=os.environ.get("OWNER_EMAIL", "owner@vertex.local"))
    parser.add_argument("--owner-password", default=os.environ.get("OWNER_PASSWORD", "Password123!"))
    parser.add_argument("--pipeline-version", choices=("v1", "v2"), default="v2")
    parser.add_argument("--compare-pipelines", action="store_true", help="Save compare_pipelines debug output instead of a single pipeline")
    parser.add_argument("--top-k", type=int, default=10)
    parser.add_argument("--candidate-k", type=int, default=32)
    parser.add_argument("--delay-seconds", type=float, default=0.0, help="Sleep between requests")
    parser.add_argument("--limit", type=int, default=0, help="Optional max number of cases to collect")
    return parser.parse_args()


def main() -> int:
    args = parse_args()
    cases = load_json(args.cases)
    if not isinstance(cases, list):
        raise SystemExit(f"cases file must contain a JSON array: {args.cases}")

    token = args.auth_token.strip() or login(args.api_base_url.rstrip("/"), args.owner_email, args.owner_password)
    output_dir = ensure_dir(args.output_dir)

    collected = 0
    for case in cases:
        case_id = str(case.get("id", "")).strip()
        query = str(case.get("query", "")).strip()
        if not case_id or not query:
            raise SystemExit("each case must include non-empty id and query")
        if args.limit > 0 and collected >= args.limit:
            break

        payload: dict[str, Any] = {
            "query": query,
            "top_k": args.top_k,
            "candidate_k": args.candidate_k,
        }
        if args.compare_pipelines:
            payload["compare_pipelines"] = True
        else:
            payload["pipeline_version"] = args.pipeline_version

        response = request_json(
            f"{args.api_base_url.rstrip('/')}/admin/retrieval/debug",
            "POST",
            payload,
            token=token,
        )
        save_json(output_dir / f"{case_id}.json", response)
        collected += 1

        if args.delay_seconds > 0:
            time.sleep(args.delay_seconds)

    print(f"collected {collected} case(s) into {output_dir}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
