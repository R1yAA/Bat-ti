package handlers

import (
	"errors"
	"math/rand/v2"
	"net/http"
	"strings"
	"time"

	"github.com/R1yAA/Bat-ti/app/database"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/shopspring/decimal"
)

// P6 — the sell side: sale order entries and the items inside them.
//
// This file is the mirror of orders.go, and the two never share a type. The
// addendum is emphatic about that (see .claude/agent-prd-v2.md §1): an order
// is something bought from a vendor, a sale order is something sold to a
// customer, and a single "Order" concept spanning both would make every spend
// figure in P4 ambiguous.

// Sale order statuses (BR-20). Held here as well as in the check constraint so
// a bad value is a readable 400 rather than a constraint violation surfacing
// as a 500.
//
// The order of this slice is the order the workflow runs in, and the UI shows
// the dropdown in it. Transitions themselves are not policed: BR-24 makes
// every field editable after the fact, and BR-20 is flagged as an assumption
// (OPEN-4), so refusing a correction would be enforcing a guess.
var saleOrderStatuses = []string{
	"pending", "confirmed", "shipped", "delivered", "cancelled",
}

func isValidSaleOrderStatus(orderStatus string) bool {
	for _, status := range saleOrderStatuses {
		if status == orderStatus {
			return true
		}
	}
	return false
}

// uncategorizedSaleOrderCategoryName is the reassignment target BR-23 names,
// and the fallback for a sale created without a category chosen.
const uncategorizedSaleOrderCategoryName = "Uncategorized"

type saleOrderEntryResponse struct {
	SaleOrderEntryID    uuid.UUID               `json:"sale_order_entry_id"`
	SaleOrderID         int32                   `json:"sale_order_id"`
	ConsumerName        string                  `json:"consumer_name"`
	OrderPlacedDate     string                  `json:"order_placed_date"`
	OrderStatus         string                  `json:"order_status"`
	DeliveredDate       *string                 `json:"delivered_date"`
	SaleOrderCategoryID uuid.UUID               `json:"sale_order_category_id"`
	CategoryName        string                  `json:"category_name"`
	TotalAmount         decimal.Decimal         `json:"total_amount"`
	ItemCount           int64                   `json:"item_count"`
	Items               []saleOrderItemResponse `json:"items,omitempty"`
}

type saleOrderItemResponse struct {
	SaleOrderItemID  uuid.UUID       `json:"sale_order_item_id"`
	SaleOrderEntryID uuid.UUID       `json:"sale_order_entry_id"`
	ProductName      string          `json:"product_name"`
	Quantity         int32           `json:"quantity"`
	PricePerUnit     decimal.Decimal `json:"price_per_unit"`
	LineTotal        decimal.Decimal `json:"line_total"`
}

// formatNullableDate renders a date the API reports as absent when unset,
// rather than as the zero date.
func formatNullableDate(value *time.Time) *string {
	if value == nil {
		return nil
	}
	formatted := value.Format(time.DateOnly)
	return &formatted
}

// ── list (FR-P6-5, FR-P6-6) ───────────────────────────────────────────────

func (server *Server) handleListSaleOrderEntries(context *gin.Context) {
	// FR-P6-6 asks only for a "pending" filter, but the parameter takes any
	// status: one filter and five is the same query, and the extra four cost
	// nothing to allow.
	statusFilter := strings.TrimSpace(context.Query("status"))
	if statusFilter != "" && !isValidSaleOrderStatus(statusFilter) {
		respondError(context, http.StatusBadRequest,
			"status must be one of "+strings.Join(saleOrderStatuses, ", "))
		return
	}

	entryRows, err := server.queries.ListSaleOrderEntries(context, statusFilter)
	if err != nil {
		server.respondDatabaseError(context, err, "sale order entries")
		return
	}

	entries := make([]saleOrderEntryResponse, 0, len(entryRows))
	for _, entryRow := range entryRows {
		entries = append(entries, saleOrderEntryResponse{
			SaleOrderEntryID:    entryRow.SaleOrderEntryID,
			SaleOrderID:         entryRow.SaleOrderID,
			ConsumerName:        entryRow.ConsumerName,
			OrderPlacedDate:     database.DateValue(entryRow.OrderPlacedDate).Format(time.DateOnly),
			OrderStatus:         entryRow.OrderStatus,
			DeliveredDate:       formatNullableDate(database.DatePointer(entryRow.DeliveredDate)),
			SaleOrderCategoryID: entryRow.SaleOrderCategoryID,
			CategoryName:        entryRow.CategoryName,
			TotalAmount:         entryRow.TotalAmount,
			ItemCount:           entryRow.ItemCount,
		})
	}
	context.JSON(http.StatusOK, gin.H{"sale_order_entries": entries})
}

