-- Records when a listing's own product page was last read.
--
-- Most vendors publish everything the catalogue needs in their feed, so their
-- listings never need a product-page fetch. WooCommerce is different: sizes,
-- per-size prices and the quantity-discount ladder exist only on the product
-- page. Those listings would otherwise show no options at all until the user
-- starred them, which puts the information needed to make that decision behind
-- the decision itself.
--
-- With this column the API can fetch the page the first time someone opens the
-- listing, and then leave it alone until the value goes stale.
alter table vendor_listings
    add column detail_fetched_at timestamptz;

comment on column vendor_listings.detail_fetched_at is
    'When the product page was last read for variants and MOQ tiers; null means never.';
