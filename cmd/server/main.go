package main

import (
	"flag"
	"log"
	"net"

	"google.golang.org/grpc"

	"github.com/guilhermemcandido/janus/internal/api"
	pb "github.com/guilhermemcandido/janus/internal/api/proto"
	"github.com/guilhermemcandido/janus/internal/engine"
)

func main() {
	addr := flag.String("addr", ":50051", "listen address")
	flag.Parse()

	lis, err := net.Listen("tcp", *addr)
	if err != nil {
		log.Fatalf("listen on %s: %v", *addr, err)
	}

	exchange := engine.NewExchange()
	defer exchange.Close()

	grpcServer := grpc.NewServer()
	pb.RegisterExchangeServer(grpcServer, api.NewServer(exchange))

	log.Printf("janus gRPC server listening on %s", *addr)
	if err := grpcServer.Serve(lis); err != nil {
		log.Fatalf("serve: %v", err)
	}
}
