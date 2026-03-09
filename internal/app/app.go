package app

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"gps-listener-backend/internal/config"
	"gps-listener-backend/internal/storage"
)

type TCPService interface {
	HandleConnection(context.Context, net.Conn)
}

func Run(serviceName string, handler TCPService) {
	cfg, err := config.Load(serviceName)
	if err != nil {
		log.Fatal(err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	store, err := storage.New(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatal(err)
	}
	defer store.Close()

	serviceCtx := config.WithStore(ctx, store)

	ln, err := net.Listen("tcp", cfg.TCPAddr)
	if err != nil {
		log.Fatal(fmt.Errorf("listen on %s: %w", cfg.TCPAddr, err))
	}
	defer ln.Close()

	log.Printf("%s started on %s", serviceName, cfg.TCPAddr)

	go func() {
		<-serviceCtx.Done()
		_ = ln.Close()
	}()

	for {
		conn, err := ln.Accept()
		if err != nil {
			select {
			case <-serviceCtx.Done():
				log.Printf("%s shutting down", serviceName)
				return
			default:
				log.Printf("accept error: %v", err)
				time.Sleep(250 * time.Millisecond)
				continue
			}
		}

		go func(c net.Conn) {
			defer func() { _ = c.Close() }()
			handler.HandleConnection(serviceCtx, c)
		}(conn)
	}
}

func MustEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		log.Fatalf("missing required env %s", key)
	}
	return v
}
