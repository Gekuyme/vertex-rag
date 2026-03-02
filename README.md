# Vertex RAG

Initial MVP scaffold for a business-focused RAG assistant platform.

## Structure

- `apps/web` - Next.js frontend
- `apps/api` - Go API service
- `apps/worker` - Go worker service
- `deploy/compose` - Docker Compose setup
- `db/migrations` - SQL migrations
- `scripts` - helper scripts

## Quick start

1. Copy environment template:

```bash
cp .env.example .env
```

2. Start local services:

```bash
docker compose -f deploy/compose/docker-compose.yml up --build
```

3. Check health endpoints:

```bash
curl http://localhost:8080/healthz
curl http://localhost:8082/healthz
```

