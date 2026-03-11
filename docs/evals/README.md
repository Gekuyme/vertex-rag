# Retrieval Evals

This directory contains the offline retrieval evaluation scaffold for `vertex-rag`.

## Files

- `retrieval_eval_seed.json`: first working batch of 52 cases grouped by category and tags.
- `retrieval_eval_corpus.json`: synthetic document corpus keyed by stable `document_alias` values used by the eval cases.
- `retrieval_eval_results.sample.json`: minimal example of the saved results format.

## Low-cost eval workflow

To avoid burning Gemini quota during retrieval evaluation, use the debug endpoint and disable LLM-assisted retrieval features while collecting results.

Recommended runtime settings for low-cost collection:

```bash
EMBED_PROVIDER=ollama
EMBED_MODEL_OLLAMA=nomic-embed-text
QUERY_REWRITE_ENABLED=false
QUERY_EXPANSION_ENABLED=false
HYDE_ENABLED=false
RERANKER_ENABLED=true
RETRIEVAL_PIPELINE_VERSION=v2
```

If you cannot use Ollama, any non-Gemini embedding provider is still preferable for baseline-vs-v2 retrieval collection.

### Seed the synthetic corpus

Before collection, seed the isolated eval corpus:

```bash
EMBED_PROVIDER=ollama \
LLM_PROVIDER=ollama \
QUERY_REWRITE_ENABLED=false \
QUERY_EXPANSION_ENABLED=false \
HYDE_ENABLED=false \
RERANKER_ENABLED=true \
SPARSE_SEARCH_PROVIDER=opensearch \
make seed-eval-corpus
```

Default eval credentials:

- email: `eval@vertex.local`
- password: `Password123!`

The seeder creates or reuses an `Eval Lab` organization, removes earlier eval documents from that org, inserts parent sections plus child chunks, and indexes sparse chunks into OpenSearch.

### Collect results from the API

Single-pipeline collection:

```bash
python3 scripts/retrieval_collect_debug.py \
  --cases docs/evals/retrieval_eval_seed.json \
  --output-dir tmp/evals/v2 \
  --pipeline-version v2 \
  --owner-email eval@vertex.local \
  --owner-password 'Password123!' \
  --delay-seconds 1.0
```

Baseline collection:

```bash
python3 scripts/retrieval_collect_debug.py \
  --cases docs/evals/retrieval_eval_seed.json \
  --output-dir tmp/evals/v1 \
  --pipeline-version v1 \
  --owner-email eval@vertex.local \
  --owner-password 'Password123!' \
  --delay-seconds 1.0
```

Or collect both in one debug response per case:

```bash
python3 scripts/retrieval_collect_debug.py \
  --cases docs/evals/retrieval_eval_seed.json \
  --output-dir tmp/evals/compare \
  --compare-pipelines \
  --delay-seconds 1.0
```

The collector only calls `/admin/retrieval/debug`. It does not create chats or generate model answers.

## Saved result format

The evaluator expects either:

- one JSON object keyed by case id, or
- one JSON file per case in a directory, named `<case-id>.json`

Each result only needs the retrieval debug payload subset used for ranking:

```json
{
  "citations": [
    {
      "chunk_id": "chunk-1",
      "document_id": "doc-1"
    }
  ]
}
```

## Run the evaluator

```bash
python3 scripts/retrieval_offline_eval.py \
  --cases docs/evals/retrieval_eval_seed.json \
  --results docs/evals/retrieval_eval_results.sample.json
```

The script prints `Recall@k`, `MRR@k`, and `nDCG@k` overall and by tag/category.

Example:

```bash
python3 scripts/retrieval_offline_eval.py \
  --cases docs/evals/retrieval_eval_seed.json \
  --results tmp/evals/v2
```

## Notes

- This is now a first working batch, not the final target dataset from the upgrade plan.
- The long-term target is still a larger real-world set with at least 150 questions.
- The evaluator is intentionally offline-only and does not call the API or any model provider.
