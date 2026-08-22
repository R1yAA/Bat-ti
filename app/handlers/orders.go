package handlers

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/R1yAA/Bat-ti/app/database"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"
)

// P3 — order entries and the items inside them.

// Order item statuses (BR-10). Kept here as well as in the database check
// constraint so a bad value is a 400 with a readable message rather than a
// constraint violation surfacing as a 500.
var validOrderStatuses = map[string]bool{
	"placed":             true,
	"cancelled":          true,
	"refunded":           true,
	"partially_refunded": true,
}

// statusRequiresRefundAmount is the other half of BR-10.
func statusRequiresRefundAmount(orderStatus string) bool {
	return orderStatus == "refunded" || orderStatus == "partially_refunded"
}

type orderEntryResponse struct {
	OrderEntryID uuid.UUID           `json:"order_entry_id"`
	EntryName    *string             `json:"entry_name"`
	OrderedOn    string              `json:"ordered_on"`
	TotalCost    decimal.Decimal     `json:"total_cost"`
	ItemCount    int64               `json:"item_count"`
	Items        []orderItemResponse `json:"items,omitempty"`
}

type orderItemResponse struct {
	OrderItemID     uuid.UUID             `json:"order_item_id"`
	OrderEntryID    uuid.UUID             `json:"order_entry_id"`
	VendorListingID *uuid.UUID            `json:"vendor_listing_id"`
	VariantID       *uuid.UUID            `json:"variant_id"`
	VendorID        uuid.UUID             `json:"vendor_id"`
	VendorName      string                `json:"vendor_name"`
	ListingName     string                `json:"listing_name"`
	VariantLabel    *string               `json:"variant_label"`
	ProductURL      *string               `json:"product_url"`
	PrimaryImageURL *string               `json:"primary_image_url"`
	Quantity        int32                 `json:"quantity"`
	PricePerUnit    decimal.Decimal       `json:"price_per_unit"`
	LineTotal       decimal.Decimal       `json:"line_total"`
	OrderStatus     string                `json:"order_status"`
	RefundAmount    *decimal.Decimal      `json:"refund_amount"`
	Rating          *int                  `json:"rating"`
	CategoryTags    []categoryResponse    `json:"category_tags"`
	OccasionTags    []occasionTagResponse `json:"occasion_tags"`
}

func (server *Server) handleListOrderEntries(context *gin.Context) {
	entryRows, err := server.queries.ListOrderEntries(context)
	if err != nil {
		server.respondDatabaseError(context, err, "order entries")
		return
	}
	entries := make([]orderEntryResponse, 0, len(entryRows))
	for _, entryRow := range entryRows {
		entries = append(entries, orderEntryResponse{
			OrderEntryID: entryRow.OrderEntryID,
			EntryName:    database.TextValue(entryRow.EntryName),
			OrderedOn:    database.DateValue(entryRow.OrderedOn).Format(time.DateOnly),
			TotalCost:    entryRow.TotalCost,
			ItemCount:    entryRow.ItemCount,
		})
	}
	context.JSON(http.StatusOK, gin.H{"order_entries": entries})
}

type orderEntryRequest struct {
	EntryName string `json:"entry_name"`
	OrderedOn string `json:"ordered_on"`
}

// parseOrderedOn reads the date the order was actually placed. It defaults to
// today, because logging an order the same day is the common case — but it is
// editable, because logging one a fortnight late is the case that would
// otherwise put the spend in the wrong month.
func parseOrderedOn(rawDate string) (time.Time, error) {
	if strings.TrimSpace(rawDate) == "" {
		return time.Now(), nil
	}
	parsedDate, err := time.Parse(time.DateOnly, rawDate)
	if err != nil {
		return time.Time{}, fmt.Errorf("ordered_on must be a date like 2026-08-22")
	}
	return parsedDate, nil
}

