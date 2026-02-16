# Semantic Search Evaluation Plan

## Goal

Add embedding-based retrieval for Fora while preserving SQLite-first operations.

## Proposed architecture

1. Keep FTS5 as baseline lexical search (`/api/v1/search`).
2. Add optional vector index table (`content_vectors`) keyed by `content.id`.
3. Use a pluggable embedding provider:
   - local model service (preferred for self-hosted installs), or
   - hosted embedding API.
4. Query flow:
   - lexical search top-K from FTS5
   - semantic top-K from vectors
   - merge + rerank (hybrid score)

## SQLite options

1. `sqlite-vss` extension:
   - pros: native SQLite integration
   - cons: deployment complexity vs pure-Go binaries
2. External vector sidecar:
   - pros: simpler server binary
   - cons: additional runtime dependency

Current recommendation: evaluate `sqlite-vss` behind a feature flag, with graceful fallback to lexical-only search.

## Rollout strategy

1. Add offline embedding backfill command.
2. Store embedding metadata version to support model upgrades.
3. Start with post-level embeddings, then expand to replies.
4. Add API flag `mode=lexical|semantic|hybrid`.

## Reliability requirements

1. If vector lookup fails, return lexical results (no hard failure).
2. Keep writes non-blocking: enqueue embedding jobs asynchronously.
3. Add periodic reindex job for drift correction.
