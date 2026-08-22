// Package handler is the Vercel entry point. Vercel's Go runtime compiles each
// .go file under /api into its own serverless function and calls the exported
// Handler; vercel.json routes every /api/* path here, so the one function
// serves the whole API.
//
// The Gin engine is built by the same factory cmd/api uses, so a route defined
// once is served identically by the local binary and by this.
package handler

import (
	"context"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/R1yAA/Bat-ti/internal/handlers"
	"github.com/R1yAA/Bat-ti/internal/middleware"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	// Built once per warm instance rather than per request. A serverless
	// function is reused across invocations, and reconnecting to Postgres on
	// every request would spend more time on handshakes than on queries.
	initialiseOnce sync.Once
	sharedEngine   *gin.Engine
	initialiseErr  error
)

// Handler is the entry point Vercel invokes.
func Handler(responseWriter http.ResponseWriter, request *http.Request) {
	initialiseOnce.Do(func() {
		sharedEngine, initialiseErr = buildEngine()
	})

	if initialiseErr != nil {
		// Logged rather than returned to the caller: a configuration failure
		// names infrastructure, and that is not the client's business.
		slog.Error("could not start the API", "error", initialiseErr)
		http.Error(responseWriter, `{"error":"the server is misconfigured"}`,
			http.StatusInternalServerError)
		return
	}

	sharedEngine.ServeHTTP(responseWriter, request)
}

func buildEngine() (*gin.Engine, error) {
	// Requests are short-lived here, but startup talks to Supabase for the
	// JWKS, so the bound is generous enough for a cold start on a slow link.
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	logger := slog.New(slog.NewJSONHandler(os.Stderr, nil))
	gin.SetMode(gin.ReleaseMode)

	pool, err := newPool(ctx, os.Getenv("DATABASE_URL"))
	if err != nil {
		return nil, err
	}

	authenticator, err := middleware.NewAuthenticator(ctx, os.Getenv("SUPABASE_URL"), logger)
	if err != nil {
		return nil, err
	}

	return handlers.NewServer(pool, logger, authenticator).BuildEngine(), nil
}

// newPool opens a connection pool sized and configured for serverless.
func newPool(ctx context.Context, databaseURL string) (*pgxpool.Pool, error) {
	poolConfig, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, err
	}

	// Many function instances can be alive at once, each with its own pool, so
	// a generous per-instance pool is how a connection limit gets exhausted.
	// A handful of concurrent queries per instance is plenty.
	poolConfig.MaxConns = 4
	poolConfig.MinConns = 0
	// Idle instances are frozen rather than shut down, so connections are
	// retired on a timer instead of being held until the pooler drops them.
	poolConfig.MaxConnIdleTime = 30 * time.Second
	poolConfig.MaxConnLifetime = 5 * time.Minute

	if isTransactionPooler(databaseURL) {
		// Supabase's transaction pooler hands each statement to whichever
		// backend is free, so a statement prepared on one connection is not
		// there when the next one runs — the classic "prepared statement
		// already exists" failure under load. QueryExecModeExec keeps proper
		// parameter binding while using unnamed statements, which is safe to
		// pool; simple protocol would interpolate arguments instead.
		poolConfig.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeExec
		poolConfig.ConnConfig.StatementCacheCapacity = 0
		poolConfig.ConnConfig.DescriptionCacheCapacity = 0
	}

	return pgxpool.NewWithConfig(ctx, poolConfig)
}

// isTransactionPooler reports whether the URL points at Supabase's
// transaction-mode pooler, which listens on 6543. The session pooler on 5432
// pins a backend per connection and needs none of the above.
func isTransactionPooler(databaseURL string) bool {
	parsed, err := url.Parse(databaseURL)
	if err != nil {
		return false
	}
	return parsed.Port() == "6543" ||
		strings.Contains(parsed.Query().Get("options"), "transaction")
}
