-- Stars every candle already in the catalogue.
--
-- From here on config.ShouldAutoStar stars a matching listing as the scrape
-- upserts it, but that reaches a listing only when its vendor's next slot comes
-- round. This catches up what is already stored, so candle price history starts
-- today rather than a day from now.
--
-- The term is spelled out here rather than read from config.AutoStarTitleTerms
-- on purpose: a migration records what was done on the day it ran, and must not
-- change meaning later when that list is edited.
--
-- Delisted listings are left alone. There is no price to follow on a product
-- the vendor has pulled, and if it returns the scrape stars it then.
update vendor_listings
set is_tracked = true, updated_at = now()
where not is_tracked
  and not is_delisted
  and listing_name ilike '%candle%';
