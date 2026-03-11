#!/usr/bin/env python3

import argparse
import json
import math
import os
import sys
from collections import defaultdict
from typing import Any


def load_json(path: str) -> Any:
    with open(path, "r", encoding="utf-8") as handle:
        return json.load(handle)


def load_results(path: str) -> dict[str, Any]:
    if os.path.isdir(path):
        loaded: dict[str, Any] = {}
        for name in sorted(os.listdir(path)):
            if not name.endswith(".json"):
                continue
            case_id = name[:-5]
            loaded[case_id] = load_json(os.path.join(path, name))
        return loaded
    payload = load_json(path)
    if isinstance(payload, dict):
        return payload
    raise SystemExit(f"results payload must be a JSON object or directory: {path}")


def normalize_ids(values: list[str] | None) -> set[str]:
    normalized: set[str] = set()
    for value in values or []:
        token = str(value).strip()
        if token:
            normalized.add(token)
    return normalized


def ranked_ids_from_response(response: dict[str, Any], target: str) -> list[str]:
    citations = response.get("citations") or []
    ranked: list[str] = []
    seen: set[str] = set()
    for citation in citations:
        if not isinstance(citation, dict):
            continue
        if target == "chunk":
            value = str(citation.get("chunk_id", "")).strip()
            if value and value not in seen:
                seen.add(value)
                ranked.append(value)
            continue

        metadata = citation.get("metadata")
        if isinstance(metadata, dict):
            alias = str(metadata.get("document_alias", "")).strip()
            if alias and alias not in seen:
                seen.add(alias)
                ranked.append(alias)
                continue

        value = str(citation.get("document_id", "")).strip()
        if value and value not in seen:
            seen.add(value)
            ranked.append(value)
    return ranked


def recall_at_k(ranked: list[str], relevant: set[str], k: int) -> float:
    if not relevant:
        return 0.0
    hits = sum(1 for item in ranked[:k] if item in relevant)
    return hits / float(len(relevant))


def mrr_at_k(ranked: list[str], relevant: set[str], k: int) -> float:
    for index, item in enumerate(ranked[:k], start=1):
        if item in relevant:
            return 1.0 / float(index)
    return 0.0


def ndcg_at_k(ranked: list[str], relevant: set[str], k: int) -> float:
    dcg = 0.0
    for index, item in enumerate(ranked[:k], start=1):
        gain = 1.0 if item in relevant else 0.0
        if gain:
            dcg += gain / math.log2(index + 1)
    ideal_hits = min(len(relevant), k)
    if ideal_hits == 0:
        return 0.0
    idcg = sum(1.0 / math.log2(index + 1) for index in range(1, ideal_hits + 1))
    return dcg / idcg if idcg else 0.0


def summarize_bucket(rows: list[dict[str, Any]], k: int) -> dict[str, float]:
    if not rows:
        return {"count": 0, "recall_at_k": 0.0, "mrr_at_k": 0.0, "ndcg_at_k": 0.0}
    return {
        "count": len(rows),
        "recall_at_k": sum(row["recall_at_k"] for row in rows) / len(rows),
        "mrr_at_k": sum(row["mrr_at_k"] for row in rows) / len(rows),
        "ndcg_at_k": sum(row["ndcg_at_k"] for row in rows) / len(rows),
    }


def build_report(cases: list[dict[str, Any]], results: dict[str, Any], k: int) -> dict[str, Any]:
    rows: list[dict[str, Any]] = []
    by_tag: dict[str, list[dict[str, Any]]] = defaultdict(list)
    missing_cases: list[str] = []

    for case in cases:
        case_id = str(case.get("id", "")).strip()
        if not case_id:
            raise SystemExit("every case must include a non-empty id")

        response = results.get(case_id)
        if response is None:
            missing_cases.append(case_id)
            continue

        expected_chunks = normalize_ids(case.get("expected_chunk_ids"))
        expected_docs = normalize_ids(case.get("expected_document_ids"))
        target = "chunk" if expected_chunks else "document"
        relevant = expected_chunks if expected_chunks else expected_docs
        ranked = ranked_ids_from_response(response, target)
        row = {
            "id": case_id,
            "query": case.get("query", ""),
            "target": target,
            "category": case.get("category", ""),
            "tags": case.get("tags", []),
            "relevant_ids": sorted(relevant),
            "ranked_ids": ranked[:k],
            "recall_at_k": recall_at_k(ranked, relevant, k),
            "mrr_at_k": mrr_at_k(ranked, relevant, k),
            "ndcg_at_k": ndcg_at_k(ranked, relevant, k),
        }
        rows.append(row)

        for tag in case.get("tags", []):
            tag_name = str(tag).strip()
            if tag_name:
                by_tag[tag_name].append(row)
        category = str(case.get("category", "")).strip()
        if category:
            by_tag[f"category:{category}"].append(row)

    return {
        "k": k,
        "summary": summarize_bucket(rows, k),
        "by_tag": {tag: summarize_bucket(items, k) for tag, items in sorted(by_tag.items())},
        "missing_cases": missing_cases,
        "rows": rows,
    }


def print_markdown(report: dict[str, Any]) -> None:
    summary = report["summary"]
    print(f"# Retrieval Offline Eval (k={report['k']})")
    print()
    print("| Bucket | Count | Recall@k | MRR@k | nDCG@k |")
    print("| --- | ---: | ---: | ---: | ---: |")
    print(
        "| overall | {count} | {recall:.3f} | {mrr:.3f} | {ndcg:.3f} |".format(
            count=summary["count"],
            recall=summary["recall_at_k"],
            mrr=summary["mrr_at_k"],
            ndcg=summary["ndcg_at_k"],
        )
    )
    for tag, bucket in report["by_tag"].items():
        print(
            "| {tag} | {count} | {recall:.3f} | {mrr:.3f} | {ndcg:.3f} |".format(
                tag=tag,
                count=bucket["count"],
                recall=bucket["recall_at_k"],
                mrr=bucket["mrr_at_k"],
                ndcg=bucket["ndcg_at_k"],
            )
        )

    if report["missing_cases"]:
        print()
        print("Missing cases:")
        for case_id in report["missing_cases"]:
            print(f"- {case_id}")


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description="Evaluate saved retrieval debug responses without live model calls."
    )
    parser.add_argument("--cases", required=True, help="Path to eval cases JSON file")
    parser.add_argument(
        "--results",
        required=True,
        help="Path to JSON object keyed by case id, or a directory with one <case-id>.json per case",
    )
    parser.add_argument("-k", type=int, default=10, help="Ranking cutoff for metrics")
    parser.add_argument(
        "--format",
        choices=("markdown", "json"),
        default="markdown",
        help="Output format",
    )
    return parser.parse_args()


def main() -> int:
    args = parse_args()
    cases = load_json(args.cases)
    if not isinstance(cases, list):
        raise SystemExit(f"cases file must contain a JSON array: {args.cases}")
    results = load_results(args.results)
    report = build_report(cases, results, args.k)

    if args.format == "json":
        json.dump(report, sys.stdout, ensure_ascii=False, indent=2)
        sys.stdout.write("\n")
    else:
        print_markdown(report)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
