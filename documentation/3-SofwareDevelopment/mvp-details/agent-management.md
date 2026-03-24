````markdown
# Agent Management — Implementation Details

---

## MVP-AI-008 — ArangoDB Backend

**Status**: 🔲 Not Started
**Branch**: `feature/AI-008_arangodb_backend`

### Goal

Implement the ArangoDB `entitygraph.DataManager` + `entitygraph.SchemaManager`
for CodeValdAI. Follows the same three-file split as CodeValdAgency:
`storage.go`, `docs.go`, `ops.go`.

### Files

| File | Purpose |
|---|---|
| `storage/arangodb/storage.go` | `Config` struct, `Backend` constructor, `ensureCollections` (creates `ai_entities`, `ai_relationships` as edge, `ai_schemas`) |
| `storage/arangodb/docs.go` | `aiEntityDoc` ArangoDB document type; `toEntityDoc` / `fromEntityDoc` converters |
| `storage/arangodb/ops.go` | Implementation of `entitygraph.DataManager` and `entitygraph.SchemaManager` methods |

### Collections Ensured on Startup

| Collection | Type |
|---|---|
| `ai_entities` | Document |
| `ai_relationships` | **Edge** (must use `arangodb.CollectionTypeEdge`) |
| `ai_schemas` | Document |

### Config

```go
type Config struct {
    URL      string // e.g. "http://localhost:8529"
    Database string // e.g. "codevaldai"
    Username string
    Password string
}
```

### New Function

```go
// New returns a DataManager and SchemaManager backed by ArangoDB.
// It ensures all required collections exist before returning.
func New(ctx context.Context, cfg Config) (entitygraph.DataManager, entitygraph.SchemaManager, error)
```

### Indexes to Create

See [architecture-storage.md](../../2-SoftwareDesignAndArchitecture/architecture-storage.md) §3 for the full index list.

### Acceptance Tests

- `New` with unreachable DB returns a connection error
- `ensureCollections` is idempotent — calling it twice does not error
- `ai_relationships` is created as an edge collection (not document)
- `SetSchema` followed by `GetSchema` returns the same schema

---

## MVP-AI-009 — gRPC Proto

**Status**: 🔲 Not Started
**Branch**: `feature/AI-009_grpc_proto`

### Goal

Write `proto/codevaldai/v1/ai.proto` and run `buf generate` to produce
Go stubs in `gen/go/`.

### File: `proto/codevaldai/v1/ai.proto`

```protobuf
syntax = "proto3";
package codevaldai.v1;
option go_package = "github.com/aosanya/CodeValdAI/gen/go/codevaldai/v1;codevaldaiv1";

import "google/protobuf/empty.proto";

service AIService {
    rpc CreateAgent(CreateAgentRequest)  returns (Agent);
    rpc GetAgent(GetAgentRequest)        returns (Agent);
    rpc ListAgents(ListAgentsRequest)    returns (ListAgentsResponse);
    rpc DeleteAgent(DeleteAgentRequest)  returns (google.protobuf.Empty);

    rpc IntakeRun(IntakeRunRequest)   returns (IntakeRunResponse);
    rpc ExecuteRun(ExecuteRunRequest) returns (AgentRun);
    rpc GetRun(GetRunRequest)         returns (AgentRun);
    rpc ListRuns(ListRunsRequest)     returns (ListRunsResponse);
}

message Agent {
    string id            = 1;
    string name          = 2;
    string description   = 3;
    string provider      = 4;
    string model         = 5;
    string system_prompt = 6;
    double temperature   = 7;
    int32  max_tokens    = 8;
    string created_at    = 9;
    string updated_at    = 10;
}

message AgentRun {
    string id             = 1;
    string agent_id       = 2;
    string workflow_id    = 3;
    string instructions   = 4;
    string status         = 5;
    string output         = 6;
    string error_message  = 7;
    int32  input_tokens   = 8;
    int32  output_tokens  = 9;
    string started_at     = 10;
    string completed_at   = 11;
    string created_at     = 12;
    string updated_at     = 13;
}

message RunField {
    string id         = 1;
    string fieldname  = 2;
    string type       = 3;
    string label      = 4;
    bool   required   = 5;
    repeated string options = 6;
    int32  ordinality = 7;
}

message RunInput {
    string fieldname = 1;
    string value     = 2;
}

message CreateAgentRequest  { Agent agent = 1; }
message GetAgentRequest     { string agent_id = 1; }
message ListAgentsRequest   {}
message ListAgentsResponse  { repeated Agent agents = 1; }
message DeleteAgentRequest  { string agent_id = 1; }

message IntakeRunRequest    { string agent_id = 1; string workflow_id = 2; string instructions = 3; }
message IntakeRunResponse   { string run_id = 1; repeated RunField fields = 2; }
message ExecuteRunRequest   { string run_id = 1; repeated RunInput inputs = 2; }
message GetRunRequest       { string run_id = 1; }
message ListRunsRequest     { string agent_id = 1; string status = 2; }
message ListRunsResponse    { repeated AgentRun runs = 1; }
```

