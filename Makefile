COMPOSE_FILE := deploy/compose/docker-compose.yml
ENV_FILE ?= .env
COMPOSE := docker compose --env-file $(ENV_FILE) -f $(COMPOSE_FILE)

.PHONY: help up up-core up-build rebuild rebuild-web rebuild-api rebuild-worker down ps logs logs-api logs-web logs-worker \
	stop-web \
	migrate migrate-extensions migrate-auth migrate-docs migrate-kb-version migrate-embedding-vector migrate-chat-settings \
	test test-api test-worker build-web dev-web health

help:
	@echo "Available targets:"
	@echo "  make up                  - Start stack in background"
	@echo "  make up-core             - Start stack without Web (for local Next dev)"
	@echo "  make up-build            - Start stack with image rebuild"
	@echo "  make rebuild             - Rebuild and restart stack"
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
	@echo "  make test                - Run API and Worker tests"
	@echo "  make build-web           - Build Next.js app"
	@echo "  make dev-web             - Run Next.js dev server locally (HMR)"
	@echo "  make health              - Check API and Worker health"

up:
	$(COMPOSE) up -d

up-core:
	$(COMPOSE) up -d postgres redis minio api worker

up-build:
	$(COMPOSE) up -d --build

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

migrate: migrate-extensions migrate-auth migrate-docs migrate-kb-version migrate-embedding-vector migrate-chat-settings

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

test: test-api test-worker

test-api:
	cd apps/api && go test ./...

test-worker:
	cd apps/worker && go test ./...

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
