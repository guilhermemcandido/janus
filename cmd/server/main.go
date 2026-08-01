package main

import (
	"context"
	"flag"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/keepalive"

	"github.com/guilhermemcandido/janus/internal/api"
	pb "github.com/guilhermemcandido/janus/internal/api/proto"
	"github.com/guilhermemcandido/janus/internal/engine"
)

// shutdownGracePeriod bounds GracefulStop, which otherwise waits forever on a long-lived stream like SubscribeTrades.
const shutdownGracePeriod = 5 * time.Second

func main() {
	addr := flag.String("addr", ":50051", "listen address")
	flag.Parse()

	lis, err := net.Listen("tcp", *addr)
	if err != nil {
		log.Fatalf("listen on %s: %v", *addr, err)
	}

	exchange := engine.NewExchange()
	defer exchange.Close()

	// MinTime must stay below the client's keepalive Time (pkg/client.Dial, 5s), or the server GOAWAYs it for pinging too often.
	grpcServer := grpc.NewServer(grpc.KeepaliveEnforcementPolicy(keepalive.EnforcementPolicy{
		MinTime:             4 * time.Second,
		PermitWithoutStream: true,
	}))
	pb.RegisterExchangeServer(grpcServer, api.NewServer(exchange))

	healthServer := health.NewServer()
	healthServer.SetServingStatus("", healthpb.HealthCheckResponse_SERVING)
	healthpb.RegisterHealthServer(grpcServer, healthServer)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		<-ctx.Done()
		log.Println("shutting down...")
		stopped := make(chan struct{})
		go func() {
			grpcServer.GracefulStop()
			close(stopped)
		}()
		select {
		case <-stopped:
		case <-time.After(shutdownGracePeriod):
			log.Println("graceful stop timed out, forcing shutdown")
			grpcServer.Stop()
		}
	}()

	log.Printf("janus gRPC server listening on %s", *addr)
	if err := grpcServer.Serve(lis); err != nil {
		log.Fatalf("serve: %v", err)
	}
}
