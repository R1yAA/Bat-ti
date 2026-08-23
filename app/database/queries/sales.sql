-- Queries backing the sell-side half of P4: FR-P4-7 (Total Gain) and
-- FR-P4-8 (Sales by Category). The buy-side figures are in spend.sql.
--
-- OPEN-6 assumed, mirroring BR-12: a cancelled sale never happened, so it is
-- left out of the headline figure. Each query also returns the gross number —
-- everything counted — so the page's "include cancelled" toggle is a switch
-- between two figures already in hand rather than a second round trip.
--
-- OPEN-5 stands: this is revenue, not profit. Profit would need each sold
-- product linked back to the materials consumed making it, and no such link
-- exists yet. Every column here is named "gain" or "revenue", never "profit".
--
-- All date filtering uses order_placed_date, never created_at, so a sale
-- logged a fortnight late still lands in the month it happened.

-- name: GetSalesSummary :one
select
    coalesce(sum(
        case when sale_order_entries.order_status <> 'cancelled'
             then sale_order_items.quantity * sale_order_items.price_per_unit
             else 0 end
    ), 0)::numeric(14, 2) as net_gain,
    coalesce(sum(sale_order_items.quantity * sale_order_items.price_per_unit), 0)::numeric(14, 2)
        as gross_gain,
    coalesce(sum(
        case when sale_order_entries.order_status = 'cancelled'
             then sale_order_items.quantity * sale_order_items.price_per_unit
             else 0 end
    ), 0)::numeric(14, 2) as cancelled_gain,
    count(distinct sale_order_entries.sale_order_entry_id) as sale_count,
    count(distinct sale_order_entries.sale_order_entry_id)
        filter (where sale_order_entries.order_status = 'pending') as pending_count
from sale_order_entries
left join sale_order_items
     on sale_order_items.sale_order_entry_id = sale_order_entries.sale_order_entry_id
where sale_order_entries.order_placed_date between @start_date and @end_date;

-- name: GetSalesByCategory :many
-- Simpler than its spend counterpart: BR-22 gives a sale exactly one category,
-- so there is no amount to divide between tags and no "Uncategorized" fallback
-- to synthesise — every sale already has a real category row.
select sale_order_categories.category_name,
       coalesce(sum(
           case when sale_order_entries.order_status <> 'cancelled'
                then sale_order_items.quantity * sale_order_items.price_per_unit
                else 0 end
       ), 0)::numeric(14, 2) as net_gain,
       coalesce(sum(sale_order_items.quantity * sale_order_items.price_per_unit), 0)::numeric(14, 2)
           as gross_gain
from sale_order_entries
join sale_order_categories
     on sale_order_categories.sale_order_category_id = sale_order_entries.sale_order_category_id
left join sale_order_items
     on sale_order_items.sale_order_entry_id = sale_order_entries.sale_order_entry_id
where sale_order_entries.order_placed_date between @start_date and @end_date
group by sale_order_categories.category_name
order by net_gain desc;

-- name: GetMonthlySalesTrend :many
-- The sell-side twin of GetMonthlySpendTrend, and deliberately the same shape:
-- always the trailing twelve months, months with no sales returned as zero so
-- the bars have no gaps. Sharing an x-axis with the spend chart is the whole
-- point — a gain bar is only readable next to the spend bar beside it.
with trailing_months as (
    select generate_series(
        date_trunc('month', current_date) - interval '11 months',
        date_trunc('month', current_date),
        interval '1 month'
    )::date as month_start
)
select trailing_months.month_start,
       coalesce(sum(
           case when sale_order_entries.order_status <> 'cancelled'
                then sale_order_items.quantity * sale_order_items.price_per_unit
                else 0 end
       ), 0)::numeric(14, 2) as net_gain,
       coalesce(sum(sale_order_items.quantity * sale_order_items.price_per_unit), 0)::numeric(14, 2)
           as gross_gain
from trailing_months
left join sale_order_entries
       on date_trunc('month', sale_order_entries.order_placed_date) = trailing_months.month_start
left join sale_order_items
       on sale_order_items.sale_order_entry_id = sale_order_entries.sale_order_entry_id
group by trailing_months.month_start
order by trailing_months.month_start;
