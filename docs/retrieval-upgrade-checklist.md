# Retrieval Upgrade Checklist

This checklist tracks the retrieval/platform upgrade in the current worktree.

## Foundation

- [x] Add retrieval v2 foundation in API
- [x] Add dense retrieval path for pgvector
- [x] Add sparse retrieval client for OpenSearch BM25
- [x] Add RRF fusion for dense + sparse candidates
- [x] Add reranker client hook
- [x] Add query analysis and query variant generation hooks
- [x] Expand retrieval debug payload for retrieval v2
- [x] Add parent-child ingestion foundation in worker
- [x] Add `document_sections` migration
- [x] Store parent sections and child chunks in Postgres
- [x] Sync sparse index from worker to OpenSearch
- [x] Add runtime/config wiring for OpenSearch and reranker
- [x] Keep API and worker test suites green after foundation changes

## Platform changes already present in the worktree

- [x] Add session-based refresh token handling
- [x] Add configurable LLM runtime and per-user/provider model selection
- [x] Add Gemini provider support for embeddings and LLM runtime
- [x] Add richer frontend message rendering and model-selection plumbing

## Current implementation focus

- [x] Add grounding summary to sync chat response payload
- [x] Add grounding summary to stream `done` payload
- [x] Add grounding summary to retrieval debug response
- [x] Compute confidence score from retrieval evidence
- [x] Compute conservative contradiction signals from retrieved context
- [x] Include multi-document coverage summary in grounding metadata
- [x] Extend frontend chat response types with grounding metadata
- [x] Store grounding metadata keyed by assistant message
- [x] Render trust summary for assistant answers in the UI
- [x] Expand source preview with parent/rank/fusion metadata
- [x] Surface contradiction warnings in the UI
- [x] Add exact evidence span to citation payload and preview UI
- [x] Strengthen comparison-aware retrieval coverage across entities/documents
- [x] Add deterministic query rewrite/expansion fallback policy
- [x] Filter query variants by critical-term preservation
- [x] Improve confidence scoring with coverage/agreement/reranker signals
- [x] Add HyDE dense-only path behind feature flag
- [x] Add debug-level baseline vs v2 retrieval comparison
- [x] Add router-lite for `vector+sparse`, `web`, and `no-retrieval`

## Validation still needed

- [x] Run `cd apps/api && go test ./...` after grounding/UI changes
- [x] Run `cd apps/worker && go test ./...` after grounding/UI changes
- [x] Run `cd apps/web && npm run build` after grounding/UI changes
- [ ] Run end-to-end local stack validation with OpenSearch enabled
- [ ] Verify `/admin/retrieval/debug` end-to-end on the local stack
- [ ] Verify upload -> ingest -> strict chat -> source preview flow

## Suggested future work

- [ ] Build offline eval set for exact fact / ambiguous / comparison queries
- [x] Expand seed eval batch to 52 categorized cases
- [x] Add offline eval harness for saved retrieval debug results
- [x] Add seed retrieval eval dataset scaffold
- [x] Add dedicated smoke coverage for grounding/confidence/contradiction metadata
- [x] Add sentence-level citation highlighting in UI
- [x] Add stronger contradiction detection beyond conservative heuristics
