---
agent: agent
---

# Refactor Go Code

Guides a safe, incremental Go refactoring for **CodeValdAI**.

---

## When to Refactor

- File exceeds **500 lines** (hard limit)
- Function exceeds **50 lines**
- Multiple concerns in one file (e.g. one manager file doing provider + agent + run logic)
- Duplicated logic across run lifecycle transitions (intake / execute / status update)
- LLM SDK imported into domain code (must move to the `LLMClient` impl)
- ArangoDB driver / raw AQL leaking outside `storage/arangodb/`
- Business logic leaked into `cmd/main.go` or a gRPC handler in `internal/server/`

---

## Refactoring Workflow

### Step 1: Understand the File

```bash
wc -l ai.go models.go errors.go schema.go
grep -n "^func " ai.go
```

### Step 2: Plan the Split

Identify distinct responsibilities. For CodeValdAI, typical splits:

```
Top-level package (codevaldai):
├── ai.go        # AIManager interface only
├── doc.go       # Package godoc
├── errors.go    # Sentinel errors
├── models.go    # Value types and request/filter structs
└── schema.go    # DefaultAISchema

Manager implementation (when it grows past 500 lines, split by concern):
internal/manager/
├── manager.go   # aiManager struct + constructor
├── provider.go  # CreateProvider / Get / List / Update / Delete
├── agent.go     # CreateAgent / Get / List / Update / Delete
└── run.go       # IntakeRun / ExecuteRun / GetRun / ListRuns

Server (gRPC handlers — translate proto ↔ domain only):
internal/server/
├── server.go         # AIService handler
├── entity_server.go  # EntityService passthrough
└── errors.go         # Domain → gRPC status mapping
```

### Step 3: Extract — One File at a Time

1. Create the new file with its package declaration
2. Move types / functions
3. Update imports
4. Run `go build ./...` — must succeed after each file move
5. Run `go test -v -race ./...`

### Step 4: Handle Shared Dependencies

If a value type is used across multiple files, keep it in `models.go` at the
module root. If a sentinel error is referenced by multiple packages, keep it in
`errors.go` at module root.

If a helper is needed by both the manager implementation and the server,
consider whether it belongs in the top-level `codevaldai` package (exported) or
in a new `internal/` helper package (unexported).

### Step 5: Validate

```bash
go build ./...           # must succeed
go vet ./...             # must show 0 issues
go test -v -race ./...   # must pass
golangci-lint run ./...  # must pass
```

---

## CodeValdAI-Specific Anti-Patterns to Fix During Refactor

- ❌ **Direct Anthropic / OpenAI SDK call in `AIManager`** → extract `LLMClient` interface; SDK only in the concrete impl
- ❌ **`grpc.Dial` inside a manager method** → inject `CrossPublisher`
- ❌ **Raw AQL or ArangoDB driver call outside `storage/arangodb/`** → use `entitygraph.DataManager`
- ❌ **Raw `AgentRunStatus` string literals** → replace with constants from `models.go`
- ❌ **Business logic in gRPC handler** → move to manager implementation; handler only translates proto ↔ domain
- ❌ **Cross publish error treated as fatal** → log and continue
- ❌ **Manager method without `context.Context` first arg** → add it, propagate to LLM and storage calls
- ❌ **API key or full LLM payload in logs** → redact; log run/agent IDs and token counts only

---

## Checklist

**Before refactoring:**
- [ ] File exceeds limit or has multiple concerns
- [ ] Identified distinct responsibilities (provider / agent / run / lifecycle)
- [ ] Planned new package structure

**During refactoring:**
- [ ] One file moved at a time with `go build` between each
- [ ] No circular dependencies introduced
- [ ] Interface injection preserved (`LLMClient`, `CrossPublisher`, `DataManager`)

**After refactoring:**
- [ ] All tests pass (unit + ArangoDB integration if reachable)
- [ ] No circular dependencies (`go build ./...`)
- [ ] Files within size limits
- [ ] No breaking changes to the public `AIManager` API (or documented)
