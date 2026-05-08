package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/yourname/tracing/internal/demo"
)

func main() {
	fmt.Println("traffic generator starting")

	collectorURL := os.Getenv("COLLECTOR_URL")
	if collectorURL == "" {
		collectorURL = "http://localhost:4318"
	}
	runner := demo.NewRunner(collectorURL)
	runner.Start()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	runner.Stop()
	fmt.Println("traffic generator stopped")
}
