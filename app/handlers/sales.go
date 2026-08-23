package handlers

import (
	"net/http"

	"github.com/R1yAA/Bat-ti/app/database"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
)

// The sell-side half of P4 — FR-P4-7 (Total Gain) and FR-P4-8 (Sales by
// Category). Its buy-side twin is spend.go, and the two share the date-range
// parsing so a preset means the same span on both halves of the page.
//
// "Gain" throughout, never "profit" (OPEN-5): this is what came in from
// customers, with nothing subtracted for the materials that went into it.
// Calling it profit would overstate the business by the whole cost of goods.

// salesSummaryResponse carries both figures, so the page's "include cancelled"
// toggle switches between two numbers already in hand (OPEN-6, mirroring the
// BR-12 pattern) rather than issuing a second request.
type salesSummaryResponse struct {
	NetGain       decimal.Decimal `json:"net_gain"`
	GrossGain     decimal.Decimal `json:"gross_gain"`
	CancelledGain decimal.Decimal `json:"cancelled_gain"`
	SaleCount     int64           `json:"sale_count"`
	PendingCount  int64           `json:"pending_count"`
}

func (server *Server) handleSalesSummary(context *gin.Context) {
	startDate, endDate, ok := parseDateRangeQuery(context)
	if !ok {
		return
	}
	summaryRow, err := server.queries.GetSalesSummary(context, database.GetSalesSummaryParams{
		StartDate: database.Date(startDate),
		EndDate:   database.Date(endDate),
	})
	if err != nil {
		server.respondDatabaseError(context, err, "sales summary")
		return
	}
	context.JSON(http.StatusOK, salesSummaryResponse{
		NetGain:       summaryRow.NetGain,
		GrossGain:     summaryRow.GrossGain,
		CancelledGain: summaryRow.CancelledGain,
		SaleCount:     summaryRow.SaleCount,
		PendingCount:  summaryRow.PendingCount,
	})
}

type categorySalesResponse struct {
	CategoryName string          `json:"category_name"`
	NetGain      decimal.Decimal `json:"net_gain"`
	GrossGain    decimal.Decimal `json:"gross_gain"`
}

func (server *Server) handleSalesByCategory(context *gin.Context) {
	startDate, endDate, ok := parseDateRangeQuery(context)
	if !ok {
		return
	}
	categoryRows, err := server.queries.GetSalesByCategory(context,
		database.GetSalesByCategoryParams{
			StartDate: database.Date(startDate),
			EndDate:   database.Date(endDate),
		})
	if err != nil {
		server.respondDatabaseError(context, err, "sales by category")
		return
	}
	categories := make([]categorySalesResponse, 0, len(categoryRows))
	for _, categoryRow := range categoryRows {
		categories = append(categories, categorySalesResponse{
			CategoryName: categoryRow.CategoryName,
			NetGain:      categoryRow.NetGain,
			GrossGain:    categoryRow.GrossGain,
		})
	}
	context.JSON(http.StatusOK, gin.H{"categories": categories})
}

type monthlySalesResponse struct {
	Month     string          `json:"month"`
	NetGain   decimal.Decimal `json:"net_gain"`
	GrossGain decimal.Decimal `json:"gross_gain"`
}

// handleMonthlySalesTrend ignores the date-range filter for the same reason
// its spend twin does (FR-P4-6): it is always the trailing twelve months, so
// the two charts can be read against each other month for month.
func (server *Server) handleMonthlySalesTrend(context *gin.Context) {
	monthRows, err := server.queries.GetMonthlySalesTrend(context)
	if err != nil {
		server.respondDatabaseError(context, err, "monthly sales trend")
		return
	}
	months := make([]monthlySalesResponse, 0, len(monthRows))
	for _, monthRow := range monthRows {
		months = append(months, monthlySalesResponse{
			Month:     database.DateValue(monthRow.MonthStart).Format("2006-01"),
			NetGain:   monthRow.NetGain,
			GrossGain: monthRow.GrossGain,
		})
	}
	context.JSON(http.StatusOK, gin.H{"months": months})
}
