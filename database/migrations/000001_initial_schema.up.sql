-- Bat-ti initial schema.
--
-- Entity definitions and business rules live in .claude/agent-prd.md; rule IDs
-- (BR-n, FR-n) are referenced here rather than restated. Where this schema
-- departs from tech-prd.md section 4, the reason is written inline.
--
-- gen_random_uuid() is core Postgres from version 13 onward, so no extension
-- is required here or on Supabase.

-- ═══════════════════════════════════════════════════════════
-- VENDORS
-- ═══════════════════════════════════════════════════════════
create table vendors (
    vendor_id                        uuid primary key default gen_random_uuid(),
    -- vendor_slug is the stable key shared with config/vendors.go and the
    -- --vendor CLI flag. vendor_id is never written down outside the database.
    vendor_slug                      text not null unique,
    vendor_name                      text not null,
    source_base_url                  text not null,
    scraper_tier                     text not null check (scraper_tier in (
                                         'shopify_json',
                                         'woocommerce_json',
                                         'dotpe_json',
                                         'static_html',
                                         'manual',
                                         'playwright'
                                     )),
    -- Staggered daily slot: one vendor per hour, scraped sequentially.
    scrape_hour_utc                  smallint not null check (scrape_hour_utc between 0 and 23),
    last_successful_scrape_timestamp timestamptz,
    -- Attempt and error are separate from success so a silently failing
    -- scraper is visible rather than just looking stale (FR-P1-1).
    last_scrape_attempt_timestamp    timestamptz,
    last_scrape_error                text,
    created_at                       timestamptz not null default now(),
    updated_at                       timestamptz not null default now()
);

