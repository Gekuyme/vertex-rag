# Scripts

Operational smoke checks:

- `smoke_role_acl.sh` - strict ACL regression (restricted document visibility by role).
- `smoke_mode_toggle.sh` - strict/unstrict toggle affects next request only.
- `smoke_pdf_ingest.sh` - PDF ingest readiness + embedding presence + strict citations.
- `smoke_retrieval_stability.sh` - retrieval stability + unstrict RBAC behavior.
- `smoke_cache_speed.sh` - strict cache speed check (first vs second response).
