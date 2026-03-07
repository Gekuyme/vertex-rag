# Maintaining Vertex RAG in Open Source

This document describes how to grow the project as an open-source codebase instead of treating GitHub as a code dump.

## Maintainer priorities

Prioritize work that improves one of these:

- trustworthiness of strict answers
- tenant and role isolation
- ingestion reliability and debuggability
- reproducible local and self-hosted operation
- public docs, evals, and contributor ergonomics

If a contribution does not clearly improve one of those, it should need stronger justification.

## Default contribution flow

1. Capture the problem in an issue.
2. Agree on scope before large implementation work.
3. Keep changes small enough for a focused review.
4. Require validation steps in every PR.
5. Merge only after CI is green and the change is understandable from the PR alone.

## What to automate first

The repository should keep a fast, reliable default automation baseline:

- Go tests for `apps/api`
- Go tests for `apps/worker`
- production build check for `apps/web`
- dependency update automation with review, not auto-merge

Add slower end-to-end and compose-based checks only after they are repeatable enough not to create noisy failures.

## Backlog structure

Keep the backlog legible for new contributors by labeling work in a few stable buckets:

- `good first issue`
- `help wanted`
- `bug`
- `enhancement`
- `docs`
- `retrieval`
- `ingestion`
- `auth`
- `web`
- `api`
- `worker`
- `ci`

## Review standard

For every PR, maintainers should ask:

- Is the problem statement clear?
- Does the solution preserve RBAC and tenant boundaries?
- Does it change retrieval or citation behavior?
- Is the validation strong enough for the risk level?
- Are docs, config examples, and migrations updated if needed?

## Release hygiene

Before tagging releases:

1. Re-run the main validation paths.
2. Check migration compatibility and rollback posture.
3. Verify README and `.env.example` still match reality.
4. Summarize user-visible changes and upgrade notes.

## OpenAI fund alignment

If the project receives external support such as OpenAI API credits, use that support on work that remains public and reusable:

- benchmarks
- eval harnesses
- reference integrations
- docs
- reliability improvements

Avoid spending project energy on private one-off customizations that do not compound for the open-source codebase.
