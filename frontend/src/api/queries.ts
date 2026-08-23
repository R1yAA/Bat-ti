import {
  useMutation,
  useQuery,
  useQueryClient,
  type QueryClient,
} from "@tanstack/react-query";
import { api, queryString } from "./client";
import type {
  Category,
  CompareEntry,
  CompareEntryDetail,
  ListingDetail,
  MonthlySpend,
  OccasionTag,
  OrderEntry,
  SaleOrderCategory,
  SaleOrderEntry,
  ScrapeRun,
  SpendSummary,
  CategorySales,
  CategorySpend,
  MonthlySales,
  OccasionSpend,
  SalesSummary,
  TrackedListing,
  UUID,
  Vendor,
  VendorListingsPage,
} from "./types";

// Query keys are arrays so a whole family can be invalidated at once — writing
// an order invalidates every spend figure without naming each date range.
export const queryKeys = {
  vendors: ["vendors"] as const,
  vendorListings: (vendorSlug: string, filters: ListingFilters) =>
    ["vendor-listings", vendorSlug, filters] as const,
  listing: (listingID: UUID) => ["listing", listingID] as const,
  trackedListings: ["tracked-listings"] as const,
  compareEntries: ["compare-entries"] as const,
  compareEntry: (entryID: UUID) => ["compare-entry", entryID] as const,
  orderEntries: ["order-entries"] as const,
  orderEntry: (orderEntryID: UUID) => ["order-entry", orderEntryID] as const,
  saleOrderEntries: ["sale-order-entries"] as const,
  saleOrderEntry: (saleOrderEntryID: UUID) =>
    ["sale-order-entry", saleOrderEntryID] as const,
  spend: ["spend"] as const,
  // Separate from `spend` so a purchase write does not refetch the sell-side
  // figures, and a sale does not refetch the buy-side ones.
  sales: ["sales"] as const,
  categories: ["categories"] as const,
  saleOrderCategories: ["sale-order-categories"] as const,
  occasionTags: ["occasion-tags"] as const,
  scrapeRuns: ["scrape-runs"] as const,
};

export interface ListingFilters {
  limit: number;
  offset: number;
  search: string;
  inStockOnly: boolean;
  trackedOnly: boolean;
  includeDelisted: boolean;
}

// ── P1: vendors and listings ──────────────────────────────────────────────

export function useVendors() {
  return useQuery({
    queryKey: queryKeys.vendors,
    queryFn: () => api.get<{ vendors: Vendor[] }>("/vendors"),
    select: (data) => data.vendors,
  });
}

export function useVendorListings(vendorSlug: string, filters: ListingFilters) {
  return useQuery({
    queryKey: queryKeys.vendorListings(vendorSlug, filters),
    queryFn: () =>
      api.get<VendorListingsPage>(
        `/vendors/${vendorSlug}/listings` +
          queryString({
            limit: filters.limit,
            offset: filters.offset,
            search: filters.search,
            in_stock_only: filters.inStockOnly,
            tracked_only: filters.trackedOnly,
            include_delisted: filters.includeDelisted,
          }),
      ),
    enabled: vendorSlug !== "",
    // Keeping the previous page on screen while the next loads stops the
    // catalogue collapsing to a spinner on every keystroke of the search box.
    placeholderData: (previous) => previous,
  });
}

export function useListing(listingID: UUID) {
  return useQuery({
    queryKey: queryKeys.listing(listingID),
    queryFn: () => api.get<ListingDetail>(`/listings/${listingID}`),
    enabled: listingID !== "",
  });
}

export function useTrackedListings() {
  return useQuery({
    queryKey: queryKeys.trackedListings,
    queryFn: () => api.get<{ listings: TrackedListing[] }>("/tracked-listings"),
    select: (data) => data.listings,
  });
}

/** Resolves a pasted product link to the listing it refers to. */
export function useFindListingByURL() {
  return useMutation({
    mutationFn: (productURL: string) =>
      api.get<{
        vendor_listing_id: UUID;
        vendor_slug: string;
        vendor_name: string;
        listing_name: string;
      }>(`/listing-by-url${queryString({ url: productURL })}`),
  });
}

