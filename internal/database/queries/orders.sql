-- Queries backing P3: order entries and order items.

-- name: ListOrderEntries :many
-- BR-9: total_cost is never stored, always derived.
select order_entries.*,
       coalesce(sum(order_items.quantity * order_items.price_per_unit), 0)::numeric(14, 2) as total_cost,
       count(order_items.order_item_id)                                                    as item_count
from order_entries
left join order_items on order_items.order_entry_id = order_entries.order_entry_id
group by order_entries.order_entry_id
order by order_entries.ordered_on desc, order_entries.created_at desc;

-- name: GetOrderEntry :one
select order_entries.*,
       coalesce(sum(order_items.quantity * order_items.price_per_unit), 0)::numeric(14, 2) as total_cost,
       count(order_items.order_item_id)                                                    as item_count
from order_entries
left join order_items on order_items.order_entry_id = order_entries.order_entry_id
where order_entries.order_entry_id = $1
group by order_entries.order_entry_id;

-- name: CreateOrderEntry :one
insert into order_entries (entry_name, ordered_on) values ($1, $2) returning *;

-- name: UpdateOrderEntry :one
update order_entries set entry_name = $2, ordered_on = $3, updated_at = now()
where order_entry_id = $1 returning *;

-- name: DeleteOrderEntry :exec
delete from order_entries where order_entry_id = $1;

-- name: ListOrderItemsForEntry :many
select order_items.*,
       vendors.vendor_name,
       vendors.vendor_slug,
       variants.variant_label,
       vendor_listings.product_url,
       vendor_listings.primary_image_url
from order_items
join vendors on vendors.vendor_id = order_items.vendor_id
left join variants on variants.variant_id = order_items.variant_id
left join vendor_listings on vendor_listings.vendor_listing_id = order_items.vendor_listing_id
where order_items.order_entry_id = $1
order by order_items.created_at;

-- name: GetOrderItem :one
select * from order_items where order_item_id = $1;

-- name: CreateOrderItem :one
-- BR-7: price_per_unit is what was actually paid, entered by the user, never
-- copied from the current scraped price.
insert into order_items (
    order_entry_id, vendor_listing_id, variant_id, vendor_id,
    listing_name_snapshot, quantity, price_per_unit,
    order_status, refund_amount, rating
) values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
returning *;

-- name: UpdateOrderItem :one
update order_items set
    vendor_listing_id     = $2,
    variant_id            = $3,
    vendor_id             = $4,
    listing_name_snapshot = $5,
    quantity              = $6,
    price_per_unit        = $7,
    order_status          = $8,
    refund_amount         = $9,
    rating                = $10,
    updated_at            = now()
where order_item_id = $1
returning *;

-- name: DeleteOrderItem :exec
delete from order_items where order_item_id = $1;

-- name: ClearOrderItemCategoryTags :exec
delete from order_item_category_tags where order_item_id = $1;

-- name: AddOrderItemCategoryTag :exec
insert into order_item_category_tags (order_item_id, category_id)
values ($1, $2) on conflict do nothing;

-- name: ClearOrderItemOccasionTags :exec
delete from order_item_occasion_tags where order_item_id = $1;

-- name: AddOrderItemOccasionTag :exec
insert into order_item_occasion_tags (order_item_id, occasion_tag_id)
values ($1, $2) on conflict do nothing;

-- name: ListCategoryTagsForOrderItem :many
select categories.* from categories
join order_item_category_tags on order_item_category_tags.category_id = categories.category_id
where order_item_category_tags.order_item_id = $1
order by categories.category_name;

-- name: ListOccasionTagsForOrderItem :many
select occasion_tags.* from occasion_tags
join order_item_occasion_tags on order_item_occasion_tags.occasion_tag_id = occasion_tags.occasion_tag_id
where order_item_occasion_tags.order_item_id = $1
order by occasion_tags.tag_name;