func (server *Server) handleGetSaleOrderEntry(context *gin.Context) {
	entryID, ok := parseUUIDParam(context, "saleOrderEntryID")
	if !ok {
		return
	}
	server.respondWithSaleOrderEntry(context, entryID, http.StatusOK)
}

// respondWithSaleOrderEntry returns an entry and every item in it, with the
// total recomputed from the items (BR-19) rather than read from a column.
func (server *Server) respondWithSaleOrderEntry(
	context *gin.Context,
	entryID uuid.UUID,
	statusCode int,
) {
	entryRow, err := server.queries.GetSaleOrderEntry(context, entryID)
	if err != nil {
		server.respondDatabaseError(context, err, "sale order entry")
		return
	}
	itemRows, err := server.queries.ListSaleOrderItemsForEntry(context, entryID)
	if err != nil {
		server.respondDatabaseError(context, err, "sale order items")
		return
	}

	items := make([]saleOrderItemResponse, 0, len(itemRows))
	for _, itemRow := range itemRows {
		items = append(items, saleOrderItemResponse{
			SaleOrderItemID:  itemRow.SaleOrderItemID,
			SaleOrderEntryID: itemRow.SaleOrderEntryID,
			ProductName:      itemRow.ProductName,
			Quantity:         itemRow.Quantity,
			PricePerUnit:     itemRow.PricePerUnit,
			LineTotal:        itemRow.PricePerUnit.Mul(decimal.NewFromInt32(itemRow.Quantity)),
		})
	}

	context.JSON(statusCode, saleOrderEntryResponse{
		SaleOrderEntryID:    entryRow.SaleOrderEntryID,
		SaleOrderID:         entryRow.SaleOrderID,
		ConsumerName:        entryRow.ConsumerName,
		OrderPlacedDate:     database.DateValue(entryRow.OrderPlacedDate).Format(time.DateOnly),
		OrderStatus:         entryRow.OrderStatus,
		DeliveredDate:       formatNullableDate(database.DatePointer(entryRow.DeliveredDate)),
		SaleOrderCategoryID: entryRow.SaleOrderCategoryID,
		CategoryName:        entryRow.CategoryName,
		TotalAmount:         entryRow.TotalAmount,
		ItemCount:           entryRow.ItemCount,
		Items:               items,
	})
}

// ── create and edit (FR-P6-1, FR-P6-2, FR-P6-8) ───────────────────────────

type saleOrderEntryRequest struct {
	ConsumerName        string     `json:"consumer_name"`
	OrderPlacedDate     string     `json:"order_placed_date"`
	OrderStatus         string     `json:"order_status"`
	DeliveredDate       string     `json:"delivered_date"`
	SaleOrderCategoryID *uuid.UUID `json:"sale_order_category_id"`
}

// resolvedSaleOrderEntry is what the request turns into once it has been
// checked and the category resolved.
type resolvedSaleOrderEntry struct {
	consumerName    string
	orderPlacedDate time.Time
	orderStatus     string
	deliveredDate   *time.Time
	categoryID      uuid.UUID
}

