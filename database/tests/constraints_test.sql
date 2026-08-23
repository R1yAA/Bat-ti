-- Schema guarantees that must hold, as a self-asserting test.
--
-- Each case runs inside a savepoint so one expected rejection does not abort
-- the rest, and asserts on SQLSTATE rather than on a message string. A case
-- that reports FAIL means a defect corrected from tech-prd.md has regressed.
--
-- Run with: make db-test

do $$
declare
    fixture_vendor_id        uuid;
    fixture_listing_id       uuid;
    fixture_compare_entry_id uuid;
    fixture_order_entry_id   uuid;
    fixture_sale_category_id uuid;
    fixture_sale_entry_id    uuid;
    failure_count            integer := 0;

    procedure_note           text;
begin
    insert into vendors (vendor_slug, vendor_name, source_base_url, scraper_tier, scrape_hour_utc)
    values ('fixture-vendor', 'Fixture Vendor', 'https://example.test', 'shopify_json', 23)
    returning vendor_id into fixture_vendor_id;

    insert into vendor_listings (vendor_id, product_url, listing_name, current_price)
    values (fixture_vendor_id, 'https://example.test/p/1', 'Fixture Listing', 100.00)
    returning vendor_listing_id into fixture_listing_id;

    insert into compare_entries (entry_name) values ('Fixture Comparison')
    returning compare_entry_id into fixture_compare_entry_id;

    insert into order_entries (entry_name) values ('Fixture Order')
    returning order_entry_id into fixture_order_entry_id;

    insert into sale_order_categories (category_name) values ('Fixture Sale Category')
    returning sale_order_category_id into fixture_sale_category_id;

    insert into sale_order_entries (sale_order_id, consumer_name, sale_order_category_id)
    values (4242, 'Fixture Customer', fixture_sale_category_id)
    returning sale_order_entry_id into fixture_sale_entry_id;

    -- 1 ────────────────────────────────────────────────────────────────────
    begin
        insert into compare_entry_members (compare_entry_id, vendor_listing_id, variant_id)
        values (fixture_compare_entry_id, fixture_listing_id, gen_random_uuid());
        failure_count := failure_count + 1;
        raise notice 'FAIL  1  compare member naming both listing and variant was accepted';
    exception when check_violation then
        raise notice 'PASS  1  compare member naming both listing and variant rejected';
    end;

    -- 2 ────────────────────────────────────────────────────────────────────
    begin
        insert into compare_entry_members (compare_entry_id) values (fixture_compare_entry_id);
        failure_count := failure_count + 1;
        raise notice 'FAIL  2  compare member naming neither was accepted';
    exception when check_violation then
        raise notice 'PASS  2  compare member naming neither rejected';
    end;

    -- 3 ── the case tech-prd.md''s primary key would have made impossible ───
    begin
        insert into compare_entry_members (compare_entry_id, vendor_listing_id)
        values (fixture_compare_entry_id, fixture_listing_id);
        raise notice 'PASS  3  a valid compare member was accepted';
    exception when others then
        failure_count := failure_count + 1;
        raise notice 'FAIL  3  a valid compare member was rejected (%)', sqlerrm;
    end;

    -- 4 ────────────────────────────────────────────────────────────────────
    begin
        insert into compare_entry_members (compare_entry_id, vendor_listing_id)
        values (fixture_compare_entry_id, fixture_listing_id);
        failure_count := failure_count + 1;
        raise notice 'FAIL  4  duplicate compare member was accepted';
    exception when unique_violation then
        raise notice 'PASS  4  duplicate compare member rejected';
    end;

    -- 5 ────────────────────────────────────────────────────────────────────
    begin
        insert into price_history_entries (vendor_listing_id, scraped_at_date, price)
        values (fixture_listing_id, current_date, 100.00);
        raise notice 'PASS  5  first price-history row for today accepted';
    exception when others then
        failure_count := failure_count + 1;
        raise notice 'FAIL  5  first price-history row rejected (%)', sqlerrm;
    end;

    -- 6 ── the defect that would blank out every price-change arrow ────────
    begin
        insert into price_history_entries (vendor_listing_id, scraped_at_date, price)
        values (fixture_listing_id, current_date, 111.00);
        failure_count := failure_count + 1;
        raise notice 'FAIL  6  a second same-day price-history row was accepted';
    exception when unique_violation then
        raise notice 'PASS  6  a second same-day price-history row rejected';
    end;

    -- 7 ── BR-10 ───────────────────────────────────────────────────────────
    begin
        insert into order_items (order_entry_id, vendor_id, vendor_listing_id,
                                 listing_name_snapshot, quantity, price_per_unit, order_status)
        values (fixture_order_entry_id, fixture_vendor_id, fixture_listing_id,
                'Fixture Listing', 5, 100.00, 'refunded');
        failure_count := failure_count + 1;
        raise notice 'FAIL  7  refunded order item without refund_amount was accepted';
    exception when check_violation then
        raise notice 'PASS  7  refunded order item without refund_amount rejected';
    end;

    -- 8 ── BR-10, the other direction ──────────────────────────────────────
    begin
        insert into order_items (order_entry_id, vendor_id, vendor_listing_id,
                                 listing_name_snapshot, quantity, price_per_unit,
                                 order_status, refund_amount)
        values (fixture_order_entry_id, fixture_vendor_id, fixture_listing_id,
                'Fixture Listing', 5, 100.00, 'placed', 50.00);
        failure_count := failure_count + 1;
        raise notice 'FAIL  8  placed order item carrying refund_amount was accepted';
    exception when check_violation then
        raise notice 'PASS  8  placed order item carrying refund_amount rejected';
    end;

    -- 9 ── BR-8, rating scale ──────────────────────────────────────────────
    begin
        insert into order_items (order_entry_id, vendor_id, vendor_listing_id,
                                 listing_name_snapshot, quantity, price_per_unit, rating)
        values (fixture_order_entry_id, fixture_vendor_id, fixture_listing_id,
                'Fixture Listing', 5, 100.00, 11);
        failure_count := failure_count + 1;
        raise notice 'FAIL  9  rating of 11 was accepted';
    exception when check_violation then
        raise notice 'PASS  9  rating outside 1..10 rejected';
    end;

    -- 10 ───────────────────────────────────────────────────────────────────
    begin
        insert into moq_tiers (vendor_listing_id, quantity_range_minimum,
                               quantity_range_maximum, price_per_unit)
        values (fixture_listing_id, 10, 5, 90.00);
        failure_count := failure_count + 1;
        raise notice 'FAIL 10  MOQ tier with maximum below minimum was accepted';
    exception when check_violation then
        raise notice 'PASS 10  MOQ tier with maximum below minimum rejected';
    end;

    -- 11 ── BR-13 depends on this row existing forever ─────────────────────
    begin
        delete from categories where category_name = 'Uncategorized';
        failure_count := failure_count + 1;
        raise notice 'FAIL 11  the Uncategorized system category was deleted';
    exception when restrict_violation then
        raise notice 'PASS 11  deleting the Uncategorized system category rejected';
    end;

    -- 12 ── order history must survive a delisted listing ──────────────────
    begin
        insert into order_items (order_entry_id, vendor_id, vendor_listing_id,
                                 listing_name_snapshot, quantity, price_per_unit)
        values (fixture_order_entry_id, fixture_vendor_id, fixture_listing_id,
                'Fixture Listing', 5, 100.00);
        delete from vendor_listings where vendor_listing_id = fixture_listing_id;
        failure_count := failure_count + 1;
        raise notice 'FAIL 12  a listing referenced by an order item was deleted';
    exception when foreign_key_violation then
        raise notice 'PASS 12  deleting a listing referenced by an order item rejected';
    end;

    -- 13 ───────────────────────────────────────────────────────────────────
    -- BR-20: a delivery date belongs to a delivered order and nowhere else.
    begin
        update sale_order_entries set delivered_date = current_date
        where sale_order_entry_id = fixture_sale_entry_id;
        failure_count := failure_count + 1;
        raise notice 'FAIL 13  a pending sale order accepted a delivered_date';
    exception when check_violation then
        raise notice 'PASS 13  delivered_date on a non-delivered sale order rejected';
    end;

    -- 14 ───────────────────────────────────────────────────────────────────
    begin
        update sale_order_entries set order_status = 'posted'
        where sale_order_entry_id = fixture_sale_entry_id;
        failure_count := failure_count + 1;
        raise notice 'FAIL 14  an unknown sale order status was accepted';
    exception when check_violation then
        raise notice 'PASS 14  unknown sale order status rejected';
    end;

    -- 15 ───────────────────────────────────────────────────────────────────
    -- BR-21: the display number is what a customer is quoted, so two sales
    -- can never carry the same one.
    begin
        insert into sale_order_entries (sale_order_id, consumer_name, sale_order_category_id)
        values (4242, 'Second Fixture Customer', fixture_sale_category_id);
        failure_count := failure_count + 1;
        raise notice 'FAIL 15  a duplicate sale_order_id was accepted';
    exception when unique_violation then
        raise notice 'PASS 15  duplicate sale_order_id rejected';
    end;

    -- 16 ───────────────────────────────────────────────────────────────────
    begin
        insert into sale_order_entries (sale_order_id, consumer_name, sale_order_category_id)
        values (999, 'Short Number Customer', fixture_sale_category_id);
        failure_count := failure_count + 1;
        raise notice 'FAIL 16  a sale_order_id below four digits was accepted';
    exception when check_violation then
        raise notice 'PASS 16  sale_order_id outside 1000..999999 rejected';
    end;

    -- 17 ───────────────────────────────────────────────────────────────────
    -- BR-23 reassigns in the handler before deleting, so the database must
    -- refuse to drop a category that still has sales pointing at it.
    begin
        delete from sale_order_categories
        where sale_order_category_id = fixture_sale_category_id;
        failure_count := failure_count + 1;
        raise notice 'FAIL 17  a sale order category still in use was deleted';
    exception when foreign_key_violation then
        raise notice 'PASS 17  deleting a sale order category still in use rejected';
    end;

    -- 18 ───────────────────────────────────────────────────────────────────
    begin
        delete from sale_order_categories where category_name = 'Uncategorized';
        failure_count := failure_count + 1;
        raise notice 'FAIL 18  the Uncategorized sale order category was deleted';
    exception when restrict_violation then
        raise notice 'PASS 18  deleting the Uncategorized sale order category rejected';
    end;

    if failure_count > 0 then
        raise exception '% schema check(s) FAILED', failure_count;
    end if;

    procedure_note := format('all schema checks passed at %s', now());
    raise notice '%', procedure_note;

    -- Fixtures exist only for the duration of this block.
    raise exception using errcode = 'triggered_action_exception', message = '__rollback_fixtures__';
exception
    when triggered_action_exception then
        raise notice 'fixture data rolled back';
end $$;
