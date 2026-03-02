# Repository Guidelines

## Project Structure & Module Organization
This repository is a small monorepo split by runtime:

- `apps/web`: Next.js frontend (`app/` router, global styles, Dockerfile).
- `apps/api`: Go API service (`cmd/api/main.go` entrypoint, `internal/httpserver` for HTTP wiring).
- `apps/worker`: Go worker service with the same layout as API.
- `db/migrations`: SQL migrations using paired `*.up.sql` and `*.down.sql` files.
- `deploy/compose`: local multi-service stack (`docker-compose.yml`).
- `scripts`: helper scripts (currently minimal scaffold).

## Build, Test, and Development Commands
- `cp .env.example .env`: create local environment config.
- `docker compose -f deploy/compose/docker-compose.yml up --build`: start full local stack.
- `docker compose -f deploy/compose/docker-compose.yml up --build --profile local-llm`: include Ollama profile.
- `cd apps/web && npm install && npm run dev`: run frontend on port `3000`.
- `cd apps/web && npm run build && npm run start`: production build/run check.
- `cd apps/api && go run ./cmd/api`: run API on `API_PORT` (default `8080`).
- `cd apps/worker && go run ./cmd/worker`: run worker on `WORKER_PORT` (default `8082`).
- `curl http://localhost:8080/healthz && curl http://localhost:8082/healthz`: basic service verification.

## Coding Style & Naming Conventions
- Go: always format with `gofmt`; keep packages lowercase; use `PascalCase` for exported identifiers.
- TypeScript/React: follow existing 2-space indentation and `PascalCase` component names.
- Environment variables: uppercase snake case (example: `NEXT_PUBLIC_API_BASE_URL`).
- Migrations: use sequential numeric prefixes, e.g. `000002_add_documents_table.up.sql`.

## Testing Guidelines
Automated tests are not yet present; add them with new features:

- Go tests: colocate as `*_test.go` and run `go test ./...` inside each Go app.
- Web tests: use `*.test.ts` or `*.test.tsx` naming if a test runner is introduced.
- At minimum, validate with health checks and relevant local run commands before opening a PR.

## Commit & Pull Request Guidelines
- Current history uses short, imperative commit subjects (example: `Add MVP roadmap checklist`).
- Keep commits focused to one logical change and subject lines concise (target <= 72 chars).
- PRs should include: purpose, touched paths, config/migration changes, validation steps, and screenshots for UI changes.
- Link the related issue or roadmap item when available.

## Security & Configuration Tips
- Never commit real secrets; keep `.env` local and start from `.env.example`.
- Treat API keys, DB credentials, and provider tokens as runtime-only secrets.
- Avoid committing generated artifacts (`apps/web/.next/`, logs, service binaries).
