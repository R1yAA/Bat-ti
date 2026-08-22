-- Queries backing P2: compare entries.

-- name: ListCompareEntries :many
select compare_entries.*,
       (select count(*) from compare_entry_members
         where compare_entry_members.compare_entry_id = compare_entries.compare_entry_id
       ) as member_count
from compare_entries
order by compare_entries.created_at desc;

-- name: GetCompareEntry :one
select * from compare_entries where compare_entry_id = $1;

-- name: CreateCompareEntry :one
insert into compare_entries (entry_name) values ($1) returning *;

-- name: RenameCompareEntry :one
update compare_entries set entry_name = $2, updated_at = now()
where compare_entry_id = $1 returning *;

-- name: DeleteCompareEntry :exec
-- BR-17: only ever manual. Nothing in the system deletes a compare entry.
delete from compare_entries where compare_entry_id = $1;

-- name: ListCompareEntryMembers :many
-- A member points at either a listing or one of its variants. The listing is
-- resolved either way so the comparison table always has a name, image and
-- vendor to show.
select compare_entry_members.compare_entry_member_id,
       compare_entry_members.added_at,
       compare_entry_members.variant_id,
       vendor_listings.vendor_listing_id,
       vendor_listings.listing_name,
       vendor_listings.product_url,
       vendor_listings.primary_image_url,
       vendor_listings.is_in_stock,
       vendor_listings.is_delisted,
       vendor_listings.pack_size    as listing_pack_size,
       vendor_listings.current_price as listing_current_price,
       vendors.vendor_name,
       vendors.vendor_slug,
       variants.variant_label,
       variants.pack_size            as variant_pack_size,
       variants.current_price        as variant_current_price,
       variants.is_in_stock          as variant_is_in_stock
from compare_entry_members
left join variants on variants.variant_id = compare_entry_members.variant_id
join vendor_listings on vendor_listings.vendor_listing_id =
     coalesce(compare_entry_members.vendor_listing_id, variants.vendor_listing_id)
join vendors on vendors.vendor_id = vendor_listings.vendor_id
where compare_entry_members.compare_entry_id = $1
order by vendors.vendor_name, vendor_listings.listing_name;

-- name: AddCompareEntryListingMember :one
-- BR-1: the same listing may sit in many compare entries, so a clash here is
-- only ever the same listing added to the same entry twice.
insert into compare_entry_members (compare_entry_id, vendor_listing_id)
values ($1, $2)
on conflict (compare_entry_id, vendor_listing_id) where vendor_listing_id is not null
do update set added_at = compare_entry_members.added_at
returning *;

-- name: AddCompareEntryVariantMember :one
insert into compare_entry_members (compare_entry_id, variant_id)
values ($1, $2)
on conflict (compare_entry_id, variant_id) where variant_id is not null
do update set added_at = compare_entry_members.added_at
returning *;

-- name: DeleteCompareEntryMember :exec
delete from compare_entry_members
where compare_entry_id = $1 and compare_entry_member_id = $2;
