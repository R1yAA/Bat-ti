// Mirrors of the Go handlers' response structs.
//
// Money arrives as a string, not a number: the API encodes numeric columns with
// shopspring/decimal, which marshals exactly rather than through float64. Kept
// as strings here for the same reason — they are parsed only at the moment of
// display, never accumulated in JavaScript.

export type Decimal = string;
export type UUID = string;

/** BR-16. "none" covers both no change and nothing to compare against. */
export type PriceDirection = "up" | "down" | "none";

/** BR-10. */
export type OrderStatus =
  | "placed"
  | "cancelled"
  | "refunded"
  | "partially_refunded";

export interface Vendor {
  vendor_id: UUID;
  vendor_slug: string;
  vendor_name: string;
  source_base_url: string;
  scraper_tier: string;
  last_successful_scrape_at: string | null;
  last_scrape_attempt_at: string | null;
  last_scrape_error: string | null;
}

export interface ListingSummary {
  vendor_listing_id: UUID;
  listing_name: string;
  product_url: string;
  primary_image_url: string | null;
  is_in_stock: boolean;
  is_delisted: boolean;
  is_tracked: boolean;
  has_variants: boolean;
  variant_count: number;
  pack_size: number | null;
  current_price: Decimal | null;
  previous_price: Decimal | null;
  price_per_unit: Decimal | null;
  price_direction: PriceDirection;
  price_last_changed_at: string | null;
  vendor_side_category: string | null;
}

export interface TrackedListing extends ListingSummary {
  vendor_name: string;
  vendor_slug: string;
}

export interface MoqTier {
  quantity_range_minimum: number;
  quantity_range_maximum: number | null;
  price_per_unit: Decimal;
  discount_percent: Decimal | null;
}

export interface PricePoint {
  date: string;
  price: Decimal;
}

export interface Variant {
  variant_id: UUID;
  variant_label: string;
  variant_sku: string | null;
  is_in_stock: boolean;
  is_delisted: boolean;
  pack_size: number | null;
  current_price: Decimal | null;
  previous_price: Decimal | null;
  price_per_unit: Decimal | null;
  price_direction: PriceDirection;
  moq_tiers: MoqTier[];
  price_history: PricePoint[];
}

export interface PastOrder {
  order_item_id: UUID;
  order_entry_id: UUID;
  ordered_on: string;
  entry_name: string | null;
  variant_label: string | null;
  quantity: number;
  price_per_unit: Decimal;
  order_status: OrderStatus;
  refund_amount: Decimal | null;
  rating: number | null;
}

export interface ListingDetail extends ListingSummary {
  description: string | null;
  vendor_side_sku: string | null;
  vendor: Vendor;
  variants: Variant[];
  moq_tiers: MoqTier[];
  price_history: PricePoint[];
  average_rating: Decimal;
  rating_count: number;
  past_orders: PastOrder[];
}

export interface VendorListingsPage {
  vendor: Vendor;
  listings: ListingSummary[];
  total_count: number;
  limit: number;
  offset: number;
}

export interface CompareEntry {
  compare_entry_id: UUID;
  entry_name: string;
  member_count: number;
  created_at: string | null;
}

export interface CompareMember {
  compare_entry_member_id: UUID;
  vendor_listing_id: UUID;
  variant_id: UUID | null;
  vendor_name: string;
  vendor_slug: string;
  listing_name: string;
  variant_label: string | null;
  product_url: string;
  primary_image_url: string | null;
  is_in_stock: boolean;
  is_delisted: boolean;
  pack_size: number | null;
  current_price: Decimal | null;
  price_per_unit: Decimal | null;
  average_rating: Decimal;
  rating_count: number;
  moq_tiers: MoqTier[];
  price_history: PricePoint[];
  past_orders: PastOrder[];
}

export interface CompareEntryDetail {
  compare_entry: CompareEntry;
  members: CompareMember[];
}

export interface Category {
  category_id: UUID;
  category_name: string;
  is_system: boolean;
  usage_count: number;
}

export interface OccasionTag {
  occasion_tag_id: UUID;
  tag_name: string;
  usage_count: number;
}

export interface OrderItem {
  order_item_id: UUID;
  order_entry_id: UUID;
  vendor_listing_id: UUID | null;
  variant_id: UUID | null;
  vendor_id: UUID;
  vendor_name: string;
  listing_name: string;
  variant_label: string | null;
  product_url: string | null;
  primary_image_url: string | null;
  quantity: number;
  price_per_unit: Decimal;
  line_total: Decimal;
  order_status: OrderStatus;
  refund_amount: Decimal | null;
  rating: number | null;
  category_tags: Category[];
  occasion_tags: OccasionTag[];
}

export interface OrderEntry {
  order_entry_id: UUID;
  entry_name: string | null;
  ordered_on: string;
  total_cost: Decimal;
  item_count: number;
  items?: OrderItem[];
}

export interface SpendSummary {
  net_spend: Decimal;
  gross_spend: Decimal;
  excluded_spend: Decimal;
  refunded_amount: Decimal;
  item_count: number;
  order_count: number;
}

export interface CategorySpend {
  category_name: string;
  net_spend: Decimal;
  gross_spend: Decimal;
}

export interface OccasionSpend {
  tag_name: string;
  net_spend: Decimal;
  gross_spend: Decimal;
}

export interface MonthlySpend {
  month: string;
  net_spend: Decimal;
  gross_spend: Decimal;
}

export interface ScrapeRun {
  scrape_run_id: UUID;
  vendor_slug: string;
  vendor_name: string;
  started_at: string | null;
  finished_at: string | null;
  run_status: string;
  listings_seen: number;
  listings_updated: number;
  listings_delisted: number;
  error_message: string | null;
}
