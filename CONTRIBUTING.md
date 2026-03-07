# Contributing to Vertex RAG

Thanks for contributing.

## What we are building

Vertex RAG is an open-source, self-hosted, role-aware RAG platform for teams. Contributions should move the project toward:

- grounded, citable retrieval flows
- secure role-based access to internal knowledge
- predictable local and self-hosted deployment
- provider flexibility across local and hosted environments

## Before you start

- Open an issue or discussion for larger changes before writing a lot of code.
- Keep pull requests focused to one logical change.
- Prefer small, reviewable iterations over broad refactors.
- If a change affects behavior, include validation steps and tests where practical.

## Local development

1. Copy the environment template:

```bash
cp .env.example .env
```

2. Start the stack:

```bash
docker compose --env-file .env -f deploy/compose/docker-compose.yml -f deploy/compose/docker-compose.dev.yml up --build
```

3. Apply migrations:

```bash
make migrate
```

4. Run checks relevant to your change:

```bash
make test
make test-e2e
make test-integration
```

## Project conventions

- Go: run `gofmt` and keep package names lowercase.
- TypeScript/React: follow the existing 2-space indentation and component conventions.
- SQL migrations: add paired `*.up.sql` and `*.down.sql` files with sequential numeric prefixes.
- Environment variables: use uppercase snake case.

## Pull request expectations

Each PR should include:

- a short statement of purpose
- touched paths or subsystems
- config, schema, or migration impact
- validation steps you ran
- screenshots for UI changes when relevant

## Areas that are especially useful

- retrieval quality and citation correctness
- ingestion reliability and document parsing
- self-hosting and deployment ergonomics
- observability, testing, and reproducible benchmarks
- access control and tenant isolation

## Community standards

By participating, you agree to follow the [Code of Conduct](CODE_OF_CONDUCT.md).
