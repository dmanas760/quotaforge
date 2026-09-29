# Memory — QuotaForge

> **AI working rule (from Rules.md):** Read this file first. Update at the end of each implemented phase.

---

## Project snapshot

| Item | Value |
|---|---|
| Module | `github.com/quotaforge/quotaforge` |
| Language | Go 1.24 |
| Project root | `d:\Backend Projects\Project 1\quotaforge\` |
| Start date | 2026-09-12 |

---

## Phase status

| Phase | Name | Status |
|---|---|---|
| 0 | Foundation & Contract | ✅ Complete |
| 1 | Secure Configuration Plane | ⬜ Not started |
| 2 | Token-Bucket Decision Engine | ⬜ Not started |
| 3 | Sliding-Window Engine | ⬜ Not started |
| 4 | Production Hardening | ⬜ Not started |
| 5 | Performance & Distributed Validation | ⬜ Not started |
| 6 | Portfolio Polish | ⬜ Not started |

---

## Phase 0 — What was built

All files committed in the initial implementation pass:

### Infrastructure
- `docker-compose.yml` — Postgres 16 + Redis 7 + API service with healthchecks
- `deployments/docker/Dockerfile` — multi-stage Go 1.24 → Alpine image
- `Makefile` — build, test, test-race, lint, migrate, run, docker-up/down, load-test
- `.github/workflows/ci.yml` — GitHub Actions: vet + test + race + golangci-lint on Postgres+Redis containers
- `api/openapi.yaml` — OpenAPI 3.1 spec for all endpoints

### Database
- `migrations/00001_create_tenants.sql` — tenants table + default tenant seed
- `migrations/00002_create_service_api_keys.sql` — credential table (hash only, never raw key)
- `migrations/00003_create_policies.sql` — policies with version column
- `migrations/00004_create_client_policy_assignments.sql` — UNIQUE(tenant_id, client_id) for upsert
- `migrations/00005_create_audit_events.sql` — audit log with keyset-pagination index

### Go packages
| Package | What it does |
|---|---|
| `internal/config` | Env-var config loader |
| `internal/domain` | Pure domain types, interfaces (Limiter, PolicyRepository, …) |
| `internal/observability` | slog JSON logger, Prometheus metrics |
| `internal/transport/http/middleware` | RequestID, Auth (Argon2id Bearer) |
| `internal/transport/http/response` | Stable JSON error envelope |
| `internal/transport/http` | Router, health, decision, policy, key handlers |
| `internal/repository/postgres` | policy_repo, credential_repo, assignment_repo, audit_repo |
| `internal/repository/redis` | token_bucket adapter, sliding_window adapter (EVALSHA + EVAL fallback) |
| `scripts/redis` | `token_bucket.lua`, `sliding_window.lua` — atomic, server-time Redis scripts |
| `internal/service` | credential_service (Argon2id), policy_service (cache), decision_service |
| `internal/app` | Postgres pool, Redis client, goose migration runner, ping helpers |
| `cmd/api/main.go` | Wiring, graceful shutdown on SIGINT/SIGTERM |
| `tests/integration` | Token-bucket (basic + 1000-concurrent), sliding-window, revoked-key tests |
| `load/k6/scenarios.js` | Same-key contention + many-key throughput load scenarios |

### Key design decisions made in Phase 0

1. **Redis Lua atomicity** — all quota mutations in Lua; no check-then-update in Go.
2. **Server-time only** — scripts call `redis.call('TIME')`, never trust caller's clock.
3. **Fail closed** — Redis errors → 503 (dependency_unavailable); no silent bypass.
4. **Argon2id + per-key salt** — format `argon2id$<salt_b64>$<hash_b64>`; prefix stored for support.
5. **Policy cache** — 30-second in-process RWMutex cache; invalidated on every write.
6. **IETF rate-limit headers** — `RateLimit-Limit`, `RateLimit-Remaining`, `RateLimit-Reset` on every decision.

---

## Known TODOs / decisions deferred

- [ ] Bootstrap endpoint: Phase 0 seeds a default tenant but has no admin user seed — first run needs a direct DB insert to get the first API key. Consider adding a `make seed` target in Phase 1.
- [ ] Assignment CRUD endpoint: POST /v1/assignments is referenced in tests but not yet wired in the router. Wire in Phase 1.
- [ ] Testcontainers wiring in `integration_test.go` has a `t.Skip` placeholder — full wiring in Phase 2.

---

## Running the project

```powershell
cd "d:\Backend Projects\Project 1\quotaforge"

# 1. Start Postgres + Redis
docker compose up -d postgres redis

# 2. Build and run API (after Go is installed)
go run ./cmd/api

# 3. Health check
curl http://localhost:8080/health/live
curl http://localhost:8080/health/ready

# 4. Full stack (docker-builds API too)
docker compose up -d --build
```
