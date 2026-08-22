-- name: UpsertListingPriceHistory :exec
-- One row per listing per day. The upsert respects BR-3's append-only intent:
-- earlier days are never touched, only today's row is corrected if the same
-- day is scraped twice.
insert into price_history_entries (vendor_listing_id, scraped_at_date, price)
values ($1, $2, $3)
on conflict (vendor_listing_id, scraped_at_date) where vendor_listing_id is not null
do update set price = excluded.price;

-- name: UpsertVariantPriceHistory :exec
insert into price_history_entries (variant_id, scraped_at_date, price)
values ($1, $2, $3)
on conflict (variant_id, scraped_at_date) where variant_id is not null
do update set price = excluded.price;

-- name: ListPriceHistoryForListing :many
select scraped_at_date, price
from price_history_entries
where vendor_listing_id = $1
order by scraped_at_date;

-- name: ListPriceHistoryForVariant :many
select scraped_at_date, price
from price_history_entries
where variant_id = $1
order by scraped_at_date;
