package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/guilhermemcandido/janus/internal/web"
	"github.com/guilhermemcandido/janus/pkg/client"
)

const shutdownGracePeriod = 5 * time.Second

func main() {
	addr := flag.String("addr", "localhost:50051", "gRPC address of the Janus exchange")
	listen := flag.String("listen", ":8080", "HTTP listen address for the web UI")
	flag.Parse()

	c, err := client.Dial(*addr)
	if err != nil {
		log.Fatalf("dial %s: %v", *addr, err)
	}
	defer c.Close()

	mux := http.NewServeMux()
	mux.Handle("/ws", web.NewWebSocketHandler(c))
	httpServer := &http.Server{Addr: *listen, Handler: mux}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	shutdownDone := make(chan struct{})
	go func() {
		defer close(shutdownDone)
		<-ctx.Done()
		log.Println("shutting down...")
		// Shutdown only waits on regular HTTP handlers - hijacked WebSocket connections are
		// already detached from the server's tracking, so they can't block this.
		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownGracePeriod)
		defer cancel()
		if err := httpServer.Shutdown(shutdownCtx); err != nil {
			log.Printf("shutdown: %v", err)
		}
	}()

	log.Printf("janus web UI listening on %s (exchange at %s)", *listen, *addr)
	if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("serve: %v", err)
	}
	<-shutdownDone
}
