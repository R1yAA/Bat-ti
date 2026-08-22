-- name: StartScrapeRun :one
insert into scrape_runs (vendor_id) values ($1) returning *;

-- name: FinishScrapeRunSuccess :exec
update scrape_runs
set finished_at        = now(),
    run_status         = 'success',
    listings_seen      = $2,
    listings_updated   = $3,
    listings_delisted  = $4
where scrape_run_id = $1;

-- name: FinishScrapeRunFailure :exec
update scrape_runs
set finished_at   = now(),
    run_status    = 'failed',
    error_message = $2
where scrape_run_id = $1;

-- name: ListRecentScrapeRuns :many
select scrape_runs.*, vendors.vendor_slug, vendors.vendor_name
from scrape_runs
join vendors on vendors.vendor_id = scrape_runs.vendor_id
order by scrape_runs.started_at desc
limit $1;
