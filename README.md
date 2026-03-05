# Vertex RAG

Initial MVP scaffold for a business-focused RAG assistant platform.

## Structure

- `apps/web` - Next.js frontend
- `apps/api` - Go API service
- `apps/worker` - Go worker service
- `deploy/compose` - Docker Compose setup
- `db/migrations` - SQL migrations
- `scripts` - helper scripts

## Runtime requirements

- Go `1.24+` for API and worker services.
- Docker Desktop (or Docker Engine + Compose plugin) for full stack.

## Quick start

1. Copy environment template:

```bash
cp .env.example .env
```

2. Start local services:

```bash
docker compose --env-file .env -f deploy/compose/docker-compose.yml -f deploy/compose/docker-compose.dev.yml up --build
```

3. Apply database migrations:

```bash
cat db/migrations/000001_enable_extensions.up.sql | docker compose -f deploy/compose/docker-compose.yml exec -T postgres psql -U vertex -d vertex_rag
cat db/migrations/000002_auth_rbac.up.sql | docker compose -f deploy/compose/docker-compose.yml exec -T postgres psql -U vertex -d vertex_rag
cat db/migrations/000003_documents.up.sql | docker compose -f deploy/compose/docker-compose.yml exec -T postgres psql -U vertex -d vertex_rag
cat db/migrations/000004_kb_version.up.sql | docker compose -f deploy/compose/docker-compose.yml exec -T postgres psql -U vertex -d vertex_rag
cat db/migrations/000005_document_embedding_vector.up.sql | docker compose -f deploy/compose/docker-compose.yml exec -T postgres psql -U vertex -d vertex_rag
cat db/migrations/000006_chat_settings.up.sql | docker compose -f deploy/compose/docker-compose.yml exec -T postgres psql -U vertex -d vertex_rag
cat db/migrations/000007_backfill_can_use_unstrict.up.sql | docker compose -f deploy/compose/docker-compose.yml exec -T postgres psql -U vertex -d vertex_rag
```

4. Check health endpoints:

```bash
curl http://localhost:8080/healthz
curl http://localhost:8082/healthz
```

## Make targets

- `make rebuild` - rebuild and restart all services.
- `make up-selfhost` - run prebuilt private images only (no local build).
- `make pull-selfhost` - pull prebuilt private images.
- `make migrate` - apply SQL migrations (`000001`..`000007`).
- `make reingest-all` - queue reingest for every document.
- `make test` - run API and worker tests.
- `make test-e2e` - run Playwright e2e smoke (`login -> upload -> strict chat`).
- `make test-integration` - run integration test targets.
- `make test-integration-acl` - run ACL integration scenario.
- `make test-integration-mode` - verify strict/unstrict toggle applies to next request only.
- `make test-integration-pdf` - verify PDF ingest `ready` + non-empty embeddings + strict citations.
- `make test-integration-retrieval` - verify retrieval stability + unstrict RBAC on internal chunks.
- `make test-integration-cache` - verify repeated strict query gets faster from cache.
- `make smoke-role-acl` - run role ACL smoke scenario (restricted document visibility by role).
- `make ps` - show compose status.

## Embeddings providers

API and worker support `EMBED_PROVIDER=local|openai|ollama`:

- `local` - deterministic local embeddings for dev/no external dependency.
- `openai` - set `OPENAI_API_KEY`, optional `OPENAI_BASE_URL`, and `EMBED_MODEL_OPENAI`.
- `ollama` - set `OLLAMA_BASE_URL` and `EMBED_MODEL_OLLAMA` (use compose `local-llm` profile if needed).
  - Recommended for real retrieval quality: `EMBED_PROVIDER=ollama` and `EMBED_MODEL_OLLAMA=nomic-embed-text`.
  - After changing embedding provider/model, run `make reingest-all`.

## LLM providers

API supports `LLM_PROVIDER=local|openai|ollama`:

- `local` - deterministic fallback for local development.
- `openai` - set `OPENAI_API_KEY`, optional `OPENAI_BASE_URL`, and `LLM_MODEL_OPENAI`.
- `ollama` - set `OLLAMA_BASE_URL` and `LLM_MODEL_OLLAMA`.
  - If Ollama runs on host machine (outside compose), use `OLLAMA_BASE_URL=http://host.docker.internal:11434`.
  - If Ollama runs as compose service (`--profile local-llm`), use `OLLAMA_BASE_URL=http://ollama:11434`.
  - Optional tuning: `LLM_OLLAMA_NUM_CTX` (default `4096`) and `LLM_OLLAMA_KEEP_ALIVE` (default `30m`).

Provider reliability controls:

- `LLM_HTTP_TIMEOUT` (default `60s`)
- `LLM_MAX_RETRIES` (default `2`)
- `LLM_RETRY_BACKOFF` (default `400ms`)
- `LLM_MAX_CONTEXT_CHARS` (default `7000`)

## Unstrict web-search (optional)

Unstrict mode can enrich answers with web snippets behind a feature flag:

- `WEB_SEARCH_ENABLED=true|false`
- `SEARCH_API_PROVIDER=brave`
- `SEARCH_API_KEY=...`
- `SEARCH_MAX_RESULTS` (default `5`)
- `SEARCH_HTTP_TIMEOUT` (default `6s`)

Web-search context is only injected in `unstrict` mode and only for roles with `can_toggle_web_search`.

`unstrict` access itself is controlled by `can_use_unstrict`. For backward compatibility one rollout supports `UNSTRICT_LEGACY_TOGGLE_WEB_SEARCH=true`, which temporarily treats `can_toggle_web_search` as sufficient for `unstrict`.

## Redis cache (RAG)

API supports Redis cache for retrieval and strict answers:

- `CACHE_ENABLED=true|false`
- `CACHE_RETRIEVAL_TTL` (default `10m`)
- `CACHE_ANSWER_TTL` (default `10m`)
- `CACHE_UNSTRICT_ANSWER=true|false` (default `false`)

Cache keys include `org_id`, `role_id`, `mode`, normalized query hash, and `kb_version`.

## Security/hardening

- `APP_ENV=production` requires non-default `JWT_SECRET`.
- `CORS_ORIGIN` supports comma-separated allowlist.
- Cookie controls: `COOKIE_SECURE`, `COOKIE_SAMESITE=lax|strict|none`.
- Basic API rate limiting: `RATE_LIMIT_RPM`, `RATE_LIMIT_BURST`.

## Self-host guide

- See `deploy/compose/SELF_HOST.md` for private registry deployment, migrations, and Ollama variants.

## Implemented API endpoints (current)

- `POST /auth/register_owner`
- `POST /auth/login`
- `POST /auth/refresh`
- `POST /auth/logout`
- `GET /me`
- `GET /me/settings`
- `PATCH /me/settings`
- `GET /roles`
- `GET /chats`
- `POST /chats`
- `GET /chats/{id}`
- `DELETE /chats/{id}`
- `GET /chats/{id}/messages`
- `POST /chats/{id}/messages`
- `POST /chats/{id}/messages/stream`
- `GET /admin/roles`
- `POST /admin/roles`
- `PATCH /admin/roles/{id}`
- `DELETE /admin/roles/{id}`
- `GET /admin/users`
- `PATCH /admin/users/{id}/role`
- `POST /admin/retrieval/debug`
- `GET /admin/stats/top-docs`
- `GET /documents`
- `POST /documents/upload`