func (server *Server) resolveSaleOrderEntry(
	context *gin.Context,
	request saleOrderEntryRequest,
) (resolvedSaleOrderEntry, bool) {
	var resolved resolvedSaleOrderEntry

	resolved.consumerName = strings.TrimSpace(request.ConsumerName)
	if resolved.consumerName == "" {
		respondError(context, http.StatusBadRequest, "consumer_name is required")
		return resolved, false
	}

	placedDate, err := parseOrderedOn(request.OrderPlacedDate)
	if err != nil {
		respondError(context, http.StatusBadRequest,
			"order_placed_date must be a date like 2026-08-23")
		return resolved, false
	}
	resolved.orderPlacedDate = placedDate

	resolved.orderStatus = strings.TrimSpace(request.OrderStatus)
	if resolved.orderStatus == "" {
		resolved.orderStatus = "pending"
	}
	if !isValidSaleOrderStatus(resolved.orderStatus) {
		respondError(context, http.StatusBadRequest,
			"order_status must be one of "+strings.Join(saleOrderStatuses, ", "))
		return resolved, false
	}

	// BR-20: a delivery date belongs to a delivered order and nowhere else.
	// Moving an order back out of Delivered drops the date rather than
	// failing, because the alternative is an edit the user cannot complete —
	// they would have to clear the date and change the status in one step
	// while the form shows neither field.
	if resolved.orderStatus == "delivered" && strings.TrimSpace(request.DeliveredDate) != "" {
		deliveredDate, err := time.Parse(time.DateOnly, strings.TrimSpace(request.DeliveredDate))
		if err != nil {
			respondError(context, http.StatusBadRequest,
				"delivered_date must be a date like 2026-08-23")
			return resolved, false
		}
		if deliveredDate.Before(placedDate) {
			respondError(context, http.StatusBadRequest,
				"delivered_date cannot be before order_placed_date")
			return resolved, false
		}
		resolved.deliveredDate = &deliveredDate
	}

	// BR-22 makes the category mandatory, so an omitted one falls back to the
	// seeded "Uncategorized" rather than rejecting the sale. Recording the
	// sale matters more than filing it, and it can be filed later.
	if request.SaleOrderCategoryID != nil {
		resolved.categoryID = *request.SaleOrderCategoryID
	} else {
		fallbackCategory, err := server.queries.GetSaleOrderCategoryByName(context,
			uncategorizedSaleOrderCategoryName)
		if err != nil {
			server.respondDatabaseError(context, err, "sale order category")
			return resolved, false
		}
		resolved.categoryID = fallbackCategory.SaleOrderCategoryID
	}

	return resolved, true
}

// saleOrderIDAttempts bounds the search for a free display number (BR-21).
// The range holds 999,000 values against a few hundred sales, so a collision
// is already unlikely; ten tries makes exhausting them impossible in practice
// while keeping the loop from spinning if something is badly wrong.
const saleOrderIDAttempts = 10

// nextSaleOrderID picks an unused four-to-six-digit number (BR-21). The check
// races with a concurrent insert, so the unique index is the real guarantee
// and the caller retries when it fires — this loop only keeps that retry rare.
func (server *Server) nextSaleOrderID(context *gin.Context) (int32, error) {
	for range saleOrderIDAttempts {
		candidate := int32(rand.IntN(999999-1000+1) + 1000)
		isTaken, err := server.queries.SaleOrderIDExists(context, candidate)
		if err != nil {
			return 0, err
		}
		if !isTaken {
			return candidate, nil
		}
	}
	return 0, errors.New("could not find a free sale order number")
}

// isUniqueViolation reports whether an error is Postgres' 23505.
func isUniqueViolation(err error) bool {
	var pgError *pgconn.PgError
	return errors.As(err, &pgError) && pgError.Code == "23505"
}

func (server *Server) handleCreateSaleOrderEntry(context *gin.Context) {
	var request saleOrderEntryRequest
	if err := context.ShouldBindJSON(&request); err != nil {
		respondError(context, http.StatusBadRequest, "invalid request body")
		return
	}
	resolved, ok := server.resolveSaleOrderEntry(context, request)
	if !ok {
		return
	}

	// BR-21: retry on the unique index rather than trusting the pre-check,
	// which cannot be atomic against a concurrent insert.
	var entryRow database.SaleOrderEntry
	var createErr error
	for range saleOrderIDAttempts {
		saleOrderID, err := server.nextSaleOrderID(context)
		if err != nil {
			server.respondDatabaseError(context, err, "sale order entry")
			return
		}
		entryRow, createErr = server.queries.CreateSaleOrderEntry(context,
			database.CreateSaleOrderEntryParams{
				SaleOrderID:         saleOrderID,
				ConsumerName:        resolved.consumerName,
				OrderPlacedDate:     database.Date(resolved.orderPlacedDate),
				OrderStatus:         resolved.orderStatus,
				DeliveredDate:       database.DateOrNull(resolved.deliveredDate),
				SaleOrderCategoryID: resolved.categoryID,
			})
		if createErr == nil {
			break
		}
		if !isUniqueViolation(createErr) {
			server.respondDatabaseError(context, createErr, "sale order entry")
			return
		}
	}
	if createErr != nil {
		server.respondDatabaseError(context, createErr, "sale order entry")
		return
	}

	server.respondWithSaleOrderEntry(context, entryRow.SaleOrderEntryID, http.StatusCreated)
}

