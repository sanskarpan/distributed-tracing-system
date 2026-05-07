package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os/signal"
	"syscall"

	"github.com/go-chi/chi/v5"
	"github.com/yourname/tracing/api"
	"github.com/yourname/tracing/internal/metrics"
	"github.com/yourname/tracing/internal/storage"
)

func main() {
	fmt.Println("collector starting on :4318")

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	store := storage.NewMemoryStore(10000)
	metricsStore := metrics.NewMetricsStore()
	sseBus := api.NewSSEBus()
	pipeline := api.NewPipelineWithDefaults(store, metricsStore, sseBus)

	r := chi.NewRouter()
	api.SetupRoutes(ctx, r, pipeline, store, metricsStore, sseBus)

	srv := &http.Server{Addr: ":4318", Handler: r}

	go func() {
		log.Printf("collector listening on :4318")
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server error: %v", err)
		}
	}()

	<-ctx.Done()
	fmt.Println("shutting down collector")
}