func (server *Server) handleCreateOrderEntry(context *gin.Context) {
	var request orderEntryRequest
	if err := context.ShouldBindJSON(&request); err != nil {
		respondError(context, http.StatusBadRequest, "invalid request body")
		return
	}
	orderedOn, err := parseOrderedOn(request.OrderedOn)
	if err != nil {
		respondError(context, http.StatusBadRequest, err.Error())
		return
	}

	entryRow, err := server.queries.CreateOrderEntry(context, database.CreateOrderEntryParams{
		EntryName: database.TextOrNull(strings.TrimSpace(request.EntryName)),
		OrderedOn: database.Date(orderedOn),
	})
	if err != nil {
		server.respondDatabaseError(context, err, "order entry")
		return
	}
	context.JSON(http.StatusCreated, orderEntryResponse{
		OrderEntryID: entryRow.OrderEntryID,
		EntryName:    database.TextValue(entryRow.EntryName),
		OrderedOn:    database.DateValue(entryRow.OrderedOn).Format(time.DateOnly),
		TotalCost:    decimal.Zero,
	})
}

func (server *Server) handleUpdateOrderEntry(context *gin.Context) {
	entryID, ok := parseUUIDParam(context, "orderEntryID")
	if !ok {
		return
	}
	var request orderEntryRequest
	if err := context.ShouldBindJSON(&request); err != nil {
		respondError(context, http.StatusBadRequest, "invalid request body")
		return
	}
	orderedOn, err := parseOrderedOn(request.OrderedOn)
	if err != nil {
		respondError(context, http.StatusBadRequest, err.Error())
		return
	}

	if _, err := server.queries.UpdateOrderEntry(context, database.UpdateOrderEntryParams{
		OrderEntryID: entryID,
		EntryName:    database.TextOrNull(strings.TrimSpace(request.EntryName)),
		OrderedOn:    database.Date(orderedOn),
	}); err != nil {
		server.respondDatabaseError(context, err, "order entry")
		return
	}
	server.respondWithOrderEntry(context, entryID, http.StatusOK)
}

func (server *Server) handleDeleteOrderEntry(context *gin.Context) {
	entryID, ok := parseUUIDParam(context, "orderEntryID")
	if !ok {
		return
	}
	if err := server.queries.DeleteOrderEntry(context, entryID); err != nil {
		server.respondDatabaseError(context, err, "order entry")
		return
	}
	context.Status(http.StatusNoContent)
}

func (server *Server) handleGetOrderEntry(context *gin.Context) {
	entryID, ok := parseUUIDParam(context, "orderEntryID")
	if !ok {
		return
	}
	server.respondWithOrderEntry(context, entryID, http.StatusOK)
}

// respondWithOrderEntry returns an entry and every item in it, with the total
// recomputed from the items (BR-9) rather than read from a stored column.
func (server *Server) respondWithOrderEntry(
	context *gin.Context,
	entryID uuid.UUID,
	statusCode int,
) {
	entryRow, err := server.queries.GetOrderEntry(context, entryID)
	if err != nil {
		server.respondDatabaseError(context, err, "order entry")
		return
	}
	itemRows, err := server.queries.ListOrderItemsForEntry(context, entryID)
	if err != nil {
		server.respondDatabaseError(context, err, "order items")
		return
	}

	items := make([]orderItemResponse, 0, len(itemRows))
	for _, itemRow := range itemRows {
		item := orderItemResponse{
			OrderItemID:     itemRow.OrderItemID,
			OrderEntryID:    itemRow.OrderEntryID,
			VendorListingID: database.UUIDValue(itemRow.VendorListingID),
			VariantID:       database.UUIDValue(itemRow.VariantID),
			VendorID:        itemRow.VendorID,
			VendorName:      itemRow.VendorName,
			ListingName:     itemRow.ListingNameSnapshot,
			VariantLabel:    database.TextValue(itemRow.VariantLabel),
			ProductURL:      database.TextValue(itemRow.ProductUrl),
			PrimaryImageURL: database.TextValue(itemRow.PrimaryImageUrl),
			Quantity:        itemRow.Quantity,
			PricePerUnit:    itemRow.PricePerUnit,
			LineTotal:       itemRow.PricePerUnit.Mul(decimal.NewFromInt32(itemRow.Quantity)),
			OrderStatus:     itemRow.OrderStatus,
			RefundAmount:    database.DecimalValue(itemRow.RefundAmount),
			Rating:          database.IntValue(itemRow.Rating),
			CategoryTags:    []categoryResponse{},
			OccasionTags:    []occasionTagResponse{},
		}

		categoryRows, err := server.queries.ListCategoryTagsForOrderItem(context, itemRow.OrderItemID)
		if err != nil {
			server.respondDatabaseError(context, err, "category tags")
			return
		}
		for _, categoryRow := range categoryRows {
			item.CategoryTags = append(item.CategoryTags, categoryResponse{
				CategoryID:   categoryRow.CategoryID,
				CategoryName: categoryRow.CategoryName,
				IsSystem:     categoryRow.IsSystem,
			})
		}

		occasionRows, err := server.queries.ListOccasionTagsForOrderItem(context, itemRow.OrderItemID)
		if err != nil {
			server.respondDatabaseError(context, err, "occasion tags")
			return
		}
		for _, occasionRow := range occasionRows {
			item.OccasionTags = append(item.OccasionTags, occasionTagResponse{
				OccasionTagID: occasionRow.OccasionTagID,
				TagName:       occasionRow.TagName,
			})
		}

		items = append(items, item)
	}

	context.JSON(statusCode, orderEntryResponse{
		OrderEntryID: entryRow.OrderEntryID,
		EntryName:    database.TextValue(entryRow.EntryName),
		OrderedOn:    database.DateValue(entryRow.OrderedOn).Format(time.DateOnly),
		TotalCost:    entryRow.TotalCost,
		ItemCount:    entryRow.ItemCount,
		Items:        items,
	})
}

