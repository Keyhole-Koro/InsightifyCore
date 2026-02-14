package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"insightify/internal/gateway/app"
)

func main() {
	a, err := app.New()
	if err != nil {
		log.Fatalf("Failed to initialize app: %v", err)
	}

	go func() {
		if err := a.Start(); err != nil {
			log.Printf("Server error: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := a.Shutdown(ctx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}

	log.Println("Server exiting")
}
