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

	runner := demo.NewRunner("http://localhost:4318")
	runner.Start()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	runner.Stop()
	fmt.Println("traffic generator stopped")
}
