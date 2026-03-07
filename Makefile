COMPOSE_FILE := deploy/compose/docker-compose.yml
COMPOSE_DEV_FILE := deploy/compose/docker-compose.dev.yml
ENV_FILE ?= .env
COMPOSE := docker compose --env-file $(ENV_FILE) -f $(COMPOSE_FILE) -f $(COMPOSE_DEV_FILE)
COMPOSE_SELFHOST := docker compose --env-file $(ENV_FILE) -f $(COMPOSE_FILE)

.PHONY: help up up-core up-build rebuild rebuild-web rebuild-api rebuild-worker down ps logs logs-api logs-web logs-worker \
	stop-web \
	migrate migrate-extensions migrate-auth migrate-docs migrate-kb-version migrate-embedding-vector migrate-chat-settings migrate-unstrict-backfill migrate-hnsw-index reingest-all \
	test test-api test-worker test-integration test-integration-acl test-integration-mode test-integration-pdf test-integration-retrieval test-integration-cache \
	test-e2e \
	build-web dev-web health smoke-user smoke-role-acl smoke-mode-toggle smoke-pdf-ingest smoke-retrieval-stability smoke-cache-speed smoke-query-matrix smoke-query-matrix-lite up-selfhost pull-selfhost

help:
	@echo "Available targets:"
	@echo "  make up                  - Start stack in background"
	@echo "  make up-core             - Start stack without Web (for local Next dev)"
	@echo "  make up-build            - Start stack with image rebuild"
	@echo "  make rebuild             - Rebuild and restart stack"
	@echo "  make up-selfhost         - Start from prebuilt images only"
	@echo "  make pull-selfhost       - Pull prebuilt images for self-host"
	@echo "  make rebuild-web         - Rebuild and restart only Web"
	@echo "  make rebuild-api         - Rebuild and restart only API"
	@echo "  make rebuild-worker      - Rebuild and restart only Worker"
	@echo "  make stop-web            - Stop Web container (free :3000 for dev)"
	@echo "  make down                - Stop stack"
	@echo "  make ps                  - Show service status"
	@echo "  make logs                - Tail all logs"
	@echo "  make logs-api            - Tail API logs"
	@echo "  make logs-web            - Tail Web logs"
	@echo "  make logs-worker         - Tail Worker logs"
	@echo "  make migrate             - Apply all SQL migrations"
	@echo "  make reingest-all        - Queue reingest for every document"
	@echo "  make test                - Run API and Worker tests"
	@echo "  make test-e2e            - Run Playwright e2e smoke test"
	@echo "  make test-integration    - Run integration test suite"
	@echo "  make test-integration-acl - Run restricted doc ACL integration test"
	@echo "  make test-integration-mode - Run strict/unstrict toggle integration test"
	@echo "  make test-integration-pdf - Run PDF ingest + strict citations integration test"
	@echo "  make test-integration-retrieval - Run retrieval stability + unstrict RBAC integration test"
	@echo "  make test-integration-cache - Run strict cache performance integration test"
	@echo "  make smoke-role-acl      - Run role-based ACL smoke scenario"
	@echo "  make smoke-mode-toggle   - Run in-flight mode toggle smoke scenario"
	@echo "  make smoke-pdf-ingest    - Run PDF ingest readiness smoke scenario"
	@echo "  make smoke-retrieval-stability - Run retrieval stability smoke scenario"
	@echo "  make smoke-cache-speed   - Run strict cache speed smoke scenario"
	@echo "  make smoke-query-matrix  - Run query matrix across modes and speed profiles"
	@echo "  make smoke-query-matrix-lite - Run reduced query matrix for quota-limited providers"
	@echo "  make build-web           - Build Next.js app"
	@echo "  make dev-web             - Run Next.js dev server locally (HMR)"
	@echo "  make health              - Check API and Worker health"
	@echo "  make smoke-user          - Ensure persistent smoke user in Vertex Demo"

up:
	$(COMPOSE) up -d

up-core:
	$(COMPOSE) up -d postgres redis minio api worker

up-build:
	$(COMPOSE) up -d --build

up-selfhost:
	$(COMPOSE_SELFHOST) up -d

pull-selfhost:
	$(COMPOSE_SELFHOST) pull

