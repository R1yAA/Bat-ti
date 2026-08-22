// Package handlers serves the HTTP API. Files are split by the product PRD's
// pages — vendors.go is P1, compare.go is P2, and so on — so a requirement can
// be traced to the code that implements it.
package handlers

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/R1yAA/Bat-ti/app/database"
	"github.com/R1yAA/Bat-ti/app/middleware"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Server holds everything the handlers need.
type Server struct {
	pool          *pgxpool.Pool
	queries       *database.Queries
	logger        *slog.Logger
	authenticator *middleware.Authenticator
}

// NewServer builds the API server.
func NewServer(
	pool *pgxpool.Pool,
	logger *slog.Logger,
	authenticator *middleware.Authenticator,
) *Server {
	return &Server{
		pool:          pool,
		queries:       database.New(pool),
		logger:        logger,
		authenticator: authenticator,
	}
}

// BuildEngine wires every route. It is a factory rather than a global so the
// same engine can be served by a local binary, a container, or a serverless
// adapter without the routes being defined twice.
func (server *Server) BuildEngine() *gin.Engine {
	engine := gin.New()
	engine.Use(gin.Recovery(), server.requestLogger())

	// Health sits outside the authenticated group: a platform health check has
	// no session, and the answer reveals nothing beyond whether the database
	// is reachable.
	engine.GET("/api/health", server.handleHealth)

	// Everything else is private. This is one person's purchasing history and
	// vendor research, so the default is closed and exceptions are explicit.
	api := engine.Group("/api", server.authenticator.RequireUser())

	// P1 — vendor catalogue and listing content page
	api.GET("/vendors", server.handleListVendors)
	api.GET("/vendors/:vendorSlug/listings", server.handleListVendorListings)
	api.GET("/listings/:listingID", server.handleGetListing)
	api.PUT("/listings/:listingID/track", server.handleSetListingTracked)
	api.GET("/tracked-listings", server.handleListTrackedListings)

	// P2 — compare entries
	api.GET("/compare-entries", server.handleListCompareEntries)
	api.POST("/compare-entries", server.handleCreateCompareEntry)
	api.GET("/compare-entries/:entryID", server.handleGetCompareEntry)
	api.PUT("/compare-entries/:entryID", server.handleRenameCompareEntry)
	api.DELETE("/compare-entries/:entryID", server.handleDeleteCompareEntry)
	api.POST("/compare-entries/:entryID/members", server.handleAddCompareEntryMember)
	api.DELETE("/compare-entries/:entryID/members/:memberID", server.handleDeleteCompareEntryMember)

	// P3 — order history
	api.GET("/order-entries", server.handleListOrderEntries)
	api.POST("/order-entries", server.handleCreateOrderEntry)
	api.GET("/order-entries/:orderEntryID", server.handleGetOrderEntry)
	api.PUT("/order-entries/:orderEntryID", server.handleUpdateOrderEntry)
	api.DELETE("/order-entries/:orderEntryID", server.handleDeleteOrderEntry)
	api.POST("/order-entries/:orderEntryID/items", server.handleCreateOrderItem)
	api.PUT("/order-items/:orderItemID", server.handleUpdateOrderItem)
	api.DELETE("/order-items/:orderItemID", server.handleDeleteOrderItem)

	// P4 — spend distribution
	api.GET("/spend-summary", server.handleSpendSummary)
	api.GET("/spend-by-category", server.handleSpendByCategory)
	api.GET("/spend-monthly-trend", server.handleMonthlySpendTrend)

	// P5 — settings
	api.GET("/categories", server.handleListCategories)
	api.POST("/categories", server.handleCreateCategory)
	api.PUT("/categories/:categoryID", server.handleRenameCategory)
	api.DELETE("/categories/:categoryID", server.handleDeleteCategory)
	api.GET("/occasion-tags", server.handleListOccasionTags)
	api.POST("/occasion-tags", server.handleCreateOccasionTag)
	api.PUT("/occasion-tags/:tagID", server.handleRenameOccasionTag)
	api.DELETE("/occasion-tags/:tagID", server.handleDeleteOccasionTag)
	api.POST("/delete-all-data", server.handleDeleteAllData)

	// Scrape observability
	api.GET("/scrape-runs", server.handleListScrapeRuns)

	return engine
}

func (server *Server) requestLogger() gin.HandlerFunc {
	return func(context *gin.Context) {
		startedAt := time.Now()
		context.Next()
		server.logger.Info("request",
			"method", context.Request.Method,
			"path", context.Request.URL.Path,
			"status", context.Writer.Status(),
			"duration", time.Since(startedAt).Round(time.Millisecond).String())
	}
}

func (server *Server) handleHealth(context *gin.Context) {
	if err := server.pool.Ping(context); err != nil {
		respondError(context, http.StatusServiceUnavailable, "database is unreachable")
		return
	}
	context.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// ── shared helpers ────────────────────────────────────────────────────────

type errorResponse struct {
	Error string `json:"error"`
}

func respondError(context *gin.Context, statusCode int, message string) {
	context.AbortWithStatusJSON(statusCode, errorResponse{Error: message})
}

// respondDatabaseError maps the two database outcomes that are really client
// errors, and treats anything else as a fault on our side.
func (server *Server) respondDatabaseError(context *gin.Context, err error, subject string) {
	if errors.Is(err, pgx.ErrNoRows) {
		respondError(context, http.StatusNotFound, subject+" not found")
		return
	}
	server.logger.Error("database error", "subject", subject, "error", err)
	respondError(context, http.StatusInternalServerError, "something went wrong")
}

// parseUUIDParam reads a path parameter that must be a UUID.
func parseUUIDParam(context *gin.Context, parameterName string) (uuid.UUID, bool) {
	parsedID, err := uuid.Parse(context.Param(parameterName))
	if err != nil {
		respondError(context, http.StatusBadRequest, parameterName+" must be a UUID")
		return uuid.UUID{}, false
	}
	return parsedID, true
}

// parseBoolQuery reads an optional boolean query parameter.
func parseBoolQuery(context *gin.Context, parameterName string, defaultValue bool) bool {
	rawValue := context.Query(parameterName)
	if rawValue == "" {
		return defaultValue
	}
	parsedValue, err := strconv.ParseBool(rawValue)
	if err != nil {
		return defaultValue
	}
	return parsedValue
}

// parseIntQuery reads an optional integer query parameter, clamped to a range.
func parseIntQuery(context *gin.Context, parameterName string, defaultValue int, minimum int, maximum int) int {
	rawValue := context.Query(parameterName)
	if rawValue == "" {
		return defaultValue
	}
	parsedValue, err := strconv.Atoi(rawValue)
	if err != nil {
		return defaultValue
	}
	if parsedValue < minimum {
		return minimum
	}
	if parsedValue > maximum {
		return maximum
	}
	return parsedValue
}