export function useSetListingTracked() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({
      listingID,
      isTracked,
    }: {
      listingID: UUID;
      isTracked: boolean;
    }) =>
      api.put<unknown>(`/listings/${listingID}/track`, {
        is_tracked: isTracked,
      }),
    onSuccess: (_result, variables) => {
      void queryClient.invalidateQueries({
        queryKey: queryKeys.listing(variables.listingID),
      });
      void queryClient.invalidateQueries({ queryKey: ["vendor-listings"] });
      void queryClient.invalidateQueries({
        queryKey: queryKeys.trackedListings,
      });
    },
  });
}

// ── P2: compare entries ───────────────────────────────────────────────────

export function useCompareEntries() {
  return useQuery({
    queryKey: queryKeys.compareEntries,
    queryFn: () =>
      api.get<{ compare_entries: CompareEntry[] }>("/compare-entries"),
    select: (data) => data.compare_entries,
  });
}

export function useCompareEntry(entryID: UUID) {
  return useQuery({
    queryKey: queryKeys.compareEntry(entryID),
    queryFn: () => api.get<CompareEntryDetail>(`/compare-entries/${entryID}`),
    enabled: entryID !== "",
  });
}

export function useCreateCompareEntry() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (entryName: string) =>
      api.post<CompareEntry>("/compare-entries", { entry_name: entryName }),
    onSuccess: () =>
      queryClient.invalidateQueries({ queryKey: queryKeys.compareEntries }),
  });
}

export function useRenameCompareEntry() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ entryID, entryName }: { entryID: UUID; entryName: string }) =>
      api.put<CompareEntry>(`/compare-entries/${entryID}`, {
        entry_name: entryName,
      }),
    onSuccess: (_result, variables) => {
      void queryClient.invalidateQueries({
        queryKey: queryKeys.compareEntries,
      });
      void queryClient.invalidateQueries({
        queryKey: queryKeys.compareEntry(variables.entryID),
      });
    },
  });
}

export function useDeleteCompareEntry() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (entryID: UUID) =>
      api.delete<unknown>(`/compare-entries/${entryID}`),
    onSuccess: () =>
      queryClient.invalidateQueries({ queryKey: queryKeys.compareEntries }),
  });
}

export function useAddCompareMember() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({
      entryID,
      vendorListingID,
      variantID,
    }: {
      entryID: UUID;
      vendorListingID?: UUID;
      variantID?: UUID;
    }) =>
      api.post<unknown>(`/compare-entries/${entryID}/members`, {
        vendor_listing_id: vendorListingID ?? null,
        variant_id: variantID ?? null,
      }),
    onSuccess: (_result, variables) => {
      void queryClient.invalidateQueries({
        queryKey: queryKeys.compareEntry(variables.entryID),
      });
      void queryClient.invalidateQueries({
        queryKey: queryKeys.compareEntries,
      });
    },
  });
}

export function useDeleteCompareMember() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ entryID, memberID }: { entryID: UUID; memberID: UUID }) =>
      api.delete<unknown>(`/compare-entries/${entryID}/members/${memberID}`),
    onSuccess: (_result, variables) => {
      void queryClient.invalidateQueries({
        queryKey: queryKeys.compareEntry(variables.entryID),
      });
      void queryClient.invalidateQueries({
        queryKey: queryKeys.compareEntries,
      });
    },
  });
}

// ── P3: orders ────────────────────────────────────────────────────────────

export function useOrderEntries() {
  return useQuery({
    queryKey: queryKeys.orderEntries,
    queryFn: () => api.get<{ order_entries: OrderEntry[] }>("/order-entries"),
    select: (data) => data.order_entries,
  });
}

export function useOrderEntry(orderEntryID: UUID) {
  return useQuery({
    queryKey: queryKeys.orderEntry(orderEntryID),
    queryFn: () => api.get<OrderEntry>(`/order-entries/${orderEntryID}`),
    enabled: orderEntryID !== "",
  });
}

/** Every order write moves a spend figure, so P4 is invalidated alongside. */
function invalidateOrders(queryClient: QueryClient, orderEntryID?: UUID) {
  void queryClient.invalidateQueries({ queryKey: queryKeys.orderEntries });
  void queryClient.invalidateQueries({ queryKey: queryKeys.spend });
  void queryClient.invalidateQueries({ queryKey: queryKeys.categories });
  void queryClient.invalidateQueries({ queryKey: queryKeys.occasionTags });
  if (orderEntryID) {
    void queryClient.invalidateQueries({
      queryKey: queryKeys.orderEntry(orderEntryID),
    });
  }
}

