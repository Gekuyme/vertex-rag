# Self-host deployment (private images)

## 1) Requirements

- Docker Engine 24+ and Compose plugin.
- Access to private registry with images:
  - `VERTEX_WEB_IMAGE`
  - `VERTEX_API_IMAGE`
  - `VERTEX_WORKER_IMAGE`
- A prepared `.env` file (start from `.env.example`).

## 2) Minimal secure env

Set these before production start:

- `APP_ENV=production`
- `JWT_SECRET=<strong-random-secret>`
- `CORS_ORIGIN=https://your-ui.example.com`
- `COOKIE_SECURE=true`
- `COOKIE_SAMESITE=lax` (or `strict` if UI is same-site)
- `RATE_LIMIT_RPM=240`
- `RATE_LIMIT_BURST=60`

Optional LLM reliability tuning:

- `LLM_HTTP_TIMEOUT=60s`
- `LLM_MAX_RETRIES=2`
- `LLM_RETRY_BACKOFF=400ms`
- `LLM_MAX_CONTEXT_CHARS=7000`

## 3) Start with prebuilt images

```bash
docker login ghcr.io
docker compose --env-file .env -f deploy/compose/docker-compose.yml pull
docker compose --env-file .env -f deploy/compose/docker-compose.yml up -d
```

Or via Makefile:

```bash
make pull-selfhost
make up-selfhost
```

## 4) Migrations

```bash
cat db/migrations/000001_enable_extensions.up.sql | docker compose --env-file .env -f deploy/compose/docker-compose.yml exec -T postgres psql -U vertex -d vertex_rag
cat db/migrations/000002_auth_rbac.up.sql | docker compose --env-file .env -f deploy/compose/docker-compose.yml exec -T postgres psql -U vertex -d vertex_rag
cat db/migrations/000003_documents.up.sql | docker compose --env-file .env -f deploy/compose/docker-compose.yml exec -T postgres psql -U vertex -d vertex_rag
cat db/migrations/000004_kb_version.up.sql | docker compose --env-file .env -f deploy/compose/docker-compose.yml exec -T postgres psql -U vertex -d vertex_rag
cat db/migrations/000005_document_embedding_vector.up.sql | docker compose --env-file .env -f deploy/compose/docker-compose.yml exec -T postgres psql -U vertex -d vertex_rag
cat db/migrations/000006_chat_settings.up.sql | docker compose --env-file .env -f deploy/compose/docker-compose.yml exec -T postgres psql -U vertex -d vertex_rag
cat db/migrations/000007_backfill_can_use_unstrict.up.sql | docker compose --env-file .env -f deploy/compose/docker-compose.yml exec -T postgres psql -U vertex -d vertex_rag
cat db/migrations/000008_document_chunks_embedding_hnsw.up.sql | docker compose --env-file .env -f deploy/compose/docker-compose.yml exec -T postgres psql -U vertex -d vertex_rag
```

## 5) Ollama modes

- External Ollama host: `OLLAMA_BASE_URL=http://host.docker.internal:11434`
- Compose Ollama profile: run with `--profile local-llm`, keep `OLLAMA_BASE_URL=http://ollama:11434`
