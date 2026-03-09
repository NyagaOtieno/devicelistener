package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"gps-listener-backend/internal/httpapi"
	"gps-listener-backend/internal/storage"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	db := getenv("DATABASE_URL", "")
	if db == "" {
		log.Fatal("DATABASE_URL is required")
	}
	store, err := storage.New(ctx, db)
	if err != nil {
		log.Fatal(err)
	}
	defer store.Close()

	srv := &http.Server{Addr: httpapi.ListenAddr(), Handler: httpapi.New(store).Handler()}
	go func() {
		log.Printf("api started on %s", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal(err)
		}
	}()
	<-ctx.Done()
	shutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutCtx)
}

func getenv(k, fallback string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return fallback
}
