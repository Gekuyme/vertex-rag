# Vertex RAG

Open-source, self-hosted, role-aware RAG platform for teams.

Vertex RAG helps organizations upload internal documents, control access by role, and chat with a shared knowledge base in two modes:

- `strict`: grounded answers with citations from approved internal documents.
- `unstrict`: broader answers that can optionally use web search when the role allows it.

The project is designed for teams that need more than a demo chatbot: document access control, predictable ingestion, reproducible local setup, and flexible model providers for both LLMs and embeddings.

## Why Vertex RAG

- Role-aware knowledge access with document visibility restricted by role.
- Cited internal answers for compliance-heavy or trust-sensitive workflows.
- Optional web-augmented responses for broader research and exploratory use cases.
- Self-hosted architecture with PostgreSQL, pgvector, Redis, and S3-compatible storage.
- Pluggable providers for local development and production deployments.

## Core capabilities

- Authentication with owner bootstrap, login, refresh, logout, and account settings.
- Chat history, streaming responses, and per-message `strict` / `unstrict` mode selection.
- Document upload, background ingestion, reingest flows, and retrieval debugging.
- Role and user administration with fine-grained permissions.
- Retrieval and answer caching keyed by organization, role, query, mode, and knowledge-base version.
- Smoke and integration checks for ACL behavior, mode switching, PDF ingestion, retrieval stability, and cache speed.
- Offline retrieval evaluation scaffold for saved debug payloads without model-backed smoke runs.

## Architecture

```text
Next.js web app
        |
        v
     Go API  ---> Redis cache
        |
        +---> PostgreSQL + pgvector
        |
        +---> S3 / MinIO object storage
        |
        +---> LLM provider (local / OpenAI / Gemini / Ollama)
        |
        +---> Web search provider (optional, unstrict only)

Go worker
  |
  +---> pulls ingest jobs from Redis queue
  +---> downloads files from object storage
  +---> extracts text, chunks content, builds embeddings
  +---> writes chunks and vectors back to PostgreSQL
```

## Supported providers

LLM providers:

- `local`
- `openai`
- `gemini`
- `ollama`

Embedding providers:

- `local`
- `openai`
- `ollama`

Supported document formats for ingestion:

- `pdf`
- `docx`
- `txt`
- `md`
- other `text/*` MIME types

## Repository layout

- `apps/web`: Next.js frontend.
- `apps/api`: Go API service.
- `apps/worker`: Go worker service.
- `db/migrations`: SQL migrations.
- `deploy/compose`: Docker Compose setup and self-host notes.
- `scripts`: smoke and operational helper scripts.

## Quick start

### 1. Create local configuration

```bash
cp .env.example .env
```

### 2. Start the local stack

```bash
docker compose --env-file .env -f deploy/compose/docker-compose.yml -f deploy/compose/docker-compose.dev.yml up --build
```

This starts:

- web app on `http://localhost:3000`
- API on `http://localhost:8080`
- worker on `http://localhost:8082`
- PostgreSQL with `pgvector`
- Redis
- MinIO
- optional Ollama when using the `local-llm` profile

To include Ollama:

```bash
docker compose --env-file .env -f deploy/compose/docker-compose.yml -f deploy/compose/docker-compose.dev.yml --profile local-llm up --build
```

### 3. Apply database migrations

```bash
make migrate
```

### 4. Verify services

```bash
make health
```

### 5. Create the first owner account

Use the UI or call the bootstrap endpoint:

```bash
curl -X POST http://localhost:8080/auth/register_owner \
  -H "Content-Type: application/json" \
  -d '{
    "organization_name": "Vertex Demo",
    "email": "owner@vertex.local",
    "password": "Password123!"
  }'
```

## Development workflow

Common commands:

- `make up`: start the full stack in background.
- `make up-core`: start API, worker, Postgres, Redis, and MinIO without the web container.
- `make rebuild`: rebuild and restart all services.
- `make stop-web`: free port `3000` for local Next.js development.
- `make dev-web`: run the Next.js app locally with HMR.
- `make ps`: inspect compose service status.
- `make logs-api`
- `make logs-worker`
- `make logs-web`
- `make reingest-all`: queue reingestion for every document.

Run services directly without Docker when useful:

```bash
cd apps/api && go run ./cmd/api
cd apps/worker && go run ./cmd/worker
cd apps/web && npm install && npm run dev
```

## Model and search configuration

### Embeddings

Set `EMBED_PROVIDER=local|openai|ollama`.

