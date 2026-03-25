// Package server implements the AIService and EntityService gRPC handlers.
// EntityServer is provided by CodeValdSharedLib/entitygraph/server — this file
// re-exports NewEntityServer so callers import only this package.
package server

import (
	egserver "github.com/aosanya/CodeValdSharedLib/entitygraph/server"
)

// NewEntityServer constructs an EntityServer backed by the given DataManager.
// It is a thin re-export of the shared implementation in
// CodeValdSharedLib/entitygraph/server.
var NewEntityServer = egserver.NewEntityServer