func (server *Server) handleUpdateSaleOrderEntry(context *gin.Context) {
	entryID, ok := parseUUIDParam(context, "saleOrderEntryID")
	if !ok {
		return
	}
	var request saleOrderEntryRequest
	if err := context.ShouldBindJSON(&request); err != nil {
		respondError(context, http.StatusBadRequest, "invalid request body")
		return
	}
	resolved, ok := server.resolveSaleOrderEntry(context, request)
	if !ok {
		return
	}

	if _, err := server.queries.UpdateSaleOrderEntry(context,
		database.UpdateSaleOrderEntryParams{
			SaleOrderEntryID:    entryID,
			ConsumerName:        resolved.consumerName,
			OrderPlacedDate:     database.Date(resolved.orderPlacedDate),
			OrderStatus:         resolved.orderStatus,
			DeliveredDate:       database.DateOrNull(resolved.deliveredDate),
			SaleOrderCategoryID: resolved.categoryID,
		}); err != nil {
		server.respondDatabaseError(context, err, "sale order entry")
		return
	}
	server.respondWithSaleOrderEntry(context, entryID, http.StatusOK)
}

func (server *Server) handleDeleteSaleOrderEntry(context *gin.Context) {
	entryID, ok := parseUUIDParam(context, "saleOrderEntryID")
	if !ok {
		return
	}
	if err := server.queries.DeleteSaleOrderEntry(context, entryID); err != nil {
		server.respondDatabaseError(context, err, "sale order entry")
		return
	}
	context.Status(http.StatusNoContent)
}

// ── items (FR-P6-3, FR-P6-9) ──────────────────────────────────────────────

type saleOrderItemRequest struct {
	ProductName  string           `json:"product_name"`
	Quantity     int32            `json:"quantity"`
	PricePerUnit *decimal.Decimal `json:"price_per_unit"`
}

// validateSaleOrderItem applies the rules that would otherwise surface as
// constraint violations, so the client gets a readable 400.
func validateSaleOrderItem(context *gin.Context, request *saleOrderItemRequest) bool {
	request.ProductName = strings.TrimSpace(request.ProductName)
	if request.ProductName == "" {
		respondError(context, http.StatusBadRequest, "product_name is required")
		return false
	}
	if request.Quantity <= 0 {
		respondError(context, http.StatusBadRequest, "quantity must be greater than zero")
		return false
	}
	if request.PricePerUnit == nil || request.PricePerUnit.IsNegative() {
		respondError(context, http.StatusBadRequest, "price_per_unit must be zero or more")
		return false
	}
	return true
}

func (server *Server) handleCreateSaleOrderItem(context *gin.Context) {
	entryID, ok := parseUUIDParam(context, "saleOrderEntryID")
	if !ok {
		return
	}
	var request saleOrderItemRequest
	if err := context.ShouldBindJSON(&request); err != nil {
		respondError(context, http.StatusBadRequest, "invalid request body")
		return
	}
	if !validateSaleOrderItem(context, &request) {
		return
	}

	if _, err := server.queries.CreateSaleOrderItem(context,
		database.CreateSaleOrderItemParams{
			SaleOrderEntryID: entryID,
			ProductName:      request.ProductName,
			Quantity:         request.Quantity,
			PricePerUnit:     *request.PricePerUnit,
		}); err != nil {
		server.respondDatabaseError(context, err, "sale order item")
		return
	}
	// The whole entry comes back, because adding an item moves the total
	// (BR-19) and the client should never have to recompute it.
	server.respondWithSaleOrderEntry(context, entryID, http.StatusCreated)
}

func (server *Server) handleUpdateSaleOrderItem(context *gin.Context) {
	itemID, ok := parseUUIDParam(context, "saleOrderItemID")
	if !ok {
		return
	}
	var request saleOrderItemRequest
	if err := context.ShouldBindJSON(&request); err != nil {
		respondError(context, http.StatusBadRequest, "invalid request body")
		return
	}
	if !validateSaleOrderItem(context, &request) {
		return
	}

	existingItem, err := server.queries.GetSaleOrderItem(context, itemID)
	if err != nil {
		server.respondDatabaseError(context, err, "sale order item")
		return
	}
	if _, err := server.queries.UpdateSaleOrderItem(context,
		database.UpdateSaleOrderItemParams{
			SaleOrderItemID: itemID,
			ProductName:     request.ProductName,
			Quantity:        request.Quantity,
			PricePerUnit:    *request.PricePerUnit,
		}); err != nil {
		server.respondDatabaseError(context, err, "sale order item")
		return
	}
	server.respondWithSaleOrderEntry(context, existingItem.SaleOrderEntryID, http.StatusOK)
}

