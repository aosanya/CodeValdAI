# CodeValdAI

LLM provider catalogue, agent definitions, and two-phase intake/execute run
lifecycle for CodeValdCortex. All entities are persisted in the agency-scoped
graph via [CodeValdSharedLib/entitygraph](../CodeValdSharedLib/entitygraph).

## Layout

- `ai.go`, `models.go`, `errors.go`, `schema.go` — public API surface.
- `cmd/server`, `cmd/dev` — slim shims that delegate to `internal/app.Run`.
- `internal/app`, `internal/config` — bootstrap wiring; configuration loaded
  from env vars (see `internal/config/config.go`).
- `internal/registrar` — Cross heartbeat + `CrossPublisher` for
  `cross.ai.{agencyID}.agent.created` / `run.completed` / `run.failed`.
- `internal/server` — `AIService` gRPC handler + re-export of the shared
  `EntityServer`.
- `storage/arangodb` — thin shim over
  [`CodeValdSharedLib/entitygraph/arangodb`](../CodeValdSharedLib/entitygraph/arangodb)
  fixing the `ai_*` collection / graph names.

## Local dev

```sh
make build         # compile everything
make test          # unit tests
make test-arango   # integration tests (requires ArangoDB; reads .env)
make dev           # build + run with .env loaded
```