rebuild:
	$(COMPOSE) up -d --build --force-recreate

rebuild-web:
	$(COMPOSE) up -d --build --force-recreate --no-deps web

rebuild-api:
	$(COMPOSE) up -d --build --force-recreate --no-deps api

rebuild-worker:
	$(COMPOSE) up -d --build --force-recreate --no-deps worker

stop-web:
	$(COMPOSE) stop web

down:
	$(COMPOSE) down

ps:
	$(COMPOSE) ps

logs:
	$(COMPOSE) logs -f

logs-api:
	$(COMPOSE) logs -f api

logs-web:
	$(COMPOSE) logs -f web

logs-worker:
	$(COMPOSE) logs -f worker

migrate: migrate-extensions migrate-auth migrate-docs migrate-kb-version migrate-embedding-vector migrate-chat-settings migrate-unstrict-backfill migrate-hnsw-index

migrate-extensions:
	cat db/migrations/000001_enable_extensions.up.sql | $(COMPOSE) exec -T postgres psql -U vertex -d vertex_rag

migrate-auth:
	cat db/migrations/000002_auth_rbac.up.sql | $(COMPOSE) exec -T postgres psql -U vertex -d vertex_rag

migrate-docs:
	cat db/migrations/000003_documents.up.sql | $(COMPOSE) exec -T postgres psql -U vertex -d vertex_rag

migrate-kb-version:
	cat db/migrations/000004_kb_version.up.sql | $(COMPOSE) exec -T postgres psql -U vertex -d vertex_rag

migrate-embedding-vector:
	cat db/migrations/000005_document_embedding_vector.up.sql | $(COMPOSE) exec -T postgres psql -U vertex -d vertex_rag

migrate-chat-settings:
	cat db/migrations/000006_chat_settings.up.sql | $(COMPOSE) exec -T postgres psql -U vertex -d vertex_rag

migrate-unstrict-backfill:
	cat db/migrations/000007_backfill_can_use_unstrict.up.sql | $(COMPOSE) exec -T postgres psql -U vertex -d vertex_rag

migrate-hnsw-index:
	cat db/migrations/000008_document_chunks_embedding_hnsw.up.sql | $(COMPOSE) exec -T postgres psql -U vertex -d vertex_rag

reingest-all:
	@token="$$(curl -fsS -X POST http://localhost:8080/auth/login -H "Content-Type: application/json" -d "{\"email\":\"$${OWNER_EMAIL:-owner@vertex.local}\",\"password\":\"$${OWNER_PASSWORD:-Password123!}\"}" | jq -r '.access_token')" && \
	curl -fsS -X POST http://localhost:8080/documents/reingest_all -H "Authorization: Bearer $$token"

test: test-api test-worker

test-integration: test-integration-acl test-integration-mode test-integration-pdf test-integration-retrieval test-integration-cache

test-integration-acl: smoke-role-acl

test-integration-mode: smoke-mode-toggle

test-integration-pdf: smoke-pdf-ingest

test-integration-retrieval: smoke-retrieval-stability

test-integration-cache: smoke-cache-speed

test-api:
	cd apps/api && go test ./...

test-worker:
	cd apps/worker && go test ./...

test-e2e:
	cd apps/web && npm run test:e2e

build-web:
	cd apps/web && npm run build

dev-web:
	cd apps/web && npm run dev

health:
	@echo "API:"
	@curl -sS http://localhost:8080/healthz
	@echo
	@echo "Worker:"
	@curl -sS http://localhost:8082/healthz
	@echo

smoke-role-acl:
	./scripts/smoke_role_acl.sh

smoke-mode-toggle:
	./scripts/smoke_mode_toggle.sh

smoke-pdf-ingest:
	./scripts/smoke_pdf_ingest.sh

smoke-retrieval-stability:
	./scripts/smoke_retrieval_stability.sh

smoke-cache-speed:
	./scripts/smoke_cache_speed.sh

smoke-user:
	./scripts/ensure_smoke_user.sh

smoke-query-matrix:
	./scripts/smoke_query_matrix.sh

smoke-query-matrix-lite:
	QUERY_SET=lite CASE_SET=lite ./scripts/smoke_query_matrix.sh
