// Command dokseo-sync runs the Dokseo sync server.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	_ "modernc.org/sqlite"

	"github.com/jerhage/dokseo-sync/internal/api"
	"github.com/jerhage/dokseo-sync/internal/config"
	"github.com/jerhage/dokseo-sync/internal/store"
)

const shutdownTimeout = 10 * time.Second

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	err := run(ctx, os.Args[1:], os.Getenv, os.Stderr)
	stop()
	if errors.Is(err, flag.ErrHelp) {
		return
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "dokseo-sync: %v\n", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, getenv func(string) string, stderr io.Writer) (err error) {
	cfg, err := config.Load(args, getenv)
	if errors.Is(err, flag.ErrHelp) {
		if _, writeErr := fmt.Fprint(stderr, config.Usage); writeErr != nil {
			return writeErr
		}
		return err
	}
	if err != nil {
		return fmt.Errorf("%w\n\n%s", err, config.Usage)
	}
	logger := slog.New(slog.NewTextHandler(stderr, nil))

	if err := os.MkdirAll(cfg.DataDir, 0o700); err != nil {
		return fmt.Errorf("create data directory: %w", err)
	}
	st, err := store.Open(filepath.Join(cfg.DataDir, "dokseo-sync.db"))
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	defer func() {
		if closeErr := st.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("close store: %w", closeErr))
		}
	}()

	srv, err := newServer(ctx, cfg, st, logger)
	if err != nil {
		return err
	}
	ln, err := net.Listen("tcp", cfg.Listen)
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}
	logger.Info("starting", "addr", ln.Addr().String(), "data", cfg.DataDir, "origins", len(cfg.AllowedOrigins), "version", api.Version)

	serveErr := make(chan error, 1)
	go func() { serveErr <- srv.Serve(ln) }()
	select {
	case err := <-serveErr:
		return fmt.Errorf("serve: %w", err)
	case <-ctx.Done():
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutdown: %w", err)
	}
	logger.Info("stopped")
	return nil
}

func newServer(ctx context.Context, cfg config.Config, st *store.Store, logger *slog.Logger) (*http.Server, error) {
	account, err := st.EnsureAccount(ctx, cfg.Credentials.Username)
	if err != nil {
		return nil, fmt.Errorf("ensure account: %w", err)
	}
	handler := api.New(st, api.Options{
		AccountID:      account.ID,
		Credentials:    api.Credentials{Username: cfg.Credentials.Username, Password: cfg.Credentials.Password},
		AllowedOrigins: cfg.AllowedOrigins,
		Logger:         logger,
	})
	return &http.Server{
		Addr:              cfg.Listen,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}, nil
}