### Acceptance Tests

- `buf lint` reports 0 issues
- `buf generate` produces `gen/go/codevaldai/v1/ai.pb.go` and `ai_grpc.pb.go`
- `go build ./gen/...` succeeds

---

## MVP-AI-010 — gRPC Server

**Status**: 🔲 Not Started
**Branch**: `feature/AI-010_grpc_server`

### Goal

Implement thin gRPC handler wrappers in `internal/server/`. Handlers
delegate to `AIManager` — zero business logic inside them.

### Files

| File | Purpose |
|---|---|
| `internal/server/server.go` | `Server` struct holding `AIManager`; `NewServer` constructor; `AIService` gRPC handler implementations |
| `internal/server/entity_server.go` | `EntityService` gRPC handlers delegating to `entitygraph.DataManager` (for generic entity access) |
| `internal/server/errors.go` | `toGRPCError(err error) error` — maps domain errors to gRPC status codes |

### Handler Pattern

```go
func (s *Server) IntakeRun(ctx context.Context, req *pb.IntakeRunRequest) (*pb.IntakeRunResponse, error) {
    run, fields, err := s.manager.IntakeRun(ctx, codevaldai.IntakeRunRequest{
        AgentID:      req.AgentId,
        WorkflowID:   req.WorkflowId,
        Instructions: req.Instructions,
    })
    if err != nil {
        return nil, toGRPCError(err)
    }
    return toIntakeRunResponse(run, fields), nil
}
```

### Acceptance Tests

- Each handler correctly maps request proto → domain type → `AIManager` call
- `toGRPCError` maps each exported error to the correct gRPC status code
- `NewServer(nil)` returns an error

---

## MVP-AI-011 — Config & Registrar

**Status**: 🔲 Not Started
**Branch**: `feature/AI-011_config_registrar`

### Goal

Implement configuration loading and the CodeValdCross heartbeat registrar.

### Files

| File | Purpose |
|---|---|
| `internal/config/config.go` | `Config` struct with all service settings; `Load()` reads env vars |
| `internal/registrar/registrar.go` | Cross heartbeat loop using `registrar.New` from CodeValdSharedLib |

### Config Fields

```go
type Config struct {
    GRPCPort       string // CODEVALDAI_GRPC_PORT (default ":50056")
    ArangoURL      string // CODEVALDAI_ARANGO_URL
    ArangoDB       string // CODEVALDAI_ARANGO_DB
    ArangoUser     string // CODEVALDAI_ARANGO_USER
    ArangoPassword string // CODEVALDAI_ARANGO_PASSWORD
    CrossAddr      string // CODEVALDAI_CROSS_ADDR
    AgencyID       string // CODEVALDAI_AGENCY_ID
    AnthropicKey   string // ANTHROPIC_API_KEY
}
```

### Registrar Topics

```go
Produces: []string{
    fmt.Sprintf("cross.ai.%s.run.completed",  agencyID),
    fmt.Sprintf("cross.ai.%s.run.failed",     agencyID),
    fmt.Sprintf("cross.ai.%s.agent.created",  agencyID),
},
Consumes: []string{
    "cross.agency.created",
    "work.task.dispatched",
},
```

### HTTP Routes registered with Cross

All 8 HTTP routes from [architecture-flows.md](../../2-SoftwareDesignAndArchitecture/architecture-flows.md) §7,
using `schemaroutes.RoutesFromSchema` from CodeValdSharedLib.

### Acceptance Tests

- `Load()` returns defaults for unset optional vars
- Missing `CODEVALDAI_AGENCY_ID` returns an error
- Missing `ANTHROPIC_API_KEY` returns an error

---

## MVP-AI-012 — cmd/main.go Wiring

**Status**: 🔲 Not Started
**Branch**: `feature/AI-012_cmd_wiring`

### Goal

Wire all dependencies in `cmd/main.go`. No business logic here.

### Startup Sequence

```
1. config.Load()
2. arangodb.New(ctx, cfg) → (DataManager, SchemaManager)
3. sm.SetSchema(ctx, DefaultAISchema())  ← idempotent schema seed
4. anthropic.New(cfg.AnthropicKey, "")   → LLMClient
5. registrar.NewPublisher(cfg.CrossAddr) → Publisher
6. codevaldai.NewAIManager(dm, sm, llm, publisher, cfg.AgencyID)
7. server.NewServer(manager)
8. serverutil.NewGRPCServer() → register AIService + EntityService + health
9. registrar.New(...)  → start heartbeat in goroutine
10. serverutil.RunWithGracefulShutdown(ctx, grpcSrv, lis, 5*time.Second)
```

### Acceptance Tests

- Service starts, registers with Cross, and listens on configured port
- `DefaultAISchema()` seeds without error on a clean database
- Graceful shutdown completes within 5 s when SIGTERM received
````
