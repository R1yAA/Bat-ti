package handlers

import (
	"net/http"
	"strings"
	"time"

	"github.com/R1yAA/Bat-ti/app/database"
	"github.com/R1yAA/Bat-ti/app/money"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// P2 — compare entries.

type compareEntryResponse struct {
	CompareEntryID uuid.UUID  `json:"compare_entry_id"`
	EntryName      string     `json:"entry_name"`
	MemberCount    int64      `json:"member_count"`
	CreatedAt      *time.Time `json:"created_at"`
}

func (server *Server) handleListCompareEntries(context *gin.Context) {
	entryRows, err := server.queries.ListCompareEntries(context)
	if err != nil {
		server.respondDatabaseError(context, err, "compare entries")
		return
	}
	entries := make([]compareEntryResponse, 0, len(entryRows))
	for _, entryRow := range entryRows {
		entries = append(entries, compareEntryResponse{
			CompareEntryID: entryRow.CompareEntryID,
			EntryName:      entryRow.EntryName,
			MemberCount:    entryRow.MemberCount,
			CreatedAt:      database.TimeValue(entryRow.CreatedAt),
		})
	}
	context.JSON(http.StatusOK, gin.H{"compare_entries": entries})
}

type compareEntryNameRequest struct {
	EntryName string `json:"entry_name"`
}

// FR-P2-1: a name is the only thing required to create one.
func (server *Server) handleCreateCompareEntry(context *gin.Context) {
	var request compareEntryNameRequest
	if err := context.ShouldBindJSON(&request); err != nil {
		respondError(context, http.StatusBadRequest, "entry_name is required")
		return
	}
	entryName := strings.TrimSpace(request.EntryName)
	if entryName == "" {
		respondError(context, http.StatusBadRequest, "entry_name cannot be blank")
		return
	}

	entryRow, err := server.queries.CreateCompareEntry(context, entryName)
	if err != nil {
		server.respondDatabaseError(context, err, "compare entry")
		return
	}
	context.JSON(http.StatusCreated, compareEntryResponse{
		CompareEntryID: entryRow.CompareEntryID,
		EntryName:      entryRow.EntryName,
		CreatedAt:      database.TimeValue(entryRow.CreatedAt),
	})
}

func (server *Server) handleRenameCompareEntry(context *gin.Context) {
	entryID, ok := parseUUIDParam(context, "entryID")
	if !ok {
		return
	}
	var request compareEntryNameRequest
	if err := context.ShouldBindJSON(&request); err != nil {
		respondError(context, http.StatusBadRequest, "entry_name is required")
		return
	}
	entryName := strings.TrimSpace(request.EntryName)
	if entryName == "" {
		respondError(context, http.StatusBadRequest, "entry_name cannot be blank")
		return
	}

	entryRow, err := server.queries.RenameCompareEntry(context, database.RenameCompareEntryParams{
		CompareEntryID: entryID,
		EntryName:      entryName,
	})
	if err != nil {
		server.respondDatabaseError(context, err, "compare entry")
		return
	}
	context.JSON(http.StatusOK, compareEntryResponse{
		CompareEntryID: entryRow.CompareEntryID,
		EntryName:      entryRow.EntryName,
		CreatedAt:      database.TimeValue(entryRow.CreatedAt),
	})
}

// BR-17: nothing in the system removes a compare entry. Only this does.
func (server *Server) handleDeleteCompareEntry(context *gin.Context) {
	entryID, ok := parseUUIDParam(context, "entryID")
	if !ok {
		return
	}
	if err := server.queries.DeleteCompareEntry(context, entryID); err != nil {
		server.respondDatabaseError(context, err, "compare entry")
		return
	}
	context.Status(http.StatusNoContent)
}

type compareMemberResponse struct {
	CompareEntryMemberID uuid.UUID            `json:"compare_entry_member_id"`
	VendorListingID      uuid.UUID            `json:"vendor_listing_id"`
	VariantID            *uuid.UUID           `json:"variant_id"`
	VendorName           string               `json:"vendor_name"`
	VendorSlug           string               `json:"vendor_slug"`
	ListingName          string               `json:"listing_name"`
	VariantLabel         *string              `json:"variant_label"`
	ProductURL           string               `json:"product_url"`
	PrimaryImageURL      *string              `json:"primary_image_url"`
	IsInStock            bool                 `json:"is_in_stock"`
	IsDelisted           bool                 `json:"is_delisted"`
	PackSize             *int                 `json:"pack_size"`
	CurrentPrice         *decimal.Decimal     `json:"current_price"`
	PricePerUnit         *decimal.Decimal     `json:"price_per_unit"`
	AverageRating        decimal.Decimal      `json:"average_rating"`
	RatingCount          int64                `json:"rating_count"`
	MoqTiers             []moqTierResponse    `json:"moq_tiers"`
	PriceHistory         []pricePointResponse `json:"price_history"`
	PastOrders           []pastOrderResponse  `json:"past_orders"`
}

// FR-P2-4: one column per member, with every comparable field the table needs.
// Past orders come down with the rest even though the UI collapses them by
// default — they are a handful of rows and a second round trip per column when
// the section is opened would be worse.
func (server *Server) handleGetCompareEntry(context *gin.Context) {
	entryID, ok := parseUUIDParam(context, "entryID")
	if !ok {
		return
	}

	entryRow, err := server.queries.GetCompareEntry(context, entryID)
	if err != nil {
		server.respondDatabaseError(context, err, "compare entry")
		return
	}
	memberRows, err := server.queries.ListCompareEntryMembers(context, entryID)
	if err != nil {
		server.respondDatabaseError(context, err, "compare entry members")
		return
	}

	members := make([]compareMemberResponse, 0, len(memberRows))
	for _, memberRow := range memberRows {
		member := compareMemberResponse{
			CompareEntryMemberID: memberRow.CompareEntryMemberID,
			VendorListingID:      memberRow.VendorListingID,
			VariantID:            database.UUIDValue(memberRow.VariantID),
			VendorName:           memberRow.VendorName,
			VendorSlug:           memberRow.VendorSlug,
			ListingName:          memberRow.ListingName,
			VariantLabel:         database.TextValue(memberRow.VariantLabel),
			ProductURL:           memberRow.ProductUrl,
			PrimaryImageURL:      database.TextValue(memberRow.PrimaryImageUrl),
			IsDelisted:           memberRow.IsDelisted,
			MoqTiers:             []moqTierResponse{},
			PriceHistory:         []pricePointResponse{},
			PastOrders:           []pastOrderResponse{},
		}

		// A member points at either a variant or the listing itself; the
		// figures shown must come from whichever it is.
		if memberRow.VariantID.Valid {
			member.IsInStock = memberRow.VariantIsInStock.Bool
			member.PackSize = database.IntValue(memberRow.VariantPackSize)
			member.CurrentPrice = database.DecimalValue(memberRow.VariantCurrentPrice)

			tierRows, err := server.queries.ListMoqTiersForVariant(context, memberRow.VariantID)
			if err != nil {
				server.respondDatabaseError(context, err, "moq tiers")
				return
			}
			member.MoqTiers = toMoqTierResponses(tierRows)

			historyRows, err := server.queries.ListPriceHistoryForVariant(context, memberRow.VariantID)
			if err != nil {
				server.respondDatabaseError(context, err, "price history")
				return
			}
			for _, historyRow := range historyRows {
				member.PriceHistory = append(member.PriceHistory, pricePointResponse{
					Date:  database.DateValue(historyRow.ScrapedAtDate).Format(time.DateOnly),
					Price: historyRow.Price,
				})
			}
		} else {
			member.IsInStock = memberRow.IsInStock
			member.PackSize = database.IntValue(memberRow.ListingPackSize)
			member.CurrentPrice = database.DecimalValue(memberRow.ListingCurrentPrice)

			listingKey := database.NullUUID(memberRow.VendorListingID)
			tierRows, err := server.queries.ListMoqTiersForListingWithFallback(
				context, listingKey)
			if err != nil {
				server.respondDatabaseError(context, err, "moq tiers")
				return
			}
			member.MoqTiers = toMoqTierResponses(tierRows)

			historyRows, err := server.queries.ListPriceHistoryForListing(context, listingKey)
			if err != nil {
				server.respondDatabaseError(context, err, "price history")
				return
			}
			for _, historyRow := range historyRows {
				member.PriceHistory = append(member.PriceHistory, pricePointResponse{
					Date:  database.DateValue(historyRow.ScrapedAtDate).Format(time.DateOnly),
					Price: historyRow.Price,
				})
			}
		}
		member.PricePerUnit = money.PerUnit(member.CurrentPrice, member.PackSize)

		// Rating and past orders are always scoped to the listing, because a
		// rating describes this vendor's version of the item (BR-8).
		listingKey := database.NullUUID(memberRow.VendorListingID)
		ratingRow, err := server.queries.GetListingAggregateRating(context, listingKey)
		if err != nil {
			server.respondDatabaseError(context, err, "rating")
			return
		}
		member.AverageRating = ratingRow.AverageRating
		member.RatingCount = ratingRow.RatingCount

		pastOrderRows, err := server.queries.ListOrderItemsForListing(context, listingKey)
		if err != nil {
			server.respondDatabaseError(context, err, "past orders")
			return
		}
		for _, pastOrderRow := range pastOrderRows {
			member.PastOrders = append(member.PastOrders, pastOrderResponse{
				OrderItemID:  pastOrderRow.OrderItemID,
				OrderEntryID: pastOrderRow.OrderEntryID,
				OrderedOn:    database.DateValue(pastOrderRow.OrderedOn).Format(time.DateOnly),
				EntryName:    database.TextValue(pastOrderRow.EntryName),
				VariantLabel: database.TextValue(pastOrderRow.VariantLabel),
				Quantity:     pastOrderRow.Quantity,
				PricePerUnit: pastOrderRow.PricePerUnit,
				OrderStatus:  pastOrderRow.OrderStatus,
				RefundAmount: database.DecimalValue(pastOrderRow.RefundAmount),
				Rating:       database.IntValue(pastOrderRow.Rating),
			})
		}

		members = append(members, member)
	}

	context.JSON(http.StatusOK, gin.H{
		"compare_entry": compareEntryResponse{
			CompareEntryID: entryRow.CompareEntryID,
			EntryName:      entryRow.EntryName,
			MemberCount:    int64(len(members)),
		},
		"members": members,
	})
}

type addCompareMemberRequest struct {
	VendorListingID *uuid.UUID `json:"vendor_listing_id"`
	VariantID       *uuid.UUID `json:"variant_id"`
}

// FR-P2-2, FR-P2-3: a listing may sit in as many entries as the user likes, so
// the only thing rejected here is naming both a listing and a variant, or
// neither.
func (server *Server) handleAddCompareEntryMember(context *gin.Context) {
	entryID, ok := parseUUIDParam(context, "entryID")
	if !ok {
		return
	}
	var request addCompareMemberRequest
	if err := context.ShouldBindJSON(&request); err != nil {
		respondError(context, http.StatusBadRequest, "send either vendor_listing_id or variant_id")
		return
	}
	if (request.VendorListingID == nil) == (request.VariantID == nil) {
		respondError(context, http.StatusBadRequest,
			"send exactly one of vendor_listing_id or variant_id")
		return
	}

	if request.VariantID != nil {
		if _, err := server.queries.AddCompareEntryVariantMember(context,
			database.AddCompareEntryVariantMemberParams{
				CompareEntryID: entryID,
				VariantID:      database.NullUUID(*request.VariantID),
			}); err != nil {
			server.respondDatabaseError(context, err, "compare entry member")
			return
		}
	} else {
		if _, err := server.queries.AddCompareEntryListingMember(context,
			database.AddCompareEntryListingMemberParams{
				CompareEntryID:  entryID,
				VendorListingID: database.NullUUID(*request.VendorListingID),
			}); err != nil {
			server.respondDatabaseError(context, err, "compare entry member")
			return
		}
	}
	context.Status(http.StatusNoContent)
}

func (server *Server) handleDeleteCompareEntryMember(context *gin.Context) {
	entryID, ok := parseUUIDParam(context, "entryID")
	if !ok {
		return
	}
	memberID, ok := parseUUIDParam(context, "memberID")
	if !ok {
		return
	}
	if err := server.queries.DeleteCompareEntryMember(context,
		database.DeleteCompareEntryMemberParams{
			CompareEntryID:       entryID,
			CompareEntryMemberID: memberID,
		}); err != nil {
		server.respondDatabaseError(context, err, "compare entry member")
		return
	}
	context.Status(http.StatusNoContent)
}
