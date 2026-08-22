-- name: DeleteMoqTiersForListing :exec
-- Tier ladders are three or four rows, so a deep scrape replaces the whole
-- set inside one transaction rather than diffing row by row.
delete from moq_tiers where vendor_listing_id = $1;

-- name: DeleteMoqTiersForVariant :exec
delete from moq_tiers where variant_id = $1;

-- name: InsertListingMoqTier :one
insert into moq_tiers (
    vendor_listing_id, quantity_range_minimum, quantity_range_maximum,
    price_per_unit, discount_percent
) values ($1, $2, $3, $4, $5)
returning *;

-- name: InsertVariantMoqTier :one
insert into moq_tiers (
    variant_id, quantity_range_minimum, quantity_range_maximum,
    price_per_unit, discount_percent
) values ($1, $2, $3, $4, $5)
returning *;

-- name: ListMoqTiersForListing :many
select * from moq_tiers
where vendor_listing_id = $1
order by quantity_range_minimum;

-- name: ListMoqTiersForVariant :many
select * from moq_tiers
where variant_id = $1
order by quantity_range_minimum;

-- name: ListMoqTiersForListingWithFallback :many
-- A listing with variants carries no tiers of its own — each variant has its
-- own ladder (BR-4). The catalogue shows such a listing at its cheapest live
-- variant's price, so the comparison table shows that same variant's ladder
-- rather than an empty cell.
with cheapest_variant as (
    select variants.variant_id
    from variants
    where variants.vendor_listing_id = @vendor_listing_id
      and not variants.is_delisted
      and variants.current_price is not null
    order by variants.current_price
    limit 1
)
select moq_tiers.*
from moq_tiers
where moq_tiers.vendor_listing_id = @vendor_listing_id
   or (moq_tiers.variant_id = (select variant_id from cheapest_variant)
       and not exists (select 1 from moq_tiers existing
                        where existing.vendor_listing_id = @vendor_listing_id))
order by moq_tiers.quantity_range_minimum;
