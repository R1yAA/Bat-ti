-- Queries backing P5: categories, occasion tags and the data wipe.

-- name: ListCategories :many
select categories.*,
       (select count(*) from order_item_category_tags
         where order_item_category_tags.category_id = categories.category_id) as usage_count
from categories
order by categories.is_system desc, categories.category_name;

-- name: GetCategoryByName :one
select * from categories where category_name = $1;

-- name: CreateCategory :one
insert into categories (category_name) values ($1) returning *;

-- name: RenameCategory :one
update categories set category_name = $2 where category_id = $1 returning *;

-- name: ReassignCategoryTagsToUncategorized :exec
-- BR-13, first half: every order item tagged with the doomed category picks up
-- "Uncategorized" instead. Deletion is never blocked and no reference is left
-- dangling. Kept in SQL the application drives rather than in a trigger, so the
-- rule is readable in the handler that performs it.
insert into order_item_category_tags (order_item_id, category_id)
select order_item_category_tags.order_item_id,
       (select category_id from categories where category_name = 'Uncategorized')
from order_item_category_tags
where order_item_category_tags.category_id = $1
on conflict do nothing;

-- name: DeleteCategoryTagsForCategory :exec
-- BR-13, second half: drop the old tags now that they have been re-pointed.
delete from order_item_category_tags where category_id = $1;

-- name: DeleteCategory :exec
delete from categories where category_id = $1;

-- name: ListOccasionTags :many
select occasion_tags.*,
       (select count(*) from order_item_occasion_tags
         where order_item_occasion_tags.occasion_tag_id = occasion_tags.occasion_tag_id) as usage_count
from occasion_tags
order by occasion_tags.tag_name;

-- name: CreateOccasionTag :one
insert into occasion_tags (tag_name) values ($1) returning *;

-- name: RenameOccasionTag :one
update occasion_tags set tag_name = $2 where occasion_tag_id = $1 returning *;

-- name: DeleteOccasionTag :exec
delete from occasion_tags where occasion_tag_id = $1;

-- Data wipe (BR-14). Scope is the user's own records only: order entries,
-- compare entries, categories and occasion tags. Vendors, listings, variants,
-- MOQ tiers and price history survive, because they are not the user's data
-- and, unlike orders, price history cannot be re-entered or re-scraped once
-- gone. Order items and tag joins fall with their parents by cascade.

-- name: DeleteAllOrderEntries :exec
delete from order_entries;

-- name: DeleteAllCompareEntries :exec
delete from compare_entries;

-- name: DeleteAllOccasionTags :exec
delete from occasion_tags;

-- name: DeleteAllNonSystemCategories :exec
delete from categories where not is_system;
