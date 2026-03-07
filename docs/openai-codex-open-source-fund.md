# Vertex RAG and the OpenAI Codex Open Source Fund

This note is meant to support public project positioning and future application work.

## Project summary

Vertex RAG is an open-source, self-hosted, role-aware RAG platform for teams and organizations. It combines:

- a Next.js frontend
- Go API and worker services
- PostgreSQL with pgvector
- Redis-backed background work and caching
- S3-compatible object storage
- pluggable local, OpenAI, Gemini, and Ollama model providers

The project focuses on trustworthy internal knowledge workflows, especially:

- strict cited answers grounded in indexed documents
- role-based document access
- predictable ingestion and reingestion
- deployability outside closed SaaS environments

## Why this project is a strong fit

- It is infrastructure, not just a prompt wrapper.
- It solves a real open-source problem space: secure team knowledge assistants that can be self-hosted.
- It already includes working ingestion, retrieval, RBAC, caching, testing, and deployment scaffolding.
- API credits would accelerate public evaluation and provider-quality comparisons rather than substitute for a product idea.

## How OpenAI credits would be used

If accepted, API credits should be directed toward high-leverage open-source work:

- improving strict-answer citation reliability
- benchmarking retrieval quality across real document sets
- building public evals for grounded enterprise-style questions
- testing multilingual ingestion and answer quality
- validating provider fallbacks and failure handling
- documenting reproducible OpenAI-backed reference deployments

## Proposed milestone framing

Short-term milestones that fit the fund well:

1. Publish benchmark datasets and evaluation harnesses for strict-answer workflows.
2. Improve answer-grounding checks and citation validation in CI and smoke tests.
3. Add polished public docs, onboarding, and reference deployment examples.
4. Expand provider comparison results for OpenAI, Ollama, and local development paths.

## Short application-ready description

Vertex RAG is an open-source, self-hosted, role-aware RAG platform for teams. It lets organizations upload internal documents, restrict access by role, and chat with a shared knowledge base using strict cited answers or broader unstrict responses with optional web search. We would use OpenAI credits to improve retrieval quality, citation reliability, and public evaluation tooling for grounded knowledge workflows.
