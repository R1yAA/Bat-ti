-- name: UpsertVendorListing :batchone
-- Note what is deliberately absent from the update list:
--   is_tracked  — the user's star, never touched by a scrape
--   pack_size   — coalesced, so a hand-entered value survives a scrape that
--                 could not detect one
--
-- Price columns are owned by whoever knows the real price:
--   * a listing without variants prices itself here
--   * a listing WITH variants leaves all three price columns alone, because
--     SetListingBasePriceFromVariants derives them from the live variants.
--     Without that guard the upsert would blank current_price on every run
--     and the roll-up would restore it, churning previous_price and reporting
--     a price change on every scrape.
insert into vendor_listings (
    vendor_id, product_url, external_product_id, listing_name, description,
    primary_image_url, vendor_side_category, vendor_side_sku,
    is_in_stock, has_variants, pack_size, current_price, last_seen_at
) values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, now())
on conflict (vendor_id, product_url) do update set
    external_product_id  = coalesce(excluded.external_product_id, vendor_listings.external_product_id),
    listing_name         = excluded.listing_name,
    description          = coalesce(excluded.description, vendor_listings.description),
    primary_image_url    = coalesce(excluded.primary_image_url, vendor_listings.primary_image_url),
    vendor_side_category = coalesce(excluded.vendor_side_category, vendor_listings.vendor_side_category),
    vendor_side_sku      = coalesce(excluded.vendor_side_sku, vendor_listings.vendor_side_sku),
    is_in_stock          = excluded.is_in_stock,
    has_variants         = excluded.has_variants,
    pack_size            = coalesce(excluded.pack_size, vendor_listings.pack_size),
    previous_price       = case
                               when excluded.has_variants
                                   then vendor_listings.previous_price
                               when excluded.current_price is distinct from vendor_listings.current_price
                                   then vendor_listings.current_price
                               else vendor_listings.previous_price
                           end,
    -- A first sighting is not a price change: there was no prior price to move
    -- away from. Requiring a non-null old value keeps this stamp meaning "the
    -- vendor changed the price", which is how both the catalogue arrow and the
    -- scrape metrics read it.
    price_last_changed_at = case
                               when excluded.has_variants
                                   then vendor_listings.price_last_changed_at
                               when vendor_listings.current_price is not null
                                and excluded.current_price is distinct from vendor_listings.current_price
                                   then now()
                               else vendor_listings.price_last_changed_at
                           end,
    current_price        = case
                               when excluded.has_variants
                                   then vendor_listings.current_price
                               else excluded.current_price
                           end,
    is_delisted          = false,
    delisted_at          = null,
    last_seen_at         = now(),
    updated_at           = now()
returning *;

-- name: GetVendorListingByID :one
select * from vendor_listings where vendor_listing_id = $1;

-- name: GetVendorListingsByIDs :many
-- The roll-up above rewrites price columns, so the scraper re-reads what it
-- just wrote. One query for the whole chunk rather than one per listing.
select * from vendor_listings where vendor_listing_id = any($1::uuid[]);

-- name: MarkUnseenListingsDelisted :execrows
-- Anything not touched since this run started is gone from the vendor's site.
-- Soft delete only: price history and past orders must survive.
update vendor_listings
set is_delisted = true, delisted_at = now(), updated_at = now()
where vendor_id = $1
  and not is_delisted
  and last_seen_at < $2;

-- name: ListTrackedListingsForVendor :many
select * from vendor_listings
where vendor_id = $1 and is_tracked and not is_delisted
order by listing_name;

-- name: SetListingTracked :one
update vendor_listings
set is_tracked = $2, updated_at = now()
where vendor_listing_id = $1
returning *;

-- name: UpsertVariant :batchexec
insert into variants (
    vendor_listing_id, variant_label, external_variant_id, variant_sku,
    is_in_stock, pack_size, current_price, last_seen_at
) values ($1, $2, $3, $4, $5, $6, $7, now())
on conflict (vendor_listing_id, variant_label) do update set
    external_variant_id = coalesce(excluded.external_variant_id, variants.external_variant_id),
    variant_sku         = coalesce(excluded.variant_sku, variants.variant_sku),
    is_in_stock         = excluded.is_in_stock,
    pack_size           = coalesce(excluded.pack_size, variants.pack_size),
    previous_price      = case
                              when excluded.current_price is distinct from variants.current_price
                                  then variants.current_price
                              else variants.previous_price
                          end,
    price_last_changed_at = case
                              when variants.current_price is not null
                               and excluded.current_price is distinct from variants.current_price
                                  then now()
                              else variants.price_last_changed_at
                          end,
    current_price       = excluded.current_price,
    is_delisted         = false,
    delisted_at         = null,
    last_seen_at        = now(),
    updated_at          = now();

-- name: MarkUnseenVariantsDelisted :batchexec
update variants
set is_delisted = true, delisted_at = now(), updated_at = now()
where vendor_listing_id = $1
  and not is_delisted
  and last_seen_at < $2;

-- name: ListVariantsForListing :many
select * from variants
where vendor_listing_id = $1 and not is_delisted
order by variant_label;

-- name: SetListingBasePriceFromVariants :batchexec
-- For a listing with variants the catalogue shows "from X", so the listing's
-- own price tracks the cheapest live variant. This query owns all three price
-- columns for such listings.
update vendor_listings
set previous_price = case
                         when sub.minimum_variant_price is distinct from vendor_listings.current_price
                             then vendor_listings.current_price
                         else vendor_listings.previous_price
                     end,
    price_last_changed_at = case
                         when vendor_listings.current_price is not null
                          and sub.minimum_variant_price is distinct from vendor_listings.current_price
                             then now()
                         else vendor_listings.price_last_changed_at
                     end,
    current_price  = sub.minimum_variant_price,
    updated_at     = now()
from (
    select min(current_price) as minimum_variant_price
    from variants
    where vendor_listing_id = $1 and not is_delisted and current_price is not null
) as sub
where vendor_listings.vendor_listing_id = $1
  and sub.minimum_variant_price is not null;

-- name: GetVariantByID :one
select * from variants where variant_id = $1;

-- name: MarkListingDetailFetched :exec
update vendor_listings set detail_fetched_at = now(), updated_at = now()
where vendor_listing_id = $1;

-- name: ListListingsNeedingDetail :many
-- Listings whose product page has never been read, or was read longest ago.
--
-- Only some vendors keep anything on the product page, so this is called only
-- for those. Never-read listings come first: a listing with no options at all
-- is more wrong than one whose options are a few days old.
select * from vendor_listings
where vendor_id = @vendor_id
  and not is_delisted
  and (detail_fetched_at is null or detail_fetched_at < @stale_before)
order by detail_fetched_at nulls first, listing_name
limit @result_limit;