type orderItemRequest struct {
	VendorListingID *uuid.UUID       `json:"vendor_listing_id"`
	VariantID       *uuid.UUID       `json:"variant_id"`
	VendorID        *uuid.UUID       `json:"vendor_id"`
	ListingName     string           `json:"listing_name"`
	Quantity        int32            `json:"quantity"`
	PricePerUnit    *decimal.Decimal `json:"price_per_unit"`
	OrderStatus     string           `json:"order_status"`
	RefundAmount    *decimal.Decimal `json:"refund_amount"`
	Rating          *int             `json:"rating"`
	CategoryIDs     []uuid.UUID      `json:"category_ids"`
	OccasionTagIDs  []uuid.UUID      `json:"occasion_tag_ids"`
}

// resolvedOrderItem is what the request turns into once the referenced listing
// or variant has been looked up.
type resolvedOrderItem struct {
	vendorListingID uuid.NullUUID
	variantID       uuid.NullUUID
	vendorID        uuid.UUID
	listingName     string
}

// resolve fills in the vendor and the listing name from whatever the request
// pointed at. BR-6 keeps the vendor on the item, but deriving it from the
// listing rather than trusting the client removes a whole class of mismatch
// where an item claims one vendor and its listing belongs to another.
func (server *Server) resolve(
	context *gin.Context,
	request orderItemRequest,
) (resolvedOrderItem, bool) {
	var resolved resolvedOrderItem

	switch {
	case request.VariantID != nil:
		variantRow, err := server.queries.GetVariantByID(context, *request.VariantID)
		if err != nil {
			server.respondDatabaseError(context, err, "variant")
			return resolved, false
		}
		listingRow, err := server.queries.GetVendorListingByID(context, variantRow.VendorListingID)
		if err != nil {
			server.respondDatabaseError(context, err, "listing")
			return resolved, false
		}
		resolved.variantID = database.NullUUID(variantRow.VariantID)
		resolved.vendorListingID = database.NullUUID(listingRow.VendorListingID)
		resolved.vendorID = listingRow.VendorID
		resolved.listingName = listingRow.ListingName + " — " + variantRow.VariantLabel

	case request.VendorListingID != nil:
		listingRow, err := server.queries.GetVendorListingByID(context, *request.VendorListingID)
		if err != nil {
			server.respondDatabaseError(context, err, "listing")
			return resolved, false
		}
		resolved.vendorListingID = database.NullUUID(listingRow.VendorListingID)
		resolved.vendorID = listingRow.VendorID
		resolved.listingName = listingRow.ListingName

	default:
		// A purchase from a vendor whose catalogue is not tracked still needs
		// to be recordable, so the vendor and a name may be given directly.
		if request.VendorID == nil || strings.TrimSpace(request.ListingName) == "" {
			respondError(context, http.StatusBadRequest,
				"send vendor_listing_id or variant_id, or else vendor_id together with listing_name")
			return resolved, false
		}
		resolved.vendorID = *request.VendorID
		resolved.listingName = strings.TrimSpace(request.ListingName)
	}

	// An explicit name always wins, so a past order can keep reading the way it
	// did even after the vendor renames the product.
	if trimmedName := strings.TrimSpace(request.ListingName); trimmedName != "" {
		resolved.listingName = trimmedName
	}
	return resolved, true
}

