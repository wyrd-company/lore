package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/wyrd-company/lore/internal/config"
	"github.com/wyrd-company/lore/internal/database"
	"github.com/wyrd-company/lore/internal/httpapi"
	"github.com/wyrd-company/lore/internal/version"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		slog.Error("lore-server failed", "error", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	command := "serve"
	if len(args) > 0 {
		command = args[0]
	}
	cfg := config.FromEnvironment()
	switch command {
	case "version", "--version", "-version":
		fmt.Printf("lore-server %s\n", version.Value)
		return nil
	case "migrate":
		if err := cfg.ValidateDatabase(); err != nil {
			return err
		}
		return database.Migrate(context.Background(), cfg.DatabaseURL)
	case "serve":
		return serve(cfg)
	default:
		return fmt.Errorf("unknown command %q (expected serve, migrate, or version)", command)
	}
}

func serve(cfg config.Config) error {
	if err := cfg.ValidateServer(); err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("create database pool: %w", err)
	}
	defer pool.Close()

	server := &http.Server{
		Addr: cfg.ListenAddress, Handler: httpapi.New(pool, cfg.IngestToken, cfg.AdminToken),
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() {
		<-ctx.Done()
		shutdownContext, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		server.Shutdown(shutdownContext) //nolint:errcheck
	}()
	slog.Info("Lore server listening", "address", cfg.ListenAddress, "publicBaseURL", cfg.PublicBaseURL)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("serve HTTP: %w", err)
	}
	return nil
}
