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

	"github.com/go-chi/chi/v5"
	coltracev1 "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	"google.golang.org/grpc"

	"github.com/yourname/tracing/api"
	"github.com/yourname/tracing/internal/metrics"
	"github.com/yourname/tracing/internal/storage"
)

func main() {
	fmt.Println("collector starting on :4318")

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// If DATA_DIR is set, use BadgerDB for durable on-disk persistence.
	// Otherwise, fall back to the in-memory store.
	var store storage.TraceStore
	if dataDir := os.Getenv("DATA_DIR"); dataDir != "" {
		bs, err := storage.OpenBadger(dataDir, 10000)
		if err != nil {
			log.Fatalf("badger open: %v", err)
		}
		defer bs.Close()
		store = bs
		log.Printf("using BadgerDB persistence at %s", dataDir)
	} else {
		mem := storage.NewMemoryStore(10000)
		// Optionally evict traces older than TRACE_TTL (e.g. "1h", "30m").
		if ttlStr := os.Getenv("TRACE_TTL"); ttlStr != "" {
			if ttl, err := time.ParseDuration(ttlStr); err == nil && ttl > 0 {
				mem.StartRetention(ctx, ttl, ttl/10)
				log.Printf("trace retention enabled: TTL=%s", ttl)
			} else {
				log.Printf("invalid TRACE_TTL %q, retention disabled", ttlStr)
			}
		}
		store = mem
	}

	metricsStore := metrics.NewMetricsStore()
	sseBus := api.NewSSEBus()
	pipeline := api.NewPipelineWithDefaults(store, metricsStore, sseBus)

	apiKey := os.Getenv("API_KEY")
	if apiKey != "" {
		log.Printf("API key authentication enabled")
	}

	// Start gRPC OTLP receiver (port 4317)
	grpcAddr := ":4317"
	if a := os.Getenv("GRPC_ADDR"); a != "" {
		grpcAddr = a
	}
	grpcSrv := grpc.NewServer()
	coltracev1.RegisterTraceServiceServer(grpcSrv, api.NewOTLPTraceServiceServer(pipeline))
	go func() {
		lis, err := net.Listen("tcp", grpcAddr)
		if err != nil {
			log.Printf("gRPC listen %s: %v (gRPC disabled)", grpcAddr, err)
			return
		}
		log.Printf("gRPC OTLP receiver listening on %s", grpcAddr)
		if err := grpcSrv.Serve(lis); err != nil {
			log.Printf("gRPC serve: %v", err)
		}
	}()
	go func() {
		<-ctx.Done()
		grpcSrv.GracefulStop()
	}()

	r := chi.NewRouter()
	api.SetupRoutes(ctx, r, pipeline, store, metricsStore, sseBus, apiKey)

	addr := ":4318"
	if a := os.Getenv("LISTEN_ADDR"); a != "" {
		addr = a
	}
	srv := &http.Server{Addr: addr, Handler: r}

	certFile := os.Getenv("TLS_CERT_FILE")
	keyFile := os.Getenv("TLS_KEY_FILE")

	go func() {
		if certFile != "" && keyFile != "" {
			log.Printf("collector listening on %s (TLS)", addr)
			if err := srv.ListenAndServeTLS(certFile, keyFile); err != nil && err != http.ErrServerClosed {
				log.Fatalf("server error: %v", err)
			}
		} else {
			log.Printf("collector listening on %s", addr)
			if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				log.Fatalf("server error: %v", err)
			}
		}
	}()

	<-ctx.Done()
	fmt.Println("shutting down collector")
}
