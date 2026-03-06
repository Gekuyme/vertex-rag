# Scripts

Operational smoke checks:

- `ensure_smoke_user.sh` - create/update a persistent smoke user in `Vertex Demo` for repeatable checks.
- `smoke_role_acl.sh` - strict ACL regression (restricted document visibility by role).
- `smoke_mode_toggle.sh` - strict/unstrict toggle affects next request only.
- `smoke_pdf_ingest.sh` - PDF ingest readiness + embedding presence + strict citations.
- `smoke_retrieval_stability.sh` - retrieval stability + unstrict RBAC behavior.
- `smoke_cache_speed.sh` - strict cache speed check (first vs second response).
- `smoke_query_matrix.sh` - mini-benchmark across a broader Go question set (`definition/comparison/procedure/constraint`) and strict/unstrict + fast/balanced/thinking.
- `smoke-query-matrix-lite` (make target) - reduced matrix for quota-limited providers like Gemini free tier.
