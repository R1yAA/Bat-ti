package handlers

import (
	"net/http"
	"time"

	"github.com/R1yAA/Bat-ti/app/database"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
)

// P4 — the spend distribution page. Quick presets (FR-P4-1: last day, week,
// month, 3/6 months, year) are just precomputed start_date/end_date pairs, so
// the API only ever needs to understand a custom range — the client computes
// which range a preset means.

func parseDateRangeQuery(context *gin.Context) (time.Time, time.Time, bool) {
	rawStartDate := context.Query("start_date")
	rawEndDate := context.Query("end_date")
	if rawStartDate == "" || rawEndDate == "" {
		respondError(context, http.StatusBadRequest,
			"start_date and end_date are required, formatted like 2026-08-22")
		return time.Time{}, time.Time{}, false
	}
	startDate, err := time.Parse(time.DateOnly, rawStartDate)
	if err != nil {
		respondError(context, http.StatusBadRequest, "start_date must be a date like 2026-08-22")
		return time.Time{}, time.Time{}, false
	}
	endDate, err := time.Parse(time.DateOnly, rawEndDate)
	if err != nil {
		respondError(context, http.StatusBadRequest, "end_date must be a date like 2026-08-22")
		return time.Time{}, time.Time{}, false
	}
	if endDate.Before(startDate) {
		respondError(context, http.StatusBadRequest, "end_date cannot be before start_date")
		return time.Time{}, time.Time{}, false
	}
	return startDate, endDate, true
}

// spendSummaryResponse carries both figures BR-12 defines, so the "include
// cancelled/refunded" toggle (FR-P4-4) is a client-side switch between
// net_spend and gross_spend rather than a second request.
type spendSummaryResponse struct {
	NetSpend       decimal.Decimal `json:"net_spend"`
	GrossSpend     decimal.Decimal `json:"gross_spend"`
	ExcludedSpend  decimal.Decimal `json:"excluded_spend"`
	RefundedAmount decimal.Decimal `json:"refunded_amount"`
	ItemCount      int64           `json:"item_count"`
	OrderCount     int64           `json:"order_count"`
}

func (server *Server) handleSpendSummary(context *gin.Context) {
	startDate, endDate, ok := parseDateRangeQuery(context)
	if !ok {
		return
	}
	summaryRow, err := server.queries.GetSpendSummary(context, database.GetSpendSummaryParams{
		StartDate: database.Date(startDate),
		EndDate:   database.Date(endDate),
	})
	if err != nil {
		server.respondDatabaseError(context, err, "spend summary")
		return
	}
	context.JSON(http.StatusOK, spendSummaryResponse{
		NetSpend:       summaryRow.NetSpend,
		GrossSpend:     summaryRow.GrossSpend,
		ExcludedSpend:  summaryRow.ExcludedSpend,
		RefundedAmount: summaryRow.RefundedAmount,
		ItemCount:      summaryRow.ItemCount,
		OrderCount:     summaryRow.OrderCount,
	})
}

type categorySpendResponse struct {
	CategoryName string          `json:"category_name"`
	NetSpend     decimal.Decimal `json:"net_spend"`
	GrossSpend   decimal.Decimal `json:"gross_spend"`
}

func (server *Server) handleSpendByCategory(context *gin.Context) {
	startDate, endDate, ok := parseDateRangeQuery(context)
	if !ok {
		return
	}
	categoryRows, err := server.queries.GetSpendByCategory(context, database.GetSpendByCategoryParams{
		StartDate: database.Date(startDate),
		EndDate:   database.Date(endDate),
	})
	if err != nil {
		server.respondDatabaseError(context, err, "spend by category")
		return
	}
	categories := make([]categorySpendResponse, 0, len(categoryRows))
	for _, categoryRow := range categoryRows {
		categories = append(categories, categorySpendResponse{
			CategoryName: categoryRow.CategoryName,
			NetSpend:     categoryRow.NetSpend,
			GrossSpend:   categoryRow.GrossSpend,
		})
	}
	context.JSON(http.StatusOK, gin.H{"categories": categories})
}

type monthlySpendResponse struct {
	Month      string          `json:"month"`
	NetSpend   decimal.Decimal `json:"net_spend"`
	GrossSpend decimal.Decimal `json:"gross_spend"`
}

// handleMonthlySpendTrend ignores any date-range filter by design (FR-P4-6):
// it is always the trailing twelve months, for pacing comparisons that a
// custom range would undermine.
func (server *Server) handleMonthlySpendTrend(context *gin.Context) {
	monthRows, err := server.queries.GetMonthlySpendTrend(context)
	if err != nil {
		server.respondDatabaseError(context, err, "monthly spend trend")
		return
	}
	months := make([]monthlySpendResponse, 0, len(monthRows))
	for _, monthRow := range monthRows {
		months = append(months, monthlySpendResponse{
			Month:      database.DateValue(monthRow.MonthStart).Format("2006-01"),
			NetSpend:   monthRow.NetSpend,
			GrossSpend: monthRow.GrossSpend,
		})
	}
	context.JSON(http.StatusOK, gin.H{"months": months})
}
