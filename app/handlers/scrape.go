package handlers

import (
	"net/http"
	"time"

	"github.com/R1yAA/Bat-ti/app/database"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// Scrape observability. Both PRDs name silent scrape failure as a real risk, so
// the run history is a first-class thing the UI can show rather than something
// only visible in CI logs.

type scrapeRunResponse struct {
	ScrapeRunID      uuid.UUID  `json:"scrape_run_id"`
	VendorSlug       string     `json:"vendor_slug"`
	VendorName       string     `json:"vendor_name"`
	StartedAt        *time.Time `json:"started_at"`
	FinishedAt       *time.Time `json:"finished_at"`
	RunStatus        string     `json:"run_status"`
	ListingsSeen     int32      `json:"listings_seen"`
	ListingsUpdated  int32      `json:"listings_updated"`
	ListingsDelisted int32      `json:"listings_delisted"`
	ErrorMessage     *string    `json:"error_message"`
}

func (server *Server) handleListScrapeRuns(context *gin.Context) {
	runLimit := parseIntQuery(context, "limit", 50, 1, 500)

	runRows, err := server.queries.ListRecentScrapeRuns(context, int32(runLimit))
	if err != nil {
		server.respondDatabaseError(context, err, "scrape runs")
		return
	}

	runs := make([]scrapeRunResponse, 0, len(runRows))
	for _, runRow := range runRows {
		runs = append(runs, scrapeRunResponse{
			ScrapeRunID:      runRow.ScrapeRunID,
			VendorSlug:       runRow.VendorSlug,
			VendorName:       runRow.VendorName,
			StartedAt:        database.TimeValue(runRow.StartedAt),
			FinishedAt:       database.TimeValue(runRow.FinishedAt),
			RunStatus:        runRow.RunStatus,
			ListingsSeen:     runRow.ListingsSeen,
			ListingsUpdated:  runRow.ListingsUpdated,
			ListingsDelisted: runRow.ListingsDelisted,
			ErrorMessage:     database.TextValue(runRow.ErrorMessage),
		})
	}
	context.JSON(http.StatusOK, gin.H{"scrape_runs": runs})
}
