package handlers

import (
	stdcontext "context"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/R1yAA/Bat-ti/app/database"
	"github.com/R1yAA/Bat-ti/app/money"
	"github.com/R1yAA/Bat-ti/app/scraper/runner"
	"github.com/R1yAA/Bat-ti/config"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// P1 — the vendor catalogue view and the listing content page.

type vendorResponse struct {
	VendorID               uuid.UUID  `json:"vendor_id"`
	VendorSlug             string     `json:"vendor_slug"`
	VendorName             string     `json:"vendor_name"`
	SourceBaseURL          string     `json:"source_base_url"`
	ScraperTier            string     `json:"scraper_tier"`
	LastSuccessfulScrapeAt *time.Time `json:"last_successful_scrape_at"`
	LastScrapeAttemptAt    *time.Time `json:"last_scrape_attempt_at"`
	LastScrapeError        *string    `json:"last_scrape_error"`
}

func toVendorResponse(vendorRow database.Vendor) vendorResponse {
	return vendorResponse{
		VendorID:               vendorRow.VendorID,
		VendorSlug:             vendorRow.VendorSlug,
		VendorName:             vendorRow.VendorName,
		SourceBaseURL:          vendorRow.SourceBaseUrl,
		ScraperTier:            vendorRow.ScraperTier,
		LastSuccessfulScrapeAt: database.TimeValue(vendorRow.LastSuccessfulScrapeTimestamp),
		LastScrapeAttemptAt:    database.TimeValue(vendorRow.LastScrapeAttemptTimestamp),
		LastScrapeError:        database.TextValue(vendorRow.LastScrapeError),
	}
}

// FR-X-3: the last successful scrape is shown wherever a vendor is chosen, so
// staleness is always visible.
func (server *Server) handleListVendors(context *gin.Context) {
	vendorRows, err := server.queries.ListVendors(context)
	if err != nil {
		server.respondDatabaseError(context, err, "vendors")
		return
	}

	vendorResponses := make([]vendorResponse, 0, len(vendorRows))
	for _, vendorRow := range vendorRows {
		vendorResponses = append(vendorResponses, toVendorResponse(vendorRow))
	}
	context.JSON(http.StatusOK, gin.H{"vendors": vendorResponses})
}

type listingSummaryResponse struct {
	VendorListingID    uuid.UUID            `json:"vendor_listing_id"`
	ListingName        string               `json:"listing_name"`
	ProductURL         string               `json:"product_url"`
	PrimaryImageURL    *string              `json:"primary_image_url"`
	IsInStock          bool                 `json:"is_in_stock"`
	IsDelisted         bool                 `json:"is_delisted"`
	IsTracked          bool                 `json:"is_tracked"`
	HasVariants        bool                 `json:"has_variants"`
	VariantCount       int64                `json:"variant_count"`
	PackSize           *int                 `json:"pack_size"`
	CurrentPrice       *decimal.Decimal     `json:"current_price"`
	PreviousPrice      *decimal.Decimal     `json:"previous_price"`
	PricePerUnit       *decimal.Decimal     `json:"price_per_unit"`
	PriceDirection     money.PriceDirection `json:"price_direction"`
	PriceLastChangedAt *time.Time           `json:"price_last_changed_at"`
	VendorSideCategory *string              `json:"vendor_side_category"`
}

func toListingSummary(
	listingRow database.VendorListing,
	variantCount int64,
) listingSummaryResponse {
	currentPrice := database.DecimalValue(listingRow.CurrentPrice)
	previousPrice := database.DecimalValue(listingRow.PreviousPrice)
	packSize := database.IntValue(listingRow.PackSize)

	return listingSummaryResponse{
		VendorListingID: listingRow.VendorListingID,
		ListingName:     listingRow.ListingName,
		ProductURL:      listingRow.ProductUrl,
		PrimaryImageURL: database.TextValue(listingRow.PrimaryImageUrl),
		IsInStock:       listingRow.IsInStock,
		IsDelisted:      listingRow.IsDelisted,
		IsTracked:       listingRow.IsTracked,
		HasVariants:     listingRow.HasVariants,
		VariantCount:    variantCount,
		PackSize:        packSize,
		CurrentPrice:    currentPrice,
		PreviousPrice:   previousPrice,
		// BR-5: the headline price large, this one small beneath it.
		PricePerUnit: money.PerUnit(currentPrice, packSize),
		// BR-16, decided here rather than in the client so every view that
		// shows an arrow agrees on what it means.
		PriceDirection:     money.DirectionOf(currentPrice, previousPrice),
		PriceLastChangedAt: database.TimeValue(listingRow.PriceLastChangedAt),
		VendorSideCategory: database.TextValue(listingRow.VendorSideCategory),
	}
}

// P1-A. Out-of-stock and delisted listings are returned like any other; the
// query parameters exist so the user can filter them out deliberately, never
// so the system hides them (BR-15).
func (server *Server) handleListVendorListings(context *gin.Context) {
	vendorRow, err := server.queries.GetVendorBySlug(context, context.Param("vendorSlug"))
	if err != nil {
		server.respondDatabaseError(context, err, "vendor")
		return
	}

	pageSize := parseIntQuery(context, "limit", 60, 1, 250)
	pageOffset := parseIntQuery(context, "offset", 0, 0, 1_000_000)
	inStockOnly := parseBoolQuery(context, "in_stock_only", false)
	includeDelisted := parseBoolQuery(context, "include_delisted", false)
	searchText := context.Query("search")

	totalCount, err := server.queries.CountVendorListings(context, database.CountVendorListingsParams{
		VendorID:        vendorRow.VendorID,
		InStockOnly:     inStockOnly,
		IncludeDelisted: includeDelisted,
		SearchText:      searchText,
	})
	if err != nil {
		server.respondDatabaseError(context, err, "listings")
		return
	}

	listingRows, err := server.queries.ListVendorListings(context, database.ListVendorListingsParams{
		VendorID:        vendorRow.VendorID,
		InStockOnly:     inStockOnly,
		IncludeDelisted: includeDelisted,
		SearchText:      searchText,
		ResultLimit:     int32(pageSize),
		ResultOffset:    int32(pageOffset),
	})
	if err != nil {
		server.respondDatabaseError(context, err, "listings")
		return
	}

	listingSummaries := make([]listingSummaryResponse, 0, len(listingRows))
	for _, listingRow := range listingRows {
		listingSummaries = append(listingSummaries, toListingSummary(database.VendorListing{
			VendorListingID:    listingRow.VendorListingID,
			VendorID:           listingRow.VendorID,
			ProductUrl:         listingRow.ProductUrl,
			ListingName:        listingRow.ListingName,
			PrimaryImageUrl:    listingRow.PrimaryImageUrl,
			VendorSideCategory: listingRow.VendorSideCategory,
			IsInStock:          listingRow.IsInStock,
			HasVariants:        listingRow.HasVariants,
			IsTracked:          listingRow.IsTracked,
			IsDelisted:         listingRow.IsDelisted,
			PackSize:           listingRow.PackSize,
			CurrentPrice:       listingRow.CurrentPrice,
			PreviousPrice:      listingRow.PreviousPrice,
			PriceLastChangedAt: listingRow.PriceLastChangedAt,
		}, listingRow.VariantCount))
	}

	context.JSON(http.StatusOK, gin.H{
		"vendor":      toVendorResponse(vendorRow),
		"listings":    listingSummaries,
		"total_count": totalCount,
		"limit":       pageSize,
		"offset":      pageOffset,
	})
}

type pricePointResponse struct {
	Date  string          `json:"date"`
	Price decimal.Decimal `json:"price"`
}

type moqTierResponse struct {
	QuantityRangeMinimum int              `json:"quantity_range_minimum"`
	QuantityRangeMaximum *int             `json:"quantity_range_maximum"`
	PricePerUnit         decimal.Decimal  `json:"price_per_unit"`
	DiscountPercent      *decimal.Decimal `json:"discount_percent"`
}

type variantResponse struct {
	VariantID      uuid.UUID            `json:"variant_id"`
	VariantLabel   string               `json:"variant_label"`
	VariantSKU     *string              `json:"variant_sku"`
	IsInStock      bool                 `json:"is_in_stock"`
	IsDelisted     bool                 `json:"is_delisted"`
	PackSize       *int                 `json:"pack_size"`
	CurrentPrice   *decimal.Decimal     `json:"current_price"`
	PreviousPrice  *decimal.Decimal     `json:"previous_price"`
	PricePerUnit   *decimal.Decimal     `json:"price_per_unit"`
	PriceDirection money.PriceDirection `json:"price_direction"`
	MoqTiers       []moqTierResponse    `json:"moq_tiers"`
	PriceHistory   []pricePointResponse `json:"price_history"`
}

type pastOrderResponse struct {
	OrderItemID  uuid.UUID        `json:"order_item_id"`
	OrderEntryID uuid.UUID        `json:"order_entry_id"`
	OrderedOn    string           `json:"ordered_on"`
	EntryName    *string          `json:"entry_name"`
	VariantLabel *string          `json:"variant_label"`
	Quantity     int32            `json:"quantity"`
	PricePerUnit decimal.Decimal  `json:"price_per_unit"`
	OrderStatus  string           `json:"order_status"`
	RefundAmount *decimal.Decimal `json:"refund_amount"`
	Rating       *int             `json:"rating"`
}

type listingDetailResponse struct {
	listingSummaryResponse
	Description   *string              `json:"description"`
	VendorSideSKU *string              `json:"vendor_side_sku"`
	Vendor        vendorResponse       `json:"vendor"`
	Variants      []variantResponse    `json:"variants"`
	MoqTiers      []moqTierResponse    `json:"moq_tiers"`
	PriceHistory  []pricePointResponse `json:"price_history"`
	AverageRating decimal.Decimal      `json:"average_rating"`
	RatingCount   int64                `json:"rating_count"`
	PastOrders    []pastOrderResponse  `json:"past_orders"`
}

// P1-B. Pricing and history hang off variants when there are any, and off the
// listing itself when there are none (BR-4).
func (server *Server) handleGetListing(context *gin.Context) {
	listingID, ok := parseUUIDParam(context, "listingID")
	if !ok {
		return
	}

	listingRow, err := server.queries.GetVendorListingByID(context, listingID)
	if err != nil {
		server.respondDatabaseError(context, err, "listing")
		return
	}
	vendorRow, err := server.queries.GetVendorByID(context, listingRow.VendorID)
	if err != nil {
		server.respondDatabaseError(context, err, "vendor")
		return
	}

	// Some vendors keep the sizes and their prices on the product page only.
	// Read it before answering, so the page shows the options rather than an
	// empty section.
	server.refreshDetailIfStale(context, listingRow, vendorRow)
	if refreshedRow, refreshErr := server.queries.GetVendorListingByID(context, listingID); refreshErr == nil {
		listingRow = refreshedRow
	}

	response := listingDetailResponse{
		listingSummaryResponse: toListingSummary(listingRow, 0),
		Description:            database.TextValue(listingRow.Description),
		VendorSideSKU:          database.TextValue(listingRow.VendorSideSku),
		Vendor:                 toVendorResponse(vendorRow),
		Variants:               []variantResponse{},
		MoqTiers:               []moqTierResponse{},
		PriceHistory:           []pricePointResponse{},
		PastOrders:             []pastOrderResponse{},
	}

	if listingRow.HasVariants {
		variantRows, err := server.queries.ListVariantsForListing(context, listingID)
		if err != nil {
			server.respondDatabaseError(context, err, "variants")
			return
		}
		response.VariantCount = int64(len(variantRows))

		for _, variantRow := range variantRows {
			variantDetail, err := server.buildVariantResponse(context, variantRow)
			if err != nil {
				server.respondDatabaseError(context, err, "variant detail")
				return
			}
			response.Variants = append(response.Variants, variantDetail)
		}
	} else {
		tierRows, err := server.queries.ListMoqTiersForListing(context, database.NullUUID(listingID))
		if err != nil {
			server.respondDatabaseError(context, err, "moq tiers")
			return
		}
		response.MoqTiers = toMoqTierResponses(tierRows)

		historyRows, err := server.queries.ListPriceHistoryForListing(context, database.NullUUID(listingID))
		if err != nil {
			server.respondDatabaseError(context, err, "price history")
			return
		}
		response.PriceHistory = make([]pricePointResponse, 0, len(historyRows))
		for _, historyRow := range historyRows {
			response.PriceHistory = append(response.PriceHistory, pricePointResponse{
				Date:  database.DateValue(historyRow.ScrapedAtDate).Format(time.DateOnly),
				Price: historyRow.Price,
			})
		}
	}

	// BR-8a: computed from order-item ratings, never stored.
	ratingRow, err := server.queries.GetListingAggregateRating(context, database.NullUUID(listingID))
	if err != nil {
		server.respondDatabaseError(context, err, "rating")
		return
	}
	response.AverageRating = ratingRow.AverageRating
	response.RatingCount = ratingRow.RatingCount

	// FR-P1-9: every past purchase of this listing.
	pastOrderRows, err := server.queries.ListOrderItemsForListing(context, database.NullUUID(listingID))
	if err != nil {
		server.respondDatabaseError(context, err, "past orders")
		return
	}
	for _, pastOrderRow := range pastOrderRows {
		response.PastOrders = append(response.PastOrders, pastOrderResponse{
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

	context.JSON(http.StatusOK, response)
}

func (server *Server) buildVariantResponse(
	context *gin.Context,
	variantRow database.Variant,
) (variantResponse, error) {
	currentPrice := database.DecimalValue(variantRow.CurrentPrice)
	previousPrice := database.DecimalValue(variantRow.PreviousPrice)
	packSize := database.IntValue(variantRow.PackSize)

	tierRows, err := server.queries.ListMoqTiersForVariant(context,
		database.NullUUID(variantRow.VariantID))
	if err != nil {
		return variantResponse{}, err
	}
	historyRows, err := server.queries.ListPriceHistoryForVariant(context,
		database.NullUUID(variantRow.VariantID))
	if err != nil {
		return variantResponse{}, err
	}

	pricePoints := make([]pricePointResponse, 0, len(historyRows))
	for _, historyRow := range historyRows {
		pricePoints = append(pricePoints, pricePointResponse{
			Date:  database.DateValue(historyRow.ScrapedAtDate).Format(time.DateOnly),
			Price: historyRow.Price,
		})
	}

	return variantResponse{
		VariantID:      variantRow.VariantID,
		VariantLabel:   variantRow.VariantLabel,
		VariantSKU:     database.TextValue(variantRow.VariantSku),
		IsInStock:      variantRow.IsInStock,
		IsDelisted:     variantRow.IsDelisted,
		PackSize:       packSize,
		CurrentPrice:   currentPrice,
		PreviousPrice:  previousPrice,
		PricePerUnit:   money.PerUnit(currentPrice, packSize),
		PriceDirection: money.DirectionOf(currentPrice, previousPrice),
		MoqTiers:       toMoqTierResponses(tierRows),
		PriceHistory:   pricePoints,
	}, nil
}

func toMoqTierResponses(tierRows []database.MoqTier) []moqTierResponse {
	tiers := make([]moqTierResponse, 0, len(tierRows))
	for _, tierRow := range tierRows {
		tiers = append(tiers, moqTierResponse{
			QuantityRangeMinimum: int(tierRow.QuantityRangeMinimum),
			QuantityRangeMaximum: database.IntValue(tierRow.QuantityRangeMaximum),
			PricePerUnit:         tierRow.PricePerUnit,
			DiscountPercent:      database.DecimalValue(tierRow.DiscountPercent),
		})
	}
	return tiers
}

type setTrackedRequest struct {
	IsTracked *bool `json:"is_tracked"`
}

// The star. Turning it on is what makes the scraper start recording this
// listing's MOQ tiers and daily price history.
func (server *Server) handleSetListingTracked(context *gin.Context) {
	listingID, ok := parseUUIDParam(context, "listingID")
	if !ok {
		return
	}
	var request setTrackedRequest
	if err := context.ShouldBindJSON(&request); err != nil || request.IsTracked == nil {
		respondError(context, http.StatusBadRequest, "is_tracked must be true or false")
		return
	}

	listingRow, err := server.queries.SetListingTracked(context, database.SetListingTrackedParams{
		VendorListingID: listingID,
		IsTracked:       *request.IsTracked,
	})
	if err != nil {
		server.respondDatabaseError(context, err, "listing")
		return
	}

	response := gin.H{"listing": toListingSummary(listingRow, 0)}

	// Starring is otherwise invisible until the next nightly run, which makes
	// the feature look broken: the quantity discounts it exists to collect
	// would not appear for a day. Fetching the page now closes that gap.
	//
	// It is best effort. The star is already saved, and a vendor being slow or
	// unreachable must not undo the user's action — the nightly run will pick
	// the listing up regardless.
	if *request.IsTracked {
		enrichContext, cancelEnrich := stdcontext.WithTimeout(
			context.Request.Context(), 25*time.Second)
		defer cancelEnrich()

		if err := server.enrichNow(enrichContext, listingID, true); err != nil {
			server.logger.Warn("could not enrich on starring",
				"listing_id", listingID, "error", err)
			response["detail_status"] = "pending"
			response["detail_message"] =
				"Saved. Quantity discounts could not be read from the vendor just now, " +
					"so they will arrive with the next scrape."
		} else {
			response["detail_status"] = "ready"
		}
	}

	context.JSON(http.StatusOK, response)
}

// enrichNow reads one product page immediately. The runner is built per call
// rather than held on the server: it is a cheap struct, and the API otherwise
// has no reason to own scraping state.
func (server *Server) enrichNow(
	ctx stdcontext.Context,
	listingID uuid.UUID,
	recordPriceHistory bool,
) error {
	scrapeRunner, err := runner.New(server.pool, server.logger, 0)
	if err != nil {
		return err
	}
	return scrapeRunner.EnrichOneListing(ctx, listingID, recordPriceHistory)
}

// detailFreshnessWindow is how long a product page's contents are reused
// before being read again. Vendors change prices at most daily, so a few hours
// keeps the sizes and discounts honest without fetching on every glance.
const detailFreshnessWindow = 6 * time.Hour

// refreshDetailIfStale reads the product page when it holds something the
// catalogue feed does not and what we stored is missing or old.
//
// This is deliberately not gated on the star. The sizes and their prices are
// what someone needs in order to decide whether a product is worth tracking,
// so putting them behind tracking would hide the information the decision
// depends on. Starring still governs price history, which is the thing that
// genuinely has to accumulate over time.
func (server *Server) refreshDetailIfStale(
	context *gin.Context,
	listingRow database.VendorListing,
	vendorRow database.Vendor,
) {
	if !config.ScraperTier(vendorRow.ScraperTier).ProductPageAddsDetail() {
		return
	}
	if fetchedAt := database.TimeValue(listingRow.DetailFetchedAt); fetchedAt != nil &&
		time.Since(*fetchedAt) < detailFreshnessWindow {
		return
	}

	enrichContext, cancelEnrich := stdcontext.WithTimeout(
		context.Request.Context(), 20*time.Second)
	defer cancelEnrich()

	// Best effort: a vendor being slow must not stop the page rendering with
	// whatever is already stored.
	if err := server.enrichNow(enrichContext, listingRow.VendorListingID, false); err != nil {
		server.logger.Warn("could not refresh listing detail",
			"listing_id", listingRow.VendorListingID, "error", err)
	}
}

// handleFindListingByURL resolves a pasted product link to the listing it
// refers to. The query string carries the URL rather than the path, so a link
// full of slashes needs no escaping by the caller.
func (server *Server) handleFindListingByURL(context *gin.Context) {
	requestedURL := strings.TrimSpace(context.Query("url"))
	if requestedURL == "" {
		respondError(context, http.StatusBadRequest, "url is required")
		return
	}
	// Everything after the path is tracking noise a vendor never publishes as
	// part of the product's address.
	if parsedURL, err := url.Parse(requestedURL); err == nil {
		parsedURL.RawQuery = ""
		parsedURL.Fragment = ""
		requestedURL = parsedURL.String()
	}

	listingRow, err := server.queries.FindListingByURL(context, requestedURL)
	if err != nil {
		server.respondDatabaseError(context, err, "listing for that link")
		return
	}
	context.JSON(http.StatusOK, gin.H{
		"vendor_listing_id": listingRow.VendorListingID,
		"vendor_slug":       listingRow.VendorSlug,
		"vendor_name":       listingRow.VendorName,
		"listing_name":      listingRow.ListingName,
	})
}

func (server *Server) handleListTrackedListings(context *gin.Context) {
	trackedRows, err := server.queries.ListTrackedListings(context)
	if err != nil {
		server.respondDatabaseError(context, err, "tracked listings")
		return
	}

	type trackedListingResponse struct {
		listingSummaryResponse
		VendorName string `json:"vendor_name"`
		VendorSlug string `json:"vendor_slug"`
	}

	responses := make([]trackedListingResponse, 0, len(trackedRows))
	for _, trackedRow := range trackedRows {
		responses = append(responses, trackedListingResponse{
			listingSummaryResponse: toListingSummary(database.VendorListing{
				VendorListingID:    trackedRow.VendorListingID,
				VendorID:           trackedRow.VendorID,
				ProductUrl:         trackedRow.ProductUrl,
				ListingName:        trackedRow.ListingName,
				PrimaryImageUrl:    trackedRow.PrimaryImageUrl,
				VendorSideCategory: trackedRow.VendorSideCategory,
				IsInStock:          trackedRow.IsInStock,
				HasVariants:        trackedRow.HasVariants,
				IsTracked:          trackedRow.IsTracked,
				IsDelisted:         trackedRow.IsDelisted,
				PackSize:           trackedRow.PackSize,
				CurrentPrice:       trackedRow.CurrentPrice,
				PreviousPrice:      trackedRow.PreviousPrice,
				PriceLastChangedAt: trackedRow.PriceLastChangedAt,
			}, trackedRow.VariantCount),
			VendorName: trackedRow.VendorName,
			VendorSlug: trackedRow.VendorSlug,
		})
	}
	context.JSON(http.StatusOK, gin.H{"listings": responses})
}
