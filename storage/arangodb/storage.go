// Package arangodb implements the ArangoDB backend for CodeValdAI.
// All implementation logic lives in
// [github.com/aosanya/CodeValdSharedLib/entitygraph/arangodb]; this package
// is a thin service-scoped adapter that fixes the collection and graph names
// to their AI-specific values (ai_entities, ai_relationships,
// ai_schemas_draft, ai_schemas_published, ai_graph).
//
// Use [New] to obtain a (DataManager, SchemaManager) pair from an open database.
// Use [NewBackend] to connect and construct in a single call.
// Use [NewBackendFromDB] in tests that manage their own database lifecycle.
package arangodb

import (
	"fmt"

	driver "github.com/arangodb/go-driver"

	"github.com/aosanya/CodeValdSharedLib/entitygraph"
	sharedadb "github.com/aosanya/CodeValdSharedLib/entitygraph/arangodb"
	"github.com/aosanya/CodeValdSharedLib/types"
)

// Backend is a type alias for the shared ArangoDB Backend.
// Callers holding *Backend references continue to compile unchanged.
type Backend = sharedadb.Backend

// Config is the connection parameters for the AI ArangoDB backend.
// It is an alias of [sharedadb.ConnConfig]; see that type for field docs.
// NewBackend defaults Database to "codevaldai" when empty.
type Config = sharedadb.ConnConfig

// toSharedConfig expands an AI Config into a full SharedLib Config,
// filling in the fixed AI-specific collection and graph names.
// Returns an error if Database is empty — callers must always supply it explicitly.
func toSharedConfig(cfg Config) (sharedadb.Config, error) {
	if cfg.Database == "" {
		return sharedadb.Config{}, fmt.Errorf("arangodb: Database must be set (e.g. \"codevaldai\")")
	}
	return sharedadb.Config{
		Endpoint:            cfg.Endpoint,
		Username:            cfg.Username,
		Password:            cfg.Password,
		Database:            cfg.Database,
		Schema:              cfg.Schema,
		EntityCollection:    "ai_entities",
		RelCollection:       "ai_relationships",
		SchemasDraftCol:     "ai_schemas_draft",
		SchemasPublishedCol: "ai_schemas_published",
		GraphName:           "ai_graph",
	}, nil
}

// New constructs a Backend from an already-open driver.Database using the
// provided schema, ensures all collections and the named graph exist, and
// returns the Backend as both a DataManager and a SchemaManager.
func New(db driver.Database, schema types.Schema) (entitygraph.DataManager, entitygraph.SchemaManager, error) {
	scfg, err := toSharedConfig(Config{Schema: schema})
	if err != nil {
		return nil, nil, err
	}
	return sharedadb.New(db, scfg)
}

// NewBackend connects to ArangoDB using cfg, ensures all collections exist,
// and returns a ready-to-use Backend.
func NewBackend(cfg Config) (*Backend, error) {
	scfg, err := toSharedConfig(cfg)
	if err != nil {
		return nil, err
	}
	return sharedadb.NewBackend(scfg)
}

// NewBackendFromDB constructs a Backend from an already-open driver.Database
// using the provided schema. Intended for tests that manage their own database
// lifecycle.
func NewBackendFromDB(db driver.Database, schema types.Schema) (*Backend, error) {
	if db == nil {
		return nil, fmt.Errorf("arangodb: NewBackendFromDB: database must not be nil")
	}
	scfg, err := toSharedConfig(Config{Schema: schema})
	if err != nil {
		return nil, err
	}
	return sharedadb.NewBackendFromDB(db, scfg)
}
