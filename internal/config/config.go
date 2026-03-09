package config

import (
	"context"
	"fmt"
	"os"

	"gps-listener-backend/internal/storage"
)

type ctxKey string

const storeKey ctxKey = "store"

type Config struct {
	AppEnv      string
	ServiceName string
	TCPAddr     string
	DatabaseURL string
}

func Load(serviceName string) (Config, error) {
	cfg := Config{
		AppEnv:      getenv("APP_ENV", "development"),
		ServiceName: serviceName,
		TCPAddr:     resolveTCPAddr(serviceName),
		DatabaseURL: os.Getenv("DATABASE_URL"),
	}

	if cfg.DatabaseURL == "" {
		return Config{}, fmt.Errorf("DATABASE_URL is required")
	}
	return cfg, nil
}

func WithStore(ctx context.Context, store *storage.Store) context.Context {
	return context.WithValue(ctx, storeKey, store)
}

func StoreFromContext(ctx context.Context) *storage.Store {
	v, _ := ctx.Value(storeKey).(*storage.Store)
	return v
}

func resolveTCPAddr(serviceName string) string {
	specificKey := map[string]string{
		"gt06-listener":      "GT06_TCP_ADDR",
		"uniguard-listener":  "UNIGUARD_TCP_ADDR",
		"teltonika-listener": "TELTONIKA_TCP_ADDR",
	}
	if key, ok := specificKey[serviceName]; ok {
		if v := os.Getenv(key); v != "" {
			return v
		}
	}
	return getenv("TCP_ADDR", ":9000")
}

func getenv(key, fallback string) string {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	return v
}