export interface OrderEntryInput {
  entry_name: string;
  ordered_on: string;
}

export function useCreateOrderEntry() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (input: OrderEntryInput) =>
      api.post<OrderEntry>("/order-entries", input),
    onSuccess: () => invalidateOrders(queryClient),
  });
}

export function useUpdateOrderEntry() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({
      orderEntryID,
      input,
    }: {
      orderEntryID: UUID;
      input: OrderEntryInput;
    }) => api.put<OrderEntry>(`/order-entries/${orderEntryID}`, input),
    onSuccess: (_result, variables) =>
      invalidateOrders(queryClient, variables.orderEntryID),
  });
}

export function useDeleteOrderEntry() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (orderEntryID: UUID) =>
      api.delete<unknown>(`/order-entries/${orderEntryID}`),
    onSuccess: () => invalidateOrders(queryClient),
  });
}

export interface OrderItemInput {
  vendor_listing_id?: UUID | null;
  variant_id?: UUID | null;
  vendor_id?: UUID | null;
  listing_name?: string;
  quantity: number;
  price_per_unit: string;
  order_status: string;
  refund_amount?: string | null;
  rating?: number | null;
  category_ids?: UUID[];
  occasion_tag_ids?: UUID[];
}

export function useCreateOrderItem() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({
      orderEntryID,
      input,
    }: {
      orderEntryID: UUID;
      input: OrderItemInput;
    }) => api.post<OrderEntry>(`/order-entries/${orderEntryID}/items`, input),
    onSuccess: (_result, variables) =>
      invalidateOrders(queryClient, variables.orderEntryID),
  });
}

export function useUpdateOrderItem() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({
      orderItemID,
      input,
    }: {
      orderItemID: UUID;
      orderEntryID: UUID;
      input: OrderItemInput;
    }) => api.put<OrderEntry>(`/order-items/${orderItemID}`, input),
    onSuccess: (_result, variables) =>
      invalidateOrders(queryClient, variables.orderEntryID),
  });
}

export function useDeleteOrderItem() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ orderItemID }: { orderItemID: UUID; orderEntryID: UUID }) =>
      api.delete<unknown>(`/order-items/${orderItemID}`),
    onSuccess: (_result, variables) =>
      invalidateOrders(queryClient, variables.orderEntryID),
  });
}

// ── P6: sales orders ──────────────────────────────────────────────────────

/** FR-P6-6. An empty status means every sale. */
export function useSaleOrderEntries(status: string) {
  return useQuery({
    queryKey: [...queryKeys.saleOrderEntries, status],
    queryFn: () =>
      api.get<{ sale_order_entries: SaleOrderEntry[] }>(
        `/sale-order-entries${queryString({ status })}`,
      ),
    select: (data) => data.sale_order_entries,
  });
}

export function useSaleOrderEntry(saleOrderEntryID: UUID) {
  return useQuery({
    queryKey: queryKeys.saleOrderEntry(saleOrderEntryID),
    queryFn: () =>
      api.get<SaleOrderEntry>(`/sale-order-entries/${saleOrderEntryID}`),
    enabled: saleOrderEntryID !== "",
  });
}

/** Every sale write moves a gain figure, so the sell-side analytics and the
 *  category usage counts are invalidated alongside. */
function invalidateSales(queryClient: QueryClient, saleOrderEntryID?: UUID) {
  void queryClient.invalidateQueries({ queryKey: queryKeys.saleOrderEntries });
  void queryClient.invalidateQueries({ queryKey: queryKeys.sales });
  void queryClient.invalidateQueries({
    queryKey: queryKeys.saleOrderCategories,
  });
  if (saleOrderEntryID) {
    void queryClient.invalidateQueries({
      queryKey: queryKeys.saleOrderEntry(saleOrderEntryID),
    });
  }
}

export interface SaleOrderEntryInput {
  consumer_name: string;
  order_placed_date: string;
  order_status: string;
  /** Ignored by the API unless order_status is "delivered" (BR-20). */
  delivered_date: string | null;
  sale_order_category_id: UUID | null;
}

export function useCreateSaleOrderEntry() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (input: SaleOrderEntryInput) =>
      api.post<SaleOrderEntry>("/sale-order-entries", input),
    onSuccess: () => invalidateSales(queryClient),
  });
}