// validate applies the rules that would otherwise surface as database
// constraint violations, so the client gets a readable 400.
func validateOrderItem(context *gin.Context, request *orderItemRequest) bool {
	if request.Quantity <= 0 {
		respondError(context, http.StatusBadRequest, "quantity must be greater than zero")
		return false
	}
	if request.PricePerUnit == nil || request.PricePerUnit.IsNegative() {
		respondError(context, http.StatusBadRequest, "price_per_unit must be zero or more")
		return false
	}

	if request.OrderStatus == "" {
		request.OrderStatus = "placed"
	}
	if !validOrderStatuses[request.OrderStatus] {
		respondError(context, http.StatusBadRequest,
			"order_status must be placed, cancelled, refunded or partially_refunded")
		return false
	}

	// BR-10, both directions.
	if statusRequiresRefundAmount(request.OrderStatus) && request.RefundAmount == nil {
		respondError(context, http.StatusBadRequest,
			"refund_amount is required when order_status is "+request.OrderStatus)
		return false
	}
	if !statusRequiresRefundAmount(request.OrderStatus) && request.RefundAmount != nil {
		respondError(context, http.StatusBadRequest,
			"refund_amount does not apply when order_status is "+request.OrderStatus)
		return false
	}
	if request.RefundAmount != nil && request.RefundAmount.IsNegative() {
		respondError(context, http.StatusBadRequest, "refund_amount cannot be negative")
		return false
	}

	// BR-8: the scale is 1 to 10 everywhere.
	if request.Rating != nil && (*request.Rating < 1 || *request.Rating > 10) {
		respondError(context, http.StatusBadRequest, "rating must be between 1 and 10")
		return false
	}
	return true
}

func (server *Server) handleCreateOrderItem(context *gin.Context) {
	entryID, ok := parseUUIDParam(context, "orderEntryID")
	if !ok {
		return
	}
	var request orderItemRequest
	if err := context.ShouldBindJSON(&request); err != nil {
		respondError(context, http.StatusBadRequest, "invalid request body")
		return
	}
	if !validateOrderItem(context, &request) {
		return
	}
	resolved, ok := server.resolve(context, request)
	if !ok {
		return
	}

	transaction, err := server.pool.Begin(context)
	if err != nil {
		server.respondDatabaseError(context, err, "order item")
		return
	}
	defer transaction.Rollback(context)
	transactionalQueries := server.queries.WithTx(transaction)

	itemRow, err := transactionalQueries.CreateOrderItem(context, database.CreateOrderItemParams{
		OrderEntryID:        entryID,
		VendorListingID:     resolved.vendorListingID,
		VariantID:           resolved.variantID,
		VendorID:            resolved.vendorID,
		ListingNameSnapshot: resolved.listingName,
		Quantity:            request.Quantity,
		PricePerUnit:        *request.PricePerUnit,
		OrderStatus:         request.OrderStatus,
		RefundAmount:        database.DecimalOrNull(request.RefundAmount),
		Rating:              database.Int4OrNull(request.Rating),
	})
	if err != nil {
		server.respondDatabaseError(context, err, "order item")
		return
	}
	if !server.replaceTags(context, transactionalQueries, itemRow.OrderItemID, request) {
		return
	}
	if err := transaction.Commit(context); err != nil {
		server.respondDatabaseError(context, err, "order item")
		return
	}

	server.respondWithOrderEntry(context, entryID, http.StatusCreated)
}

