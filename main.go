package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	handler "github.com/yafyx/baak-api/api"
	"github.com/yafyx/baak-api/config"
)

func main() {
	config.LoadConfig()
	if err := config.AppConfig.Validate(); err != nil {
		log.Fatalf("invalid configuration: %v", err)
	}

	port := config.AppConfig.Port
	fmt.Printf("Server starting on port %s...\n", port)
	server := &http.Server{
		Addr:              port,
		Handler:           http.HandlerFunc(Handler),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      config.RequestTimeout + 5*time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}
	listener, err := net.Listen("tcp", port)
	if err != nil {
		log.Fatalf("failed to listen on %s: %v", port, err)
	}

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	serveErr := make(chan error, 1)
	go func() { serveErr <- server.Serve(listener) }()

	select {
	case err := <-serveErr:
		if err != nil && err != http.ErrServerClosed {
			log.Fatalf("server stopped: %v", err)
		}
	case <-stop:
		ctx, cancel := context.WithTimeout(context.Background(), config.ShutdownTimeout)
		defer cancel()
		if err := server.Shutdown(ctx); err != nil {
			log.Printf("graceful shutdown failed: %v", err)
			_ = server.Close()
		}
	}
}

// Handler is exported to be used by Vercel
func Handler(w http.ResponseWriter, r *http.Request) {
	handler.Handler(w, r)
}
