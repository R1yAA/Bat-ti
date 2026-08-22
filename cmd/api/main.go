// Command api serves the HTTP API described in the tech PRD's route table.
//
//	go run ./cmd/api            serve on :8080, or $PORT if set
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/R1yAA/Bat-ti/app/handlers"
	"github.com/R1yAA/Bat-ti/app/middleware"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	if err := run(logger); err != nil {
		logger.Error("api command failed", "error", err)
		os.Exit(1)
	}
}

// buildAuthenticator wires up Supabase JWT verification.
//
// Turning authentication off requires saying so out loud with
// AUTH_DISABLED=true, and is refused unless the database is a local one. An
// unauthenticated deployment pointed at the real database would expose every
// order and price in it, and that must not be reachable by forgetting to set a
// variable.
func buildAuthenticator(
	ctx context.Context,
	logger *slog.Logger,
) (*middleware.Authenticator, error) {
	if os.Getenv("AUTH_DISABLED") == "true" {
		if !isLocalDatabase(os.Getenv("DATABASE_URL")) {
			return nil, errors.New(
				"AUTH_DISABLED is only honoured against a local database; " +
					"refusing to serve a remote one unauthenticated")
		}
		return middleware.NewDisabledAuthenticator(logger), nil
	}
	return middleware.NewAuthenticator(ctx, os.Getenv("SUPABASE_URL"), logger)
}

// isLocalDatabase reports whether the URL points at this machine.
//
// The host is parsed out rather than matched as a substring: "@localhost"
// appears in postgres://user:pw@localhost.example.com/db too, and treating
// that as local would serve a remote database with authentication switched
// off — the exact outcome the caller is guarding against.
func isLocalDatabase(databaseURL string) bool {
	parsed, err := url.Parse(databaseURL)
	if err != nil {
		return false
	}
	switch parsed.Hostname() {
	case "localhost", "127.0.0.1", "::1":
		return true
	default:
		return false
	}
}

func run(logger *slog.Logger) error {
	ctx, stopSignalHandling := signal.NotifyContext(context.Background(),
		os.Interrupt, syscall.SIGTERM)
	defer stopSignalHandling()

	_ = godotenv.Load()
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		return fmt.Errorf("DATABASE_URL is not set; copy .env.example to .env or export it")
	}

	// Settled before anything else connects: whether this process is willing
	// to serve unauthenticated must not depend on the database answering.
	authenticator, err := buildAuthenticator(ctx, logger)
	if err != nil {
		return err
	}

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return fmt.Errorf("connecting to the database: %w", err)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		return fmt.Errorf("pinging the database: %w", err)
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	server := handlers.NewServer(pool, logger, authenticator)
	httpServer := &http.Server{
		Addr:    ":" + port,
		Handler: server.BuildEngine(),
	}

	serveErrors := make(chan error, 1)
	go func() {
		logger.Info("listening", "port", port)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErrors <- err
		}
		close(serveErrors)
	}()

	select {
	case err := <-serveErrors:
		if err != nil {
			return fmt.Errorf("serving: %w", err)
		}
	case <-ctx.Done():
		logger.Info("shutting down")
		shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancelShutdown()
		if err := httpServer.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("shutting down: %w", err)
		}
	}
	return nil
}
