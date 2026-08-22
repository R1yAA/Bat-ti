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
