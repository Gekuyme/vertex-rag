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
```

4. Check health endpoints:

```bash
curl http://localhost:8080/healthz
curl http://localhost:8082/healthz
```

## Implemented API endpoints (current)

- `POST /auth/register_owner`
- `POST /auth/login`
- `POST /auth/refresh`
- `POST /auth/logout`
- `GET /me`
- `GET /admin/roles`
- `POST /admin/roles`
- `GET /admin/users`
- `PATCH /admin/users/{id}/role`
