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
docker compose -f deploy/compose/docker-compose.yml up --build
```

3. Apply database migrations:

```bash
cat db/migrations/000001_enable_extensions.up.sql | docker compose -f deploy/compose/docker-compose.yml exec -T postgres psql -U vertex -d vertex_rag
cat db/migrations/000002_auth_rbac.up.sql | docker compose -f deploy/compose/docker-compose.yml exec -T postgres psql -U vertex -d vertex_rag
cat db/migrations/000003_documents.up.sql | docker compose -f deploy/compose/docker-compose.yml exec -T postgres psql -U vertex -d vertex_rag
cat db/migrations/000004_kb_version.up.sql | docker compose -f deploy/compose/docker-compose.yml exec -T postgres psql -U vertex -d vertex_rag
cat db/migrations/000005_document_embedding_vector.up.sql | docker compose -f deploy/compose/docker-compose.yml exec -T postgres psql -U vertex -d vertex_rag
```

4. Check health endpoints:

```bash
curl http://localhost:8080/healthz
curl http://localhost:8082/healthz
```

## Make targets

- `make rebuild` - rebuild and restart all services.
- `make migrate` - apply SQL migrations (`000001`..`000005`).
- `make test` - run API and worker tests.
- `make ps` - show compose status.

## Embeddings providers

Worker supports `EMBED_PROVIDER=local|openai|ollama`:

- `local` (default) - deterministic local embeddings for dev/no external dependency.
- `openai` - set `OPENAI_API_KEY`, optional `OPENAI_BASE_URL`, and `EMBED_MODEL_OPENAI`.
- `ollama` - set `OLLAMA_BASE_URL` and `EMBED_MODEL_OLLAMA` (use compose `local-llm` profile if needed).

## Implemented API endpoints (current)

- `POST /auth/register_owner`
- `POST /auth/login`
- `POST /auth/refresh`
- `POST /auth/logout`
- `GET /me`
- `GET /roles`
- `GET /admin/roles`
- `POST /admin/roles`
- `GET /admin/users`
- `PATCH /admin/users/{id}/role`
- `GET /documents`
- `POST /documents/upload`