- `local`: deterministic embeddings for development and tests.
- `openai`: requires `OPENAI_API_KEY`, optional `OPENAI_BASE_URL`, and `EMBED_MODEL_OPENAI`.
- `ollama`: requires `OLLAMA_BASE_URL` and `EMBED_MODEL_OLLAMA`.

Recommended local retrieval setup:

```bash
EMBED_PROVIDER=ollama
EMBED_MODEL_OLLAMA=nomic-embed-text
```

After changing the embedding provider or model, reingest documents:

```bash
make reingest-all
```

### LLMs

Set `LLM_PROVIDER=local|openai|gemini|ollama`.

- `local`: deterministic fallback for local development.
- `openai`: requires `OPENAI_API_KEY`, optional `OPENAI_BASE_URL`, and `LLM_MODEL_OPENAI`.
- `gemini`: requires `GEMINI_API_KEY`, optional `GEMINI_BASE_URL`, and `LLM_MODEL_GEMINI`.
- `ollama`: uses `LLM_OLLAMA_BASE_URL`, `LLM_MODEL_OLLAMA`, and optional strict/unstrict overrides.

Reliability controls:

- `LLM_HTTP_TIMEOUT`
- `LLM_MAX_RETRIES`
- `LLM_RETRY_BACKOFF`
- `LLM_MAX_CONTEXT_CHARS`

### Web search in `unstrict`

Optional web search is available only in `unstrict` mode and only for roles that are allowed to use it.

Configuration:

- `WEB_SEARCH_ENABLED=true|false`
- `SEARCH_API_PROVIDER=brave`
- `SEARCH_API_KEY=...`
- `SEARCH_MAX_RESULTS`
- `SEARCH_HTTP_TIMEOUT`

## Security and access model

- `APP_ENV=production` requires a non-default `JWT_SECRET`.
- `CORS_ORIGIN` supports a comma-separated allowlist.
- Cookie behavior is controlled by `COOKIE_SECURE` and `COOKIE_SAMESITE`.
- Basic API rate limiting is controlled by `RATE_LIMIT_RPM` and `RATE_LIMIT_BURST`.
- `strict` answers are designed to stay grounded in indexed internal content.
- `unstrict` access and web search access are permission-gated at the role level.

## Validation and testing

Unit and e2e coverage already exist for key flows.

- `make test`: run Go unit tests for API and worker.
- `make test-e2e`: run Playwright end-to-end smoke.
- `make test-integration`: run smoke-backed integration suite.
- `make smoke-role-acl`
- `make smoke-mode-toggle`
- `make smoke-pdf-ingest`
- `make smoke-retrieval-stability`
- `make smoke-cache-speed`
- `make smoke-query-matrix`
- `python3 scripts/retrieval_offline_eval.py --cases docs/evals/retrieval_eval_seed.json --results <saved-results.json>`
- `python3 scripts/retrieval_collect_debug.py --cases docs/evals/retrieval_eval_seed.json --output-dir tmp/evals/v2 --pipeline-version v2`
- `make seed-eval-corpus`

Script details are documented in [scripts/README.md](scripts/README.md).
Low-cost retrieval evaluation workflow is documented in [docs/evals/README.md](docs/evals/README.md).

## Self-hosting

For private-registry deployment and production-oriented notes, see [deploy/compose/SELF_HOST.md](deploy/compose/SELF_HOST.md).

## Current API surface

Implemented endpoints include:

- auth: register owner, login, refresh, logout, current user, settings
- chats: list, create, inspect, delete, list messages, create message, stream message
- admin: list and manage roles, list users, update user role, retrieval debug, top-doc stats
- documents: list, upload, reingest one, reingest all

See [apps/api/internal/httpserver/server.go](apps/api/internal/httpserver/server.go) for the current route map.

## Roadmap

Near-term priorities:

- improve public docs and examples for contributors
- expand automated tests around retrieval quality and failure handling
- grow the offline retrieval eval set from the current seed scaffold to a real benchmark set
- add public evaluation datasets and benchmarks for strict-answer workflows
- improve observability around ingestion, retrieval, and latency
- keep provider integrations interchangeable across local and hosted setups

## Open source readiness

This repository is intended to be a public, contributor-friendly project.

- License: [MIT](LICENSE)
- Third-party notices: [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md)
- Contributing guide: [CONTRIBUTING.md](CONTRIBUTING.md)
- Code of conduct: [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md)
- Security policy: [SECURITY.md](SECURITY.md)
- Maintainer guide: [docs/maintaining-open-source.md](docs/maintaining-open-source.md)

For OpenAI application positioning, see [docs/openai-codex-open-source-fund.md](docs/openai-codex-open-source-fund.md).
