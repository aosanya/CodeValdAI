module github.com/aosanya/CodeValdAI

go 1.25.3

replace github.com/aosanya/CodeValdSharedLib => ../CodeValdSharedLib

require (
	github.com/aosanya/CodeValdSharedLib v0.0.0-20260324114722-2ab98458026d
	github.com/arangodb/go-driver v1.6.0
	google.golang.org/grpc v1.79.1
	google.golang.org/protobuf v1.36.11
)

require (
	github.com/arangodb/go-velocypack v0.0.0-20200318135517-5af53c29c67e // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	golang.org/x/net v0.48.0 // indirect
	golang.org/x/sys v0.39.0 // indirect
	golang.org/x/text v0.32.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20251202230838-ff82c1b0f217 // indirect
)
