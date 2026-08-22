-- name: UpsertVendorFromConfig :one
-- config/vendors.go is the source of truth for the registry; this keeps the
-- vendors table in step with it on every run so the two never drift.
insert into vendors (vendor_slug, vendor_name, source_base_url, scraper_tier, scrape_hour_utc)
values ($1, $2, $3, $4, $5)
on conflict (vendor_slug) do update set
    vendor_name     = excluded.vendor_name,
    source_base_url = excluded.source_base_url,
    scraper_tier    = excluded.scraper_tier,
    scrape_hour_utc = excluded.scrape_hour_utc,
    updated_at      = now()
returning *;

-- name: GetVendorBySlug :one
select * from vendors where vendor_slug = $1;

-- name: ListVendors :many
select * from vendors order by vendor_name;

-- name: ListVendorsDueForScrape :many
-- A vendor is due when it has not had a successful run in ~a day AND either
-- its hour slot is the current hour, or it is overdue enough that it clearly
-- missed its slot.
--
-- The generous windows are deliberate. GitHub Actions delays scheduled runs
-- under load, so matching the hour exactly would silently skip a vendor whose
-- slot boundary was crossed while the runner was queued; the 26-hour catch-up
-- arm picks it up on the next hourly tick instead of a day later.
select * from vendors
where scraper_tier <> 'manual'
  and (
        last_successful_scrape_timestamp is null
     or last_successful_scrape_timestamp < now() - interval '23 hours'
      )
  and (
        last_successful_scrape_timestamp is null
     or extract(hour from (now() at time zone 'utc'))::int = scrape_hour_utc
     or last_successful_scrape_timestamp < now() - interval '26 hours'
      )
order by scrape_hour_utc;

-- name: MarkVendorScrapeAttempt :exec
update vendors
set last_scrape_attempt_timestamp = now(), updated_at = now()
where vendor_id = $1;

-- name: MarkVendorScrapeSuccess :exec
update vendors
set last_successful_scrape_timestamp = now(), last_scrape_error = null, updated_at = now()
where vendor_id = $1;

-- name: MarkVendorScrapeFailure :exec
update vendors
set last_scrape_error = $2, updated_at = now()
where vendor_id = $1;