export function useUpdateSaleOrderEntry() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({
      saleOrderEntryID,
      input,
    }: {
      saleOrderEntryID: UUID;
      input: SaleOrderEntryInput;
    }) =>
      api.put<SaleOrderEntry>(`/sale-order-entries/${saleOrderEntryID}`, input),
    onSuccess: (_result, variables) =>
      invalidateSales(queryClient, variables.saleOrderEntryID),
  });
}

export function useDeleteSaleOrderEntry() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (saleOrderEntryID: UUID) =>
      api.delete<unknown>(`/sale-order-entries/${saleOrderEntryID}`),
    onSuccess: () => invalidateSales(queryClient),
  });
}

export interface SaleOrderItemInput {
  product_name: string;
  quantity: number;
  price_per_unit: string;
}

export function useCreateSaleOrderItem() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({
      saleOrderEntryID,
      input,
    }: {
      saleOrderEntryID: UUID;
      input: SaleOrderItemInput;
    }) =>
      api.post<SaleOrderEntry>(
        `/sale-order-entries/${saleOrderEntryID}/items`,
        input,
      ),
    onSuccess: (_result, variables) =>
      invalidateSales(queryClient, variables.saleOrderEntryID),
  });
}

export function useUpdateSaleOrderItem() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({
      saleOrderItemID,
      input,
    }: {
      saleOrderItemID: UUID;
      saleOrderEntryID: UUID;
      input: SaleOrderItemInput;
    }) =>
      api.put<SaleOrderEntry>(`/sale-order-items/${saleOrderItemID}`, input),
    onSuccess: (_result, variables) =>
      invalidateSales(queryClient, variables.saleOrderEntryID),
  });
}

export function useDeleteSaleOrderItem() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({
      saleOrderItemID,
    }: {
      saleOrderItemID: UUID;
      saleOrderEntryID: UUID;
    }) => api.delete<unknown>(`/sale-order-items/${saleOrderItemID}`),
    onSuccess: (_result, variables) =>
      invalidateSales(queryClient, variables.saleOrderEntryID),
  });
}

// ── P4: spend ─────────────────────────────────────────────────────────────

export function useSpendSummary(startDate: string, endDate: string) {
  return useQuery({
    queryKey: [...queryKeys.spend, "summary", startDate, endDate],
    queryFn: () =>
      api.get<SpendSummary>(
        `/spend-summary${queryString({ start_date: startDate, end_date: endDate })}`,
      ),
  });
}

export function useSpendByCategory(startDate: string, endDate: string) {
  return useQuery({
    queryKey: [...queryKeys.spend, "by-category", startDate, endDate],
    queryFn: () =>
      api.get<{ categories: CategorySpend[] }>(
        `/spend-by-category${queryString({ start_date: startDate, end_date: endDate })}`,
      ),
    select: (data) => data.categories,
  });
}

export function useSpendByOccasion(startDate: string, endDate: string) {
  return useQuery({
    queryKey: [...queryKeys.spend, "by-occasion", startDate, endDate],
    queryFn: () =>
      api.get<{ occasions: OccasionSpend[] }>(
        `/spend-by-occasion${queryString({ start_date: startDate, end_date: endDate })}`,
      ),
    select: (data) => data.occasions,
  });
}

export function useMonthlySpendTrend() {
  return useQuery({
    queryKey: [...queryKeys.spend, "monthly"],
    queryFn: () => api.get<{ months: MonthlySpend[] }>("/spend-monthly-trend"),
    select: (data) => data.months,
  });
}

// ── P4, sell side: FR-P4-7 and FR-P4-8 ────────────────────────────────────

export function useSalesSummary(startDate: string, endDate: string) {
  return useQuery({
    queryKey: [...queryKeys.sales, "summary", startDate, endDate],
    queryFn: () =>
      api.get<SalesSummary>(
        `/sales-summary${queryString({ start_date: startDate, end_date: endDate })}`,
      ),
  });
}

export function useSalesByCategory(startDate: string, endDate: string) {
  return useQuery({
    queryKey: [...queryKeys.sales, "by-category", startDate, endDate],
    queryFn: () =>
      api.get<{ categories: CategorySales[] }>(
        `/sales-by-category${queryString({ start_date: startDate, end_date: endDate })}`,
      ),
    select: (data) => data.categories,
  });
}