-- ═══════════════════════════════════════════════════════════
-- VENDOR LISTINGS  (one product page on one vendor's site)
-- ═══════════════════════════════════════════════════════════
create table vendor_listings (
    vendor_listing_id    uuid primary key default gen_random_uuid(),
    vendor_id            uuid not null references vendors(vendor_id) on delete cascade,
    product_url          text not null,
    -- Platform-native id (Shopify product id, Woo product id, DotPe item id).
    -- Matching on this survives a vendor renaming a product or its handle.
    external_product_id  text,
    listing_name         text not null,
    description          text,
    primary_image_url    text,
    vendor_side_category text,
    vendor_side_sku      text,
    is_in_stock          boolean not null default true,
    has_variants         boolean not null default false,
    -- The star: catalogue sync covers every listing cheaply, but MOQ tiers and
    -- daily price history are recorded only for tracked listings.
    is_tracked           boolean not null default false,
    -- Soft delete. A vendor pulling a product must never destroy its price
    -- history or break an order item that references it.
    is_delisted          boolean not null default false,
    delisted_at          timestamptz,
    last_seen_at         timestamptz not null default now(),
    -- Null unless the listing is sold in packs (e.g. Restokart "Pack of 100").
    -- Per-unit price = tier price / pack_size (BR-5).
    pack_size            integer check (pack_size is null or pack_size > 0),
    -- Denormalised so the catalogue can draw a price-change arrow (FR-P1-2) on
    -- every listing, not only the tracked ones that have full price history.
    -- previous_price moves only when the value actually changes, so an arrow
    -- always means a real change rather than "we scraped again" (BR-16).
    current_price        numeric(12, 2),
    previous_price       numeric(12, 2),
    -- When previous_price last advanced. The catalogue arrow (FR-P1-2) is
    -- drawn from current vs previous; this says when that change happened, so
    -- a stale arrow is distinguishable from a fresh one and a scrape run can
    -- count what it actually changed.
    price_last_changed_at timestamptz,
    created_at           timestamptz not null default now(),
    updated_at           timestamptz not null default now(),
    unique (vendor_id, product_url)
);

create unique index uq_vendor_listings_external_product
    on vendor_listings (vendor_id, external_product_id)
    where external_product_id is not null;

create index idx_vendor_listings_live
    on vendor_listings (vendor_id)
    where not is_delisted;

create index idx_vendor_listings_tracked
    on vendor_listings (vendor_id)
    where is_tracked and not is_delisted;

-- ═══════════════════════════════════════════════════════════
-- VARIANTS  (only when the parent listing has has_variants = true)
-- ═══════════════════════════════════════════════════════════
create table variants (
    variant_id          uuid primary key default gen_random_uuid(),
    vendor_listing_id   uuid not null references vendor_listings(vendor_listing_id) on delete cascade,
    variant_label       text not null,
    external_variant_id text,
    variant_sku         text,
    -- Shopify and the WooCommerce Store API both report availability per
    -- variant: one size can be out of stock while another is not.
    is_in_stock         boolean not null default true,
    is_delisted         boolean not null default false,
    delisted_at         timestamptz,
    last_seen_at        timestamptz not null default now(),
    pack_size           integer check (pack_size is null or pack_size > 0),
    current_price       numeric(12, 2),
    previous_price      numeric(12, 2),
    price_last_changed_at timestamptz,
    created_at          timestamptz not null default now(),
    updated_at          timestamptz not null default now(),
    -- Label uniqueness keeps upserts deterministic for vendors that expose no
    -- stable variant id.
    unique (vendor_listing_id, variant_label)
);

create unique index uq_variants_external_variant
    on variants (vendor_listing_id, external_variant_id)
    where external_variant_id is not null;

-- ═══════════════════════════════════════════════════════════
-- MOQ TIERS  (quantity-based pricing rows — BR-4, BR-5)
-- Belongs to either a variant OR a listing directly, never both.
-- ═══════════════════════════════════════════════════════════
create table moq_tiers (
    moq_tier_id            uuid primary key default gen_random_uuid(),
    vendor_listing_id      uuid references vendor_listings(vendor_listing_id) on delete cascade,
    variant_id             uuid references variants(variant_id) on delete cascade,
    quantity_range_minimum integer not null check (quantity_range_minimum > 0),
    -- Null means "and above", e.g. the "100+" row.
    quantity_range_maximum integer,
    price_per_unit         numeric(12, 2) not null check (price_per_unit >= 0),
    discount_percent       numeric(5, 2),
    created_at             timestamptz not null default now(),
    constraint moq_tier_targets_exactly_one_owner check (
        (vendor_listing_id is not null and variant_id is null)
        or (vendor_listing_id is null and variant_id is not null)
    ),
    constraint moq_tier_range_is_ordered check (
        quantity_range_maximum is null
        or quantity_range_maximum >= quantity_range_minimum
    )
);

create unique index uq_moq_tier_listing_minimum
    on moq_tiers (vendor_listing_id, quantity_range_minimum)
    where vendor_listing_id is not null;

create unique index uq_moq_tier_variant_minimum
    on moq_tiers (variant_id, quantity_range_minimum)
    where variant_id is not null;

-- ═══════════════════════════════════════════════════════════
-- PRICE HISTORY  (BR-3, BR-4)
-- ═══════════════════════════════════════════════════════════
create table price_history_entries (
    price_history_entry_id uuid primary key default gen_random_uuid(),
    vendor_listing_id      uuid references vendor_listings(vendor_listing_id) on delete cascade,
    variant_id             uuid references variants(variant_id) on delete cascade,
    scraped_at_date        date not null,
    price                  numeric(12, 2) not null check (price >= 0),
    created_at             timestamptz not null default now(),
    constraint price_history_targets_exactly_one_owner check (
        (vendor_listing_id is not null and variant_id is null)
        or (vendor_listing_id is null and variant_id is not null)
    )
);

-- One row per target per day, enforced rather than assumed.
--
-- Without this, a retry, a manual re-run, or a GitHub Actions schedule delayed
-- across an hour boundary writes a second row for the same date. BR-16 reads
-- the two most recent rows to decide the price-change arrow, so two same-day
-- rows would silently blank out every arrow in the catalogue.
--
-- Writers use ON CONFLICT DO UPDATE. This respects BR-3's append-only intent:
-- earlier days are never touched, only today's row is corrected.
create unique index uq_price_history_listing_date
    on price_history_entries (vendor_listing_id, scraped_at_date)
    where vendor_listing_id is not null;

create unique index uq_price_history_variant_date
    on price_history_entries (variant_id, scraped_at_date)
    where variant_id is not null;

create index idx_price_history_date
    on price_history_entries (scraped_at_date);

-- ═══════════════════════════════════════════════════════════
-- COMPARE ENTRIES  (BR-1, BR-2, BR-17)
-- ═══════════════════════════════════════════════════════════
create table compare_entries (
    compare_entry_id uuid primary key default gen_random_uuid(),
    entry_name       text not null,
    created_at       timestamptz not null default now(),
    updated_at       timestamptz not null default now()
);

-- tech-prd.md gives this table the primary key
--   (compare_entry_id, vendor_listing_id, variant_id)
-- which cannot work: Postgres makes every primary-key column implicitly NOT
-- NULL, while the CHECK below requires exactly one of the two target columns
-- to be NULL. Every insert would fail. A surrogate key plus partial unique
-- indexes gives the intended "no duplicate member per entry" guarantee.
create table compare_entry_members (
    compare_entry_member_id uuid primary key default gen_random_uuid(),
    compare_entry_id        uuid not null references compare_entries(compare_entry_id) on delete cascade,
    vendor_listing_id       uuid references vendor_listings(vendor_listing_id) on delete cascade,
    variant_id              uuid references variants(variant_id) on delete cascade,
    added_at                timestamptz not null default now(),
    constraint compare_member_targets_exactly_one_owner check (
        (vendor_listing_id is not null and variant_id is null)
        or (vendor_listing_id is null and variant_id is not null)
    )
);

create unique index uq_compare_member_listing
    on compare_entry_members (compare_entry_id, vendor_listing_id)
    where vendor_listing_id is not null;

create unique index uq_compare_member_variant
    on compare_entry_members (compare_entry_id, variant_id)
    where variant_id is not null;

-- ═══════════════════════════════════════════════════════════
-- CATEGORIES  (BR-13) and OCCASION TAGS
-- ═══════════════════════════════════════════════════════════
create table categories (
    category_id   uuid primary key default gen_random_uuid(),
    category_name text not null unique,
    -- "Uncategorized" is the reassignment target for BR-13 and must always
    -- exist; deleting it would break that rule permanently.
    is_system     boolean not null default false,
    created_at    timestamptz not null default now()
);

create function reject_system_category_deletion() returns trigger as $$
begin
    if old.is_system then
        raise exception 'category "%" is a system category and cannot be deleted', old.category_name
            using errcode = 'restrict_violation';
    end if;
    return old;
end;
$$ language plpgsql;

create trigger trg_reject_system_category_deletion
    before delete on categories
    for each row execute function reject_system_category_deletion();

insert into categories (category_name, is_system) values ('Uncategorized', true);

create table occasion_tags (
    occasion_tag_id uuid primary key default gen_random_uuid(),
    tag_name        text not null unique,
    created_at      timestamptz not null default now()
);

-- ═══════════════════════════════════════════════════════════
-- ORDER ENTRIES  (BR-6, BR-9)
-- ═══════════════════════════════════════════════════════════
create table order_entries (
    order_entry_id uuid primary key default gen_random_uuid(),
    -- Optional label for the ordering session, e.g. "Diwali restock".
    entry_name     text,
    -- The date the order was actually placed, which is not the date the row
    -- was created: an order placed on the 5th may be logged on the 20th. All
    -- P4 spend reporting filters on this column, never on created_at.
    ordered_on     date not null default current_date,
    created_at     timestamptz not null default now(),
    updated_at     timestamptz not null default now()
    -- total_cost is never stored; it is always derived from order_items (BR-9).
);

create index idx_order_entries_ordered_on on order_entries (ordered_on);

-- ═══════════════════════════════════════════════════════════
-- ORDER ITEMS  (BR-6, BR-7, BR-10, BR-11)
-- ═══════════════════════════════════════════════════════════
create table order_items (
    order_item_id          uuid primary key default gen_random_uuid(),
    order_entry_id         uuid not null references order_entries(order_entry_id) on delete cascade,
    -- RESTRICT, not CASCADE: purchase history must never disappear because a
    -- catalogue row was removed. Listings are soft-deleted (is_delisted), so
    -- in normal operation nothing is ever deleted here anyway.
    vendor_listing_id      uuid references vendor_listings(vendor_listing_id) on delete restrict,
    variant_id             uuid references variants(variant_id) on delete restrict,
    vendor_id              uuid not null references vendors(vendor_id) on delete restrict,
    -- The listing name as it read when the order was placed, so past orders
    -- stay legible after a vendor renames the product.
    listing_name_snapshot  text not null,
    quantity               integer not null check (quantity > 0),
    -- Entered by the user, not copied from the current scraped price (BR-7).
    price_per_unit         numeric(12, 2) not null check (price_per_unit >= 0),
    order_status           text not null default 'placed' check (order_status in (
                               'placed', 'cancelled', 'refunded', 'partially_refunded'
                           )),
    refund_amount          numeric(12, 2) check (refund_amount is null or refund_amount >= 0),
    -- Nullable until rated. Any item is ratable regardless of status, and
    -- every rating counts toward the listing's aggregate (BR-8a, BR-11).
    rating                 integer check (rating is null or rating between 1 and 10),
    created_at             timestamptz not null default now(),
    updated_at             timestamptz not null default now(),
    -- BR-10 enforced here rather than only in the handler: refund_amount is
    -- required for refunded statuses and meaningless otherwise.
    constraint order_item_refund_amount_matches_status check (
        (order_status in ('refunded', 'partially_refunded') and refund_amount is not null)
        or (order_status in ('placed', 'cancelled') and refund_amount is null)
    )
);

create index idx_order_items_entry on order_items (order_entry_id);
create index idx_order_items_listing on order_items (vendor_listing_id);
create index idx_order_items_variant on order_items (variant_id);

-- ═══════════════════════════════════════════════════════════
-- ORDER ITEM TAGS  (FR-P3-8 — two independent tag types)
-- ═══════════════════════════════════════════════════════════
create table order_item_category_tags (
    order_item_id uuid not null references order_items(order_item_id) on delete cascade,
    -- RESTRICT: BR-13 reassigns referencing rows to "Uncategorized" in the
    -- application layer before the category row is deleted, so a category
    -- delete must never silently drop tags.
    category_id   uuid not null references categories(category_id) on delete restrict,
    primary key (order_item_id, category_id)
);

create index idx_order_item_category_tags_category on order_item_category_tags (category_id);

create table order_item_occasion_tags (
    order_item_id   uuid not null references order_items(order_item_id) on delete cascade,
    occasion_tag_id uuid not null references occasion_tags(occasion_tag_id) on delete cascade,
    primary key (order_item_id, occasion_tag_id)
);

-- ═══════════════════════════════════════════════════════════
-- SCRAPE RUNS  (observability — both PRDs flag silent scrape failure as a risk)
-- ═══════════════════════════════════════════════════════════
create table scrape_runs (
    scrape_run_id     uuid primary key default gen_random_uuid(),
    vendor_id         uuid not null references vendors(vendor_id) on delete cascade,
    started_at        timestamptz not null default now(),
    finished_at       timestamptz,
    run_status        text not null default 'running' check (run_status in ('running', 'success', 'failed')),
    listings_seen     integer not null default 0,
    listings_updated  integer not null default 0,
    listings_delisted integer not null default 0,
    error_message     text
);

create index idx_scrape_runs_vendor_started on scrape_runs (vendor_id, started_at desc);
