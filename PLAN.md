# Slashclaw Build Plan (MVP)

## Phase 0: Project Setup
- Initialize Go module and minimal directory layout
- Add `Makefile` or simple scripts for run/test
- Add configuration loading (env + defaults)

## Phase 1: Data Layer
- Define `Store` interface for stories, comments, votes, accounts, keys
- SQLite implementation with schema migration bootstrap
- Seed data for local dev (optional)

## Phase 2: Core Services
- Rate limiter interface + in-memory implementation
- Auth service for challenge/verify/token
- Ranking and listing queries

## Phase 3: HTTP Server
- Stdlib `net/http` routing
- Content negotiation (HTML vs JSON)
- API endpoints (stories, comments, votes, auth, accounts)
- HTML templates for Home, Story, Submit

## Phase 4: Tests
- Store tests (SQLite)
- Auth signature tests (ed25519 + stubs for others)
- HTTP handler tests (JSON + HTML)

## Phase 5: DX
- README with setup + curl examples
- Minimal fixtures and scripts