export function useMonthlySalesTrend() {
  return useQuery({
    queryKey: [...queryKeys.sales, "monthly"],
    queryFn: () => api.get<{ months: MonthlySales[] }>("/sales-monthly-trend"),
    select: (data) => data.months,
  });
}

// ── P5: settings ──────────────────────────────────────────────────────────

export function useCategories() {
  return useQuery({
    queryKey: queryKeys.categories,
    queryFn: () => api.get<{ categories: Category[] }>("/categories"),
    select: (data) => data.categories,
  });
}

export function useCreateCategory() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (name: string) => api.post<Category>("/categories", { name }),
    onSuccess: () =>
      queryClient.invalidateQueries({ queryKey: queryKeys.categories }),
  });
}

export function useRenameCategory() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ categoryID, name }: { categoryID: UUID; name: string }) =>
      api.put<Category>(`/categories/${categoryID}`, { name }),
    onSuccess: () =>
      queryClient.invalidateQueries({ queryKey: queryKeys.categories }),
  });
}

export function useDeleteCategory() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (categoryID: UUID) =>
      api.delete<unknown>(`/categories/${categoryID}`),
    // BR-13 moves the orphaned items to Uncategorized, so orders change too.
    onSuccess: () => invalidateOrders(queryClient),
  });
}

export function useOccasionTags() {
  return useQuery({
    queryKey: queryKeys.occasionTags,
    queryFn: () => api.get<{ occasion_tags: OccasionTag[] }>("/occasion-tags"),
    select: (data) => data.occasion_tags,
  });
}

export function useCreateOccasionTag() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (name: string) =>
      api.post<OccasionTag>("/occasion-tags", { name }),
    onSuccess: () =>
      queryClient.invalidateQueries({ queryKey: queryKeys.occasionTags }),
  });
}

export function useRenameOccasionTag() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ tagID, name }: { tagID: UUID; name: string }) =>
      api.put<OccasionTag>(`/occasion-tags/${tagID}`, { name }),
    onSuccess: () =>
      queryClient.invalidateQueries({ queryKey: queryKeys.occasionTags }),
  });
}

export function useDeleteOccasionTag() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (tagID: UUID) => api.delete<unknown>(`/occasion-tags/${tagID}`),
    onSuccess: () => invalidateOrders(queryClient),
  });
}

// FR-P5-4: a separate managed list from the material categories above. The
// two are never merged — see the collision warning in the addendum.

export function useSaleOrderCategories() {
  return useQuery({
    queryKey: queryKeys.saleOrderCategories,
    queryFn: () =>
      api.get<{ sale_order_categories: SaleOrderCategory[] }>(
        "/sale-order-categories",
      ),
    select: (data) => data.sale_order_categories,
  });
}

export function useCreateSaleOrderCategory() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (name: string) =>
      api.post<SaleOrderCategory>("/sale-order-categories", { name }),
    onSuccess: () =>
      queryClient.invalidateQueries({
        queryKey: queryKeys.saleOrderCategories,
      }),
  });
}

export function useRenameSaleOrderCategory() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({
      saleOrderCategoryID,
      name,
    }: {
      saleOrderCategoryID: UUID;
      name: string;
    }) =>
      api.put<SaleOrderCategory>(
        `/sale-order-categories/${saleOrderCategoryID}`,
        { name },
      ),
    onSuccess: () => invalidateSales(queryClient),
  });
}

export function useDeleteSaleOrderCategory() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (saleOrderCategoryID: UUID) =>
      api.delete<unknown>(`/sale-order-categories/${saleOrderCategoryID}`),
    // BR-23 moves the orphaned sales to Uncategorized, so the sales change too.
    onSuccess: () => invalidateSales(queryClient),
  });
}

export function useDeleteAllData() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (confirmation: string) =>
      api.post<unknown>("/delete-all-data", { confirmation }),
    onSuccess: () => queryClient.invalidateQueries(),
  });
}

export function useScrapeRuns() {
  return useQuery({
    queryKey: queryKeys.scrapeRuns,
    queryFn: () => api.get<{ scrape_runs: ScrapeRun[] }>("/scrape-runs"),
    select: (data) => data.scrape_runs,
  });
}
