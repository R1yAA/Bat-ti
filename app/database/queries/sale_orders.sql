-- Queries backing P6: sale order entries, the items inside them, and the
-- category list they are filed under. The purchase-side equivalents live in
-- orders.sql; the two are deliberately never joined.

-- name: ListSaleOrderEntries :many
-- BR-19: total_amount is never stored, always derived.
--
-- FR-P6-6 asks for a "pending only" filter. It is a parameter rather than a
-- second query so the list and its filtered form cannot drift apart. Passing
-- an empty status means no filter at all.
select sale_order_entries.*,
       sale_order_categories.category_name,
       coalesce(sum(sale_order_items.quantity * sale_order_items.price_per_unit), 0)::numeric(14, 2)
           as total_amount,
       count(sale_order_items.sale_order_item_id) as item_count
from sale_order_entries
join sale_order_categories
     on sale_order_categories.sale_order_category_id = sale_order_entries.sale_order_category_id
left join sale_order_items
     on sale_order_items.sale_order_entry_id = sale_order_entries.sale_order_entry_id
where @status_filter::text = '' or sale_order_entries.order_status = @status_filter::text
group by sale_order_entries.sale_order_entry_id, sale_order_categories.category_name
order by sale_order_entries.order_placed_date desc, sale_order_entries.created_at desc;

-- name: GetSaleOrderEntry :one
select sale_order_entries.*,
       sale_order_categories.category_name,
       coalesce(sum(sale_order_items.quantity * sale_order_items.price_per_unit), 0)::numeric(14, 2)
           as total_amount,
       count(sale_order_items.sale_order_item_id) as item_count
from sale_order_entries
join sale_order_categories
     on sale_order_categories.sale_order_category_id = sale_order_entries.sale_order_category_id
left join sale_order_items
     on sale_order_items.sale_order_entry_id = sale_order_entries.sale_order_entry_id
where sale_order_entries.sale_order_entry_id = $1
group by sale_order_entries.sale_order_entry_id, sale_order_categories.category_name;

-- name: CreateSaleOrderEntry :one
-- BR-21: sale_order_id is supplied by the handler, which picks a free four-to
-- six-digit number and retries if the unique index rejects it.
insert into sale_order_entries (
    sale_order_id, consumer_name, order_placed_date,
    order_status, delivered_date, sale_order_category_id
) values ($1, $2, $3, $4, $5, $6)
returning *;

-- name: UpdateSaleOrderEntry :one
-- sale_order_id is deliberately absent: it is the number the customer was
-- given, so it is assigned once and never edited (BR-21).
update sale_order_entries set
    consumer_name          = $2,
    order_placed_date      = $3,
    order_status           = $4,
    delivered_date         = $5,
    sale_order_category_id = $6,
    updated_at             = now()
where sale_order_entry_id = $1
returning *;

-- name: DeleteSaleOrderEntry :exec
delete from sale_order_entries where sale_order_entry_id = $1;

-- name: SaleOrderIDExists :one
select exists (select 1 from sale_order_entries where sale_order_id = $1);

-- name: ListSaleOrderItemsForEntry :many
select * from sale_order_items
where sale_order_entry_id = $1
order by created_at;

-- name: GetSaleOrderItem :one
select * from sale_order_items where sale_order_item_id = $1;

-- name: CreateSaleOrderItem :one
insert into sale_order_items (sale_order_entry_id, product_name, quantity, price_per_unit)
values ($1, $2, $3, $4)
returning *;

-- name: UpdateSaleOrderItem :one
update sale_order_items set
    product_name   = $2,
    quantity       = $3,
    price_per_unit = $4,
    updated_at     = now()
where sale_order_item_id = $1
returning *;

-- name: DeleteSaleOrderItem :exec
delete from sale_order_items where sale_order_item_id = $1;

-- ── Sale order categories (FR-P5-4, BR-23) ───────────────────────────────

-- name: ListSaleOrderCategories :many
select sale_order_categories.*,
       (select count(*) from sale_order_entries
         where sale_order_entries.sale_order_category_id
               = sale_order_categories.sale_order_category_id) as usage_count
from sale_order_categories
order by sale_order_categories.is_system desc, sale_order_categories.category_name;

-- name: GetSaleOrderCategoryByName :one
select * from sale_order_categories where category_name = $1;

-- name: CreateSaleOrderCategory :one
insert into sale_order_categories (category_name) values ($1) returning *;

-- name: RenameSaleOrderCategory :one
update sale_order_categories set category_name = $2
where sale_order_category_id = $1 returning *;

-- name: ReassignSaleOrdersToUncategorized :exec
-- BR-23, mirroring BR-13: deletion is never blocked. Every sale filed under
-- the doomed category moves to "Uncategorized" first, so nothing dangles and
-- no sale loses its place in the by-category chart.
update sale_order_entries
set sale_order_category_id = (
        select uncategorized.sale_order_category_id
        from sale_order_categories as uncategorized
        where uncategorized.category_name = 'Uncategorized'
    ),
    updated_at = now()
where sale_order_entries.sale_order_category_id = $1;

-- name: DeleteSaleOrderCategory :exec
delete from sale_order_categories where sale_order_category_id = $1;

-- Data wipe (BR-14). Sales are the user's own records exactly as purchases
-- are, so a wipe that left them behind would not be the wipe it promises.

-- name: DeleteAllSaleOrderEntries :exec
delete from sale_order_entries;

-- name: DeleteAllNonSystemSaleOrderCategories :exec
delete from sale_order_categories where not is_system;
