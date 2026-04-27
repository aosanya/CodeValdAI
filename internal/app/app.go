// Package app holds the shared runtime wiring for CodeValdAI. Both the
// production binary (cmd/server) and the local dev binary (cmd/dev) call
// Run; they differ only in which environment variables they set before
// loading config.
package app

import (
	"context"
	"fmt"
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
	aiarangodb "github.com/aosanya/CodeValdAI/storage/arangodb"
	"github.com/aosanya/CodeValdSharedLib/entitygraph"
	healthpb "github.com/aosanya/CodeValdSharedLib/gen/go/codevaldhealth/v1"
	entitygraphpb "github.com/aosanya/CodeValdSharedLib/gen/go/entitygraph/v1"
	"github.com/aosanya/CodeValdSharedLib/health"
	"github.com/aosanya/CodeValdSharedLib/serverutil"
)

// Run starts all CodeValdAI subsystems (Cross registrar, ArangoDB
// entitygraph backend, gRPC server) and blocks until SIGINT/SIGTERM triggers
// graceful shutdown.
func Run(cfg config.Config) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// ── Cross registrar (optional) ───────────────────────────────────────────
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

	// ── ArangoDB entitygraph backend ─────────────────────────────────────────
	backend, err := aiarangodb.NewBackend(aiarangodb.Config{
		Endpoint: cfg.ArangoEndpoint,
		Username: cfg.ArangoUser,
		Password: cfg.ArangoPassword,
		Database: cfg.ArangoDatabase,
		Schema:   codevaldai.DefaultAISchema(),
	})
	if err != nil {
		return fmt.Errorf("ArangoDB backend: %w", err)
	}

	// ── Schema seed (idempotent on startup) ──────────────────────────────────
	if cfg.AgencyID != "" {
		seedCtx, seedCancel := context.WithTimeout(ctx, 10*time.Second)
		if err := entitygraph.SeedSchema(seedCtx, backend, cfg.AgencyID, codevaldai.DefaultAISchema()); err != nil {
			log.Printf("codevaldai: schema seed: %v", err)
		}
		seedCancel()
	} else {
		log.Println("codevaldai: CODEVALDAI_AGENCY_ID not set — skipping schema seed")
	}

	// ── AIManager ────────────────────────────────────────────────────────────
	mgr := codevaldai.NewAIManager(backend, backend, pub, cfg.AgencyID)

	// ── gRPC server ───────────────────────────────────────────────────────────
	lis, err := net.Listen("tcp", ":"+cfg.GRPCPort)
	if err != nil {
		return fmt.Errorf("listen on :%s: %w", cfg.GRPCPort, err)
	}

	grpcServer, _ := serverutil.NewGRPCServer()
	pb.RegisterAIServiceServer(grpcServer, server.New(mgr))
	entitygraphpb.RegisterEntityServiceServer(grpcServer, server.NewEntityServer(backend))
	healthpb.RegisterHealthServiceServer(grpcServer, health.New("codevaldai"))

	// ── Signal handling ───────────────────────────────────────────────────────
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		<-quit
		log.Println("codevaldai: shutdown signal received")
		cancel()
	}()

	log.Printf("codevaldai: gRPC server listening on :%s", cfg.GRPCPort)
	serverutil.RunWithGracefulShutdown(ctx, grpcServer, lis, 30*time.Second)
	return nil
}
