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
	"github.com/aosanya/CodeValdAI/internal/recovery"
	"github.com/aosanya/CodeValdAI/internal/registrar"
	"github.com/aosanya/CodeValdAI/internal/server"
	aiarangodb "github.com/aosanya/CodeValdAI/storage/arangodb"
	agencypb "github.com/aosanya/CodeValdAgency/gen/go/codevaldagency/v1"
	"github.com/aosanya/CodeValdSharedLib/entitygraph"
	healthpb "github.com/aosanya/CodeValdSharedLib/gen/go/codevaldhealth/v1"
	sharedev1 "github.com/aosanya/CodeValdSharedLib/gen/go/codevaldshared/v1"
	entitygraphpb "github.com/aosanya/CodeValdSharedLib/gen/go/entitygraph/v1"
	"github.com/aosanya/CodeValdSharedLib/health"
	"github.com/aosanya/CodeValdSharedLib/serverutil"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// Run starts all CodeValdAI subsystems (Cross registrar, ArangoDB
// entitygraph backend, gRPC server) and blocks until SIGINT/SIGTERM triggers
// graceful shutdown.
func Run(cfg config.Config) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// ── Agency gRPC client (shared: subscription sync + dispatcher) ──────────
	var agencyClient agencypb.AgencyServiceClient
	if cfg.AgencyGRPCAddr != "" {
		agencyConn, err := grpc.NewClient(cfg.AgencyGRPCAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			log.Printf("codevaldai: agency client: %v — dispatch and subscription sync disabled", err)
		} else {
			defer agencyConn.Close()
			agencyClient = agencypb.NewAgencyServiceClient(agencyConn)
		}
	} else {
		log.Println("codevaldai: AGENCY_GRPC_ADDR not set — dispatch and subscription sync disabled")
	}

	// ── Subscribe topics derived from agency work plans ───────────────────────
	subscribeTopics := workPlanTopics(ctx, agencyClient)

	// ── Cross registrar (optional) ───────────────────────────────────────────
	var pub codevaldai.CrossPublisher
	if cfg.CrossGRPCAddr != "" {
		reg, err := registrar.New(
			cfg.CrossGRPCAddr,
			cfg.AdvertiseAddr,
			cfg.AgencyID,
			cfg.PingInterval,
			cfg.PingTimeout,
			subscribeTopics,
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

	// ── Boot sweep — fail any AgentRun left in "running" by a prior crash ───
	if cfg.AgencyID != "" {
		sweepCtx, sweepCancel := context.WithTimeout(ctx, 30*time.Second)
		if err := recovery.ReconcileRunningRuns(sweepCtx, backend, pub, cfg.AgencyID, nil); err != nil {
			log.Printf("codevaldai: reconcile running runs: %v", err)
			// continue — reconcile failure must not block startup
		}
		sweepCancel()
	}

	// ── AIManager ────────────────────────────────────────────────────────────
	mgr := codevaldai.NewAIManager(backend, backend, pub, cfg.AgencyID)

	// ── RACI dispatcher (reuses agency client opened above) ──────────────────
	var dispatcher server.EventDispatcher
	if agencyClient != nil {
		dispatcher = server.NewRACIDispatcher(agencyClient, mgr, cfg.AgencyID)
	}

	// ── gRPC server ───────────────────────────────────────────────────────────
	lis, err := net.Listen("tcp", ":"+cfg.GRPCPort)
	if err != nil {
		return fmt.Errorf("listen on :%s: %w", cfg.GRPCPort, err)
	}

	grpcServer, _ := serverutil.NewGRPCServer()
	pb.RegisterAIServiceServer(grpcServer, server.New(mgr))
	entitygraphpb.RegisterEntityServiceServer(grpcServer, server.NewEntityServer(backend))
	sharedev1.RegisterEventReceiverServiceServer(grpcServer, server.NewEventReceiver(backend, cfg.AgencyID, dispatcher))
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

// workPlanTopics fetches all enabled work plans from the Agency service and
// returns the deduplicated trigger_topic values for plans whose handler_service
// is "codevaldai". Best-effort: on error it logs and returns nil so startup is
// never blocked.
func workPlanTopics(ctx context.Context, client agencypb.AgencyServiceClient) []string {
	if client == nil {
		return nil
	}
	fetchCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	resp, err := client.ListWorkPlans(fetchCtx, &agencypb.ListWorkPlansRequest{})
	if err != nil {
		log.Printf("codevaldai: workPlanTopics: ListWorkPlans: %v — no topics subscribed", err)
		return nil
	}

	seen := map[string]bool{}
	var out []string
	for _, wp := range resp.GetWorkPlans() {
		if wp.GetEnabled() && wp.GetHandlerService() == "codevaldai" && wp.GetTriggerTopic() != "" {
			if !seen[wp.GetTriggerTopic()] {
				seen[wp.GetTriggerTopic()] = true
				out = append(out, wp.GetTriggerTopic())
			}
		}
	}
	log.Printf("codevaldai: workPlanTopics: subscribing to %d topic(s): %v", len(out), out)
	return out
}
