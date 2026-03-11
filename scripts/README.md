# Scripts

Operational smoke checks:

- `ensure_smoke_user.sh` - create/update a persistent smoke user in `Vertex Demo` for repeatable checks.
- `smoke_role_acl.sh` - strict ACL regression (restricted document visibility by role).
- `smoke_mode_toggle.sh` - strict/unstrict toggle affects next request only.
- `smoke_pdf_ingest.sh` - PDF ingest readiness + embedding presence + strict citations.
- `smoke_retrieval_stability.sh` - retrieval stability + unstrict RBAC behavior.
- `smoke_grounding_debug.sh` - retrieval debug grounding metadata, confidence fields, contradictions, and evidence spans without relying on answer generation.
- `smoke_cache_speed.sh` - strict cache speed check (first vs second response).
- `smoke_query_matrix.sh` - mini-benchmark across a broader Go question set (`definition/comparison/procedure/constraint`) and strict/unstrict + fast/balanced/thinking.
- `smoke-query-matrix-lite` (make target) - reduced matrix for quota-limited providers like Gemini free tier.
- `LOW_QUOTA_MODE=true` reduces case count and adds pacing for providers limited to about 10-20 requests per minute.
- `LLM_RPM_LIMIT=10` derives a delay between LLM-backed requests.
- `REQUEST_DELAY_SECONDS=6` can be used instead of `LLM_RPM_LIMIT` for manual pacing.

Offline evaluation:

- `retrieval_collect_debug.py` - collect throttled `/admin/retrieval/debug` responses into a directory for offline eval.
- `retrieval_offline_eval.py` - compute `Recall@k`, `MRR@k`, and `nDCG@k` from saved retrieval debug JSON without calling the API or any model provider.
- `make seed-eval-corpus` / `apps/api/cmd/seed_eval_corpus` - seed the synthetic retrieval eval corpus into an isolated org with parent-child chunks and OpenSearch documents.
