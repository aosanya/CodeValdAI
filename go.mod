module github.com/aosanya/CodeValdAI

go 1.25.3

replace github.com/aosanya/CodeValdSharedLib => ../CodeValdSharedLib

require (
	github.com/aosanya/CodeValdSharedLib v0.0.0-20260324114722-2ab98458026d
	github.com/arangodb/go-driver v1.6.0
)

require (
	github.com/arangodb/go-velocypack v0.0.0-20200318135517-5af53c29c67e // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/pkg/errors v0.9.1 // indirect
)
