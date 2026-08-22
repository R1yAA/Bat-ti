-- Queries backing P1: the vendor catalogue view and the listing content page.

-- name: CountVendorListings :one
select count(*) from vendor_listings
where vendor_id = @vendor_id
  and (not @in_stock_only::boolean or is_in_stock)
  and (not @tracked_only::boolean or is_tracked)
  and (@include_delisted::boolean or not is_delisted)
  and (@search_text::text = ''
        or listing_name ilike '%' || @search_text::text || '%'
        or product_url ilike '%' || @search_text::text || '%');

-- name: ListVendorListings :many
-- The catalogue never hides an out-of-stock or delisted listing on its own
-- (BR-15); the filters here are driven by the user's own controls.
--
-- current_price and previous_price come back as they are stored, so the client
-- can draw the price-change arrow (FR-P1-2) without a second query.
select vendor_listings.*,
       (select count(*) from variants
         where variants.vendor_listing_id = vendor_listings.vendor_listing_id
           and not variants.is_delisted) as variant_count
from vendor_listings
where vendor_id = @vendor_id
  and (not @in_stock_only::boolean or is_in_stock)
  and (not @tracked_only::boolean or is_tracked)
  and (@include_delisted::boolean or not is_delisted)
  and (@search_text::text = ''
        or listing_name ilike '%' || @search_text::text || '%'
        or product_url ilike '%' || @search_text::text || '%')
order by listing_name
limit @result_limit offset @result_offset;

-- name: GetListingAggregateRating :one
-- BR-8a: a listing's rating is the average of the ratings given to order items
-- referencing it, not a separately entered value. Computed on read so it can
-- never go stale.
select coalesce(round(avg(rating)::numeric, 2), 0)::numeric(4, 2) as average_rating,
       count(rating)                                              as rating_count
from order_items
where vendor_listing_id = $1 and rating is not null;

-- name: ListOrderItemsForListing :many
-- FR-P1-9: every past purchase of this specific listing, with what was paid.
select order_items.*,
       order_entries.ordered_on,
       order_entries.entry_name,
       variants.variant_label
from order_items
join order_entries on order_entries.order_entry_id = order_items.order_entry_id
left join variants on variants.variant_id = order_items.variant_id
where order_items.vendor_listing_id = $1
order by order_entries.ordered_on desc;

-- name: ListTrackedListings :many
select vendor_listings.*, vendors.vendor_name, vendors.vendor_slug,
       (select count(*) from variants
         where variants.vendor_listing_id = vendor_listings.vendor_listing_id
           and not variants.is_delisted) as variant_count
from vendor_listings
join vendors on vendors.vendor_id = vendor_listings.vendor_id
where vendor_listings.is_tracked and not vendor_listings.is_delisted
order by vendors.vendor_name, vendor_listings.listing_name;

-- name: FindListingByURL :one
-- Pasting a product link jumps straight to it, whichever vendor it belongs to.
-- Trailing slashes differ between what a vendor publishes and what gets copied
-- from a browser, so both sides are trimmed before comparing.
select vendor_listings.*, vendors.vendor_slug, vendors.vendor_name
from vendor_listings
join vendors on vendors.vendor_id = vendor_listings.vendor_id
where rtrim(vendor_listings.product_url, '/') = rtrim(@product_url::text, '/')
limit 1;
