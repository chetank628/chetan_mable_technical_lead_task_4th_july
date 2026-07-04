// Command api is the Mable tracking ingest + analytics service. It loads
// configuration, opens and migrates the SQLite store, starts the long-running
// windowed pipeline worker, mounts the HTTP API, and shuts everything down
// gracefully on SIGINT/SIGTERM: stop accepting, drain the ingest channel, flush
// the final window, then close the database.
package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/mable/mono/api/internal/auth"
	"github.com/mable/mono/api/internal/config"
	"github.com/mable/mono/api/internal/handlers"
	"github.com/mable/mono/api/internal/ingest"
	"github.com/mable/mono/api/internal/store"
)

func main() {
	if err := run(); err != nil {
		log.Fatalf("fatal: %v", err)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	if cfg.IsProd() {
		gin.SetMode(gin.ReleaseMode)
	}

	// Root context cancelled on the first SIGINT/SIGTERM.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	st, err := store.Open(ctx, cfg.DBPath)
	if err != nil {
		return err
	}
	defer func() { _ = st.Close() }()

	worker := ingest.New(cfg, st)
	go worker.Run(ctx)

	authn := auth.New(cfg.JWTSecret, cfg.JWTTTL, cfg.IsProd())
	h := handlers.New(cfg, st, worker, authn)

	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           h.Router(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	// Run the HTTP server until the root context is cancelled.
	serverErr := make(chan error, 1)
	go func() {
		log.Printf("api listening on %s (env=%s, db=%s)", cfg.Addr, cfg.Env, cfg.DBPath)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
	}()

	select {
	case err := <-serverErr:
		return err
	case <-ctx.Done():
		log.Print("shutdown signal received")
	}

	// Graceful shutdown sequence.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// 1. Stop accepting new HTTP connections (no more events submitted).
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("http shutdown: %v", err)
	}
	// 2. Close the ingest channel and wait for the worker to flush the final
	//    window to SQLite.
	worker.Close()
	worker.Wait()
	log.Print("ingest worker drained; bye")
	return nil
}
