// Command server starts the CodeValdAI gRPC microservice.
//
// Configuration is via environment variables:
//
//	CODEVALDAI_GRPC_PORT         gRPC listener port (required)
//	CROSS_GRPC_ADDR              CodeValdCross gRPC address for service
//	                             registration heartbeats and event publishing
//	                             (optional; omit to disable)
//	AI_GRPC_ADVERTISE_ADDR       address CodeValdCross dials back (default ":PORT")
//	CODEVALDAI_AGENCY_ID         agency ID sent in every Register heartbeat
//	                             (required when CROSS_GRPC_ADDR is set)
//	CROSS_PING_INTERVAL          heartbeat cadence (default "20s")
//	CROSS_PING_TIMEOUT           per-RPC timeout for each Register call (default "5s")
//
// ArangoDB backend:
//
//	AI_ARANGO_ENDPOINT           ArangoDB endpoint URL (default "http://localhost:8529")
//	AI_ARANGO_USER               ArangoDB username (default "root")
//	AI_ARANGO_PASSWORD           ArangoDB password
//	AI_ARANGO_DATABASE           ArangoDB database name (default "codevaldai")
package main

import (
	"context"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	codevaldai "github.com/aosanya/CodeValdAI"
	pb "github.com/aosanya/CodeValdAI/gen/go/codevaldai/v1"
	"github.com/aosanya/CodeValdAI/internal/config"
	"github.com/aosanya/CodeValdAI/internal/registrar"
	"github.com/aosanya/CodeValdAI/internal/server"
	arangodb "github.com/aosanya/CodeValdAI/storage/arangodb"
	"github.com/aosanya/CodeValdSharedLib/entitygraph"
	healthpb "github.com/aosanya/CodeValdSharedLib/gen/go/codevaldhealth/v1"
	entitygraphpb "github.com/aosanya/CodeValdSharedLib/gen/go/entitygraph/v1"
	"github.com/aosanya/CodeValdSharedLib/health"
	"github.com/aosanya/CodeValdSharedLib/serverutil"
)

func main() {
	cfg := config.Load()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var pub codevaldai.CrossPublisher
	if cfg.CrossGRPCAddr != "" {
		reg, err := registrar.New(
			cfg.CrossGRPCAddr,
			cfg.AdvertiseAddr,
			cfg.AgencyID,
			cfg.PingInterval,
			cfg.PingTimeout,
		)
		if err != nil {
			log.Printf("codevaldai: registrar: failed to create: %v — continuing without registration", err)
		} else {
			defer reg.Close()
			go reg.Run(ctx)
			pub = reg
		}
	} else {
		log.Println("codevaldai: CROSS_GRPC_ADDR not set — skipping CodeValdCross registration")
	}

	// Connect to ArangoDB and construct the DataManager + SchemaManager.
	backend, err := arangodb.NewBackend(arangodb.Config{
		Endpoint: cfg.ArangoEndpoint,
		Username: cfg.ArangoUser,
		Password: cfg.ArangoPassword,
		Database: cfg.ArangoDatabase,
		Schema:   codevaldai.DefaultAISchema(),
	})
	if err != nil {
		log.Fatalf("codevaldai: ArangoDB backend: %v", err)
	}

	// Seed the pre-delivered schema idempotently on startup.
	if cfg.AgencyID != "" {
		seedCtx, seedCancel := context.WithTimeout(ctx, 10*time.Second)
		if err := entitygraph.SeedSchema(seedCtx, backend, cfg.AgencyID, codevaldai.DefaultAISchema()); err != nil {
			log.Printf("codevaldai: schema seed: %v", err)
		}
		seedCancel()
	} else {
		log.Println("codevaldai: CODEVALDAI_AGENCY_ID not set — skipping schema seed")
	}

	mgr := codevaldai.NewAIManager(backend, backend, pub, cfg.AgencyID)

	lis, err := net.Listen("tcp", ":"+cfg.GRPCPort)
	if err != nil {
		log.Fatalf("codevaldai: failed to listen on :%s: %v", cfg.GRPCPort, err)
	}

	grpcServer, _ := serverutil.NewGRPCServer()
	pb.RegisterAIServiceServer(grpcServer, server.New(mgr))
	entitygraphpb.RegisterEntityServiceServer(grpcServer, server.NewEntityServer(backend))
	healthpb.RegisterHealthServiceServer(grpcServer, health.New("codevaldai"))

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		<-quit
		log.Println("codevaldai: shutdown signal received")
		cancel()
	}()

	log.Printf("CodeValdAI gRPC server listening on :%s", cfg.GRPCPort)
	serverutil.RunWithGracefulShutdown(ctx, grpcServer, lis, 30*time.Second)
}
