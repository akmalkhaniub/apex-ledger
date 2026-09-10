package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/akmalkhaniub/apex-ledger/internal/api"
	"github.com/akmalkhaniub/apex-ledger/internal/domain/ledger"
	gen "github.com/akmalkhaniub/apex-ledger/internal/generated/api"
	"github.com/akmalkhaniub/apex-ledger/internal/middleware"
	"github.com/akmalkhaniub/apex-ledger/internal/repository"
	"github.com/go-chi/chi/v5"
	chiMiddleware "github.com/go-chi/chi/v5/middleware"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	// 1. Initialize core storage & domain service
	repo := repository.NewMemoryRepository()
	service := ledger.NewService(repo)
	apiHandler := api.NewLedgerAPI(service)

	// 2. Setup Chi router with standard middleware
	r := chi.NewRouter()
	r.Use(chiMiddleware.RequestID)
	r.Use(chiMiddleware.RealIP)
	r.Use(chiMiddleware.Logger)
	r.Use(chiMiddleware.Recoverer)
	r.Use(chiMiddleware.Timeout(30 * time.Second))

	// 3. Mount IETF Idempotency Middleware on mutating endpoints
	r.Use(middleware.IdempotencyMiddleware(repo))

	// 4. Register OpenAPI generated routes
	gen.HandlerFromMux(apiHandler, r)

	// Health check endpoint
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"UP","engine":"ApexLedger"}`))
	})

	srv := &http.Server{
		Addr:         fmt.Sprintf(":%s", port),
		Handler:      r,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Graceful shutdown handling
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	go func() {
		log.Printf("⚡ ApexLedger Core Banking API running on http://localhost:%s", port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server failed to start: %v", err)
		}
	}()

	<-stop
	log.Println("Shutting down ApexLedger gracefully...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}

	log.Println("ApexLedger server exited cleanly.")
}
