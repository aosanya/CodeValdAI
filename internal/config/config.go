// Package config loads CodeValdAI runtime configuration from environment variables.
package config

import (
	"time"

	"github.com/aosanya/CodeValdSharedLib/serverutil"
)

// Config holds all runtime configuration for the CodeValdAI service.
type Config struct {
	// GRPCPort is the port the gRPC server listens on (required).
	GRPCPort string

	// ArangoEndpoint is the ArangoDB HTTP endpoint (default "http://localhost:8529").
	ArangoEndpoint string

	// ArangoUser is the ArangoDB username (default "root").
	ArangoUser string

	// ArangoPassword is the ArangoDB password.
	ArangoPassword string

	// ArangoDatabase is the ArangoDB database name (default "codevaldai").
	ArangoDatabase string

	// PubSubGRPCAddr is the CodeValdPubSub gRPC address for event subscriptions.
	// Empty string disables the subscriber.
	PubSubGRPCAddr string

	// CrossGRPCAddr is the CodeValdCross gRPC address for registration heartbeats.
	// Empty string disables registration.
	CrossGRPCAddr string

	// AdvertiseAddr is the address CodeValdCross dials back on (default ":GRPCPort").
	AdvertiseAddr string

	// AgencyID is the agency ID sent in every Register heartbeat to CodeValdCross.
	AgencyID string

	// PingInterval is the heartbeat cadence sent to CodeValdCross (default 20s).
	PingInterval time.Duration

	// PingTimeout is the per-RPC timeout for each Register call (default 5s).
	PingTimeout time.Duration
}

// Load reads configuration from environment variables, falling back to defaults
// for any variable that is unset or empty.
func Load() Config {
	port := serverutil.MustGetEnv("CODEVALDAI_GRPC_PORT")
	return Config{
		GRPCPort:       port,
		ArangoEndpoint: serverutil.EnvOrDefault("AI_ARANGO_ENDPOINT", "http://localhost:8529"),
		ArangoUser:     serverutil.EnvOrDefault("AI_ARANGO_USER", "root"),
		ArangoPassword: serverutil.EnvOrDefault("AI_ARANGO_PASSWORD", ""),
		ArangoDatabase: serverutil.EnvOrDefault("AI_ARANGO_DATABASE", "codevaldai"),
		PubSubGRPCAddr: serverutil.EnvOrDefault("PUBSUB_GRPC_ADDR", ""),
		CrossGRPCAddr:  serverutil.EnvOrDefault("CROSS_GRPC_ADDR", ""),
		AdvertiseAddr:  serverutil.EnvOrDefault("AI_GRPC_ADVERTISE_ADDR", ":"+port),
		AgencyID:       serverutil.EnvOrDefault("CODEVALDAI_AGENCY_ID", ""),
		PingInterval:   serverutil.ParseDurationString("CROSS_PING_INTERVAL", 20*time.Second),
		PingTimeout:    serverutil.ParseDurationString("CROSS_PING_TIMEOUT", 5*time.Second),
	}
}
