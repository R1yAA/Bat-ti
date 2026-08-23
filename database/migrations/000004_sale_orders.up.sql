-- P6 — the sell side.
--
-- Everything before this migration tracks what Riya buys. This adds what she
-- sells, as a deliberately parallel set of tables rather than a reuse of the
-- purchase ones. The addendum in .claude/agent-prd-v2.md is explicit about
-- why: an `Order Entry` is a purchase from a vendor and a `Sale Order Entry`
-- is a sale to a customer, and collapsing them would make every existing
-- spend figure ambiguous. Rule IDs (BR-19..BR-24, FR-P6-n) refer to that file.

-- ═══════════════════════════════════════════════════════════
-- SALE ORDER CATEGORIES  (BR-23)
-- ═══════════════════════════════════════════════════════════
-- A separate managed list from `categories`. Selling groups ("Wedding
-- favours", "Bulk corporate gifting") answer a different question from
-- raw-material groups ("Wax", "Wicks"), and merging the two lists would put
-- every material in the dropdown for a sale.
create table sale_order_categories (
    sale_order_category_id uuid primary key default gen_random_uuid(),
    category_name          text not null unique,
    -- Same role as categories.is_system: "Uncategorized" is the reassignment
    -- target BR-23 names, so it must always exist.
    is_system              boolean not null default false,
    created_at             timestamptz not null default now()
);

create function reject_system_sale_order_category_deletion() returns trigger as $$
begin
    if old.is_system then
        raise exception 'sale order category "%" is a system category and cannot be deleted',
            old.category_name
            using errcode = 'restrict_violation';
    end if;
    return old;
end;
$$ language plpgsql;

create trigger trg_reject_system_sale_order_category_deletion
    before delete on sale_order_categories
    for each row execute function reject_system_sale_order_category_deletion();

insert into sale_order_categories (category_name, is_system) values ('Uncategorized', true);

-- ═══════════════════════════════════════════════════════════
-- SALE ORDER ENTRIES  (BR-19, BR-20, BR-21, BR-22)
-- ═══════════════════════════════════════════════════════════
create table sale_order_entries (
    sale_order_entry_id    uuid primary key default gen_random_uuid(),
    -- BR-21: the number quoted to a customer, four to six digits, generated at
    -- creation. The primary key stays a UUID; this is a display identifier and
    -- is never used as a foreign key. Unique so two customers can never be
    -- told the same number.
    sale_order_id          integer not null unique
                               check (sale_order_id between 1000 and 999999),
    consumer_name          text not null check (length(trim(consumer_name)) > 0),
    -- The date the customer placed the order, which is not the date the row was
    -- created — the same distinction order_entries.ordered_on makes, and for
    -- the same reason: P4 groups on this.
    order_placed_date      date not null default current_date,
    order_status           text not null default 'pending' check (order_status in (
                               'pending', 'confirmed', 'shipped', 'delivered', 'cancelled'
                           )),
    -- BR-20: only meaningful once the order is delivered. The constraint runs
    -- one way only — a delivered order need not carry a date yet (OPEN-7
    -- assumed non-blocking), but a pending one can never carry one.
    delivered_date         date,
    -- BR-22: exactly one category, not a tag list. RESTRICT because BR-23
    -- reassigns to "Uncategorized" in the handler before deleting, so a
    -- category delete must never silently orphan a sale.
    sale_order_category_id uuid not null
                               references sale_order_categories(sale_order_category_id)
                               on delete restrict,
    created_at             timestamptz not null default now(),
    updated_at             timestamptz not null default now(),
    constraint sale_order_delivered_date_needs_delivered_status check (
        delivered_date is null or order_status = 'delivered'
    )
    -- total_amount is never stored; it is always derived from the items (BR-19).
);

create index idx_sale_order_entries_placed_on on sale_order_entries (order_placed_date);
-- FR-P6-6 filters on status, and the list view sorts by date within it.
create index idx_sale_order_entries_status on sale_order_entries (order_status, order_placed_date desc);
create index idx_sale_order_entries_category on sale_order_entries (sale_order_category_id);

-- ═══════════════════════════════════════════════════════════
-- SALE ORDER ITEMS  (FR-P6-3)
-- ═══════════════════════════════════════════════════════════
-- product_name is free text, not a reference: there is no catalogue of
-- finished goods yet, only of raw materials bought. When one exists, a
-- nullable product reference can be added beside this column without
-- disturbing the sales already recorded.
create table sale_order_items (
    sale_order_item_id  uuid primary key default gen_random_uuid(),
    sale_order_entry_id uuid not null
                            references sale_order_entries(sale_order_entry_id)
                            on delete cascade,
    product_name        text not null check (length(trim(product_name)) > 0),
    quantity            integer not null check (quantity > 0),
    price_per_unit      numeric(12, 2) not null check (price_per_unit >= 0),
    created_at          timestamptz not null default now(),
    updated_at          timestamptz not null default now()
);

create index idx_sale_order_items_entry on sale_order_items (sale_order_entry_id);