func (server *Server) handleUpdateOrderItem(context *gin.Context) {
	itemID, ok := parseUUIDParam(context, "orderItemID")
	if !ok {
		return
	}
	var request orderItemRequest
	if err := context.ShouldBindJSON(&request); err != nil {
		respondError(context, http.StatusBadRequest, "invalid request body")
		return
	}
	if !validateOrderItem(context, &request) {
		return
	}

	existingItem, err := server.queries.GetOrderItem(context, itemID)
	if err != nil {
		server.respondDatabaseError(context, err, "order item")
		return
	}
	resolved, ok := server.resolve(context, request)
	if !ok {
		return
	}

	transaction, err := server.pool.Begin(context)
	if err != nil {
		server.respondDatabaseError(context, err, "order item")
		return
	}
	defer transaction.Rollback(context)
	transactionalQueries := server.queries.WithTx(transaction)

	if _, err := transactionalQueries.UpdateOrderItem(context, database.UpdateOrderItemParams{
		OrderItemID:         itemID,
		VendorListingID:     resolved.vendorListingID,
		VariantID:           resolved.variantID,
		VendorID:            resolved.vendorID,
		ListingNameSnapshot: resolved.listingName,
		Quantity:            request.Quantity,
		PricePerUnit:        *request.PricePerUnit,
		OrderStatus:         request.OrderStatus,
		RefundAmount:        database.DecimalOrNull(request.RefundAmount),
		Rating:              database.Int4OrNull(request.Rating),
	}); err != nil {
		server.respondDatabaseError(context, err, "order item")
		return
	}
	if !server.replaceTags(context, transactionalQueries, itemID, request) {
		return
	}
	if err := transaction.Commit(context); err != nil {
		server.respondDatabaseError(context, err, "order item")
		return
	}

	server.respondWithOrderEntry(context, existingItem.OrderEntryID, http.StatusOK)
}

func (server *Server) handleDeleteOrderItem(context *gin.Context) {
	itemID, ok := parseUUIDParam(context, "orderItemID")
	if !ok {
		return
	}
	existingItem, err := server.queries.GetOrderItem(context, itemID)
	if err != nil {
		server.respondDatabaseError(context, err, "order item")
		return
	}
	if err := server.queries.DeleteOrderItem(context, itemID); err != nil {
		server.respondDatabaseError(context, err, "order item")
		return
	}
	server.respondWithOrderEntry(context, existingItem.OrderEntryID, http.StatusOK)
}

// replaceTags sets both tag lists to exactly what the request asked for. The
// two kinds stay separate throughout (FR-P3-9): a category is for spend
// reporting, an occasion tag is free-form, and they are never merged.
func (server *Server) replaceTags(
	context *gin.Context,
	transactionalQueries *database.Queries,
	orderItemID uuid.UUID,
	request orderItemRequest,
) bool {
	if err := transactionalQueries.ClearOrderItemCategoryTags(context, orderItemID); err != nil {
		server.respondDatabaseError(context, err, "category tags")
		return false
	}
	for _, categoryID := range request.CategoryIDs {
		if err := transactionalQueries.AddOrderItemCategoryTag(context,
			database.AddOrderItemCategoryTagParams{
				OrderItemID: orderItemID,
				CategoryID:  categoryID,
			}); err != nil {
			if err == pgx.ErrNoRows {
				continue
			}
			respondError(context, http.StatusBadRequest, "unknown category id "+categoryID.String())
			return false
		}
	}

	if err := transactionalQueries.ClearOrderItemOccasionTags(context, orderItemID); err != nil {
		server.respondDatabaseError(context, err, "occasion tags")
		return false
	}
	for _, occasionTagID := range request.OccasionTagIDs {
		if err := transactionalQueries.AddOrderItemOccasionTag(context,
			database.AddOrderItemOccasionTagParams{
				OrderItemID:   orderItemID,
				OccasionTagID: occasionTagID,
			}); err != nil {
			respondError(context, http.StatusBadRequest, "unknown occasion tag id "+occasionTagID.String())
			return false
		}
	}
	return true
}