func (server *Server) handleDeleteSaleOrderItem(context *gin.Context) {
	itemID, ok := parseUUIDParam(context, "saleOrderItemID")
	if !ok {
		return
	}
	existingItem, err := server.queries.GetSaleOrderItem(context, itemID)
	if err != nil {
		server.respondDatabaseError(context, err, "sale order item")
		return
	}
	if err := server.queries.DeleteSaleOrderItem(context, itemID); err != nil {
		server.respondDatabaseError(context, err, "sale order item")
		return
	}
	server.respondWithSaleOrderEntry(context, existingItem.SaleOrderEntryID, http.StatusOK)
}

// ── categories (FR-P5-4, BR-23) ───────────────────────────────────────────

type saleOrderCategoryResponse struct {
	SaleOrderCategoryID uuid.UUID `json:"sale_order_category_id"`
	CategoryName        string    `json:"category_name"`
	IsSystem            bool      `json:"is_system"`
	UsageCount          int64     `json:"usage_count"`
}

func (server *Server) handleListSaleOrderCategories(context *gin.Context) {
	categoryRows, err := server.queries.ListSaleOrderCategories(context)
	if err != nil {
		server.respondDatabaseError(context, err, "sale order categories")
		return
	}
	categories := make([]saleOrderCategoryResponse, 0, len(categoryRows))
	for _, categoryRow := range categoryRows {
		categories = append(categories, saleOrderCategoryResponse{
			SaleOrderCategoryID: categoryRow.SaleOrderCategoryID,
			CategoryName:        categoryRow.CategoryName,
			IsSystem:            categoryRow.IsSystem,
			UsageCount:          categoryRow.UsageCount,
		})
	}
	context.JSON(http.StatusOK, gin.H{"sale_order_categories": categories})
}

func (server *Server) handleCreateSaleOrderCategory(context *gin.Context) {
	categoryName, ok := readName(context)
	if !ok {
		return
	}
	categoryRow, err := server.queries.CreateSaleOrderCategory(context, categoryName)
	if err != nil {
		server.respondDatabaseError(context, err, "sale order category")
		return
	}
	context.JSON(http.StatusCreated, saleOrderCategoryResponse{
		SaleOrderCategoryID: categoryRow.SaleOrderCategoryID,
		CategoryName:        categoryRow.CategoryName,
		IsSystem:            categoryRow.IsSystem,
	})
}

func (server *Server) handleRenameSaleOrderCategory(context *gin.Context) {
	categoryID, ok := parseUUIDParam(context, "saleOrderCategoryID")
	if !ok {
		return
	}
	categoryName, ok := readName(context)
	if !ok {
		return
	}
	categoryRow, err := server.queries.RenameSaleOrderCategory(context,
		database.RenameSaleOrderCategoryParams{
			SaleOrderCategoryID: categoryID,
			CategoryName:        categoryName,
		})
	if err != nil {
		server.respondDatabaseError(context, err, "sale order category")
		return
	}
	context.JSON(http.StatusOK, saleOrderCategoryResponse{
		SaleOrderCategoryID: categoryRow.SaleOrderCategoryID,
		CategoryName:        categoryRow.CategoryName,
		IsSystem:            categoryRow.IsSystem,
	})
}

// BR-23, the sell-side twin of BR-13: deleting a category is never blocked.
// Every sale filed under it moves to "Uncategorized" first, inside one
// transaction, so no sale is left pointing at a row that is about to vanish.
func (server *Server) handleDeleteSaleOrderCategory(context *gin.Context) {
	categoryID, ok := parseUUIDParam(context, "saleOrderCategoryID")
	if !ok {
		return
	}

	transaction, err := server.pool.Begin(context)
	if err != nil {
		server.respondDatabaseError(context, err, "sale order category")
		return
	}
	defer transaction.Rollback(context)
	transactionalQueries := server.queries.WithTx(transaction)

	if err := transactionalQueries.ReassignSaleOrdersToUncategorized(context,
		categoryID); err != nil {
		server.respondDatabaseError(context, err, "sale order category")
		return
	}
	if err := transactionalQueries.DeleteSaleOrderCategory(context, categoryID); err != nil {
		// The trigger guarding "Uncategorized" surfaces here.
		respondError(context, http.StatusConflict,
			"this category cannot be deleted: "+err.Error())
		return
	}
	if err := transaction.Commit(context); err != nil {
		server.respondDatabaseError(context, err, "sale order category")
		return
	}
	context.Status(http.StatusNoContent)
}
