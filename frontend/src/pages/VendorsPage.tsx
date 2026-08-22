import { useEffect, useState } from "react";
import { Link, useNavigate, useParams } from "react-router-dom";
import {
  useFindListingByURL,
  useVendorListings,
  useVendors,
} from "../api/queries";
import type { ListingSummary } from "../api/types";
import { PageHeading } from "../components/AppShell";
import {
  Button,
  DelistedBadge,
  EmptyState,
  ErrorNotice,
  PriceArrow,
  Spinner,
  StockBadge,
} from "../components/ui";
import { decodeEntities, formatRelativeTime, formatRupees } from "../lib/format";

const PAGE_SIZE = 40;

export function VendorsPage() {
  const { vendorSlug } = useParams<{ vendorSlug?: string }>();
  const navigate = useNavigate();
  const vendorsQuery = useVendors();

  const [searchText, setSearchText] = useState("");
  const [debouncedSearch, setDebouncedSearch] = useState("");
  const [inStockOnly, setInStockOnly] = useState(false);
  const [offset, setOffset] = useState(0);
  const findListingByURL = useFindListingByURL();
  const [urlLookupMessage, setUrlLookupMessage] = useState<string | null>(null);

  // Typing in the search box should not fire a request per keystroke.
  useEffect(() => {
    const timer = setTimeout(() => {
      setDebouncedSearch(searchText);
      setOffset(0);
    }, 300);
    return () => clearTimeout(timer);
  }, [searchText]);

  // A pasted product link is a request for one specific product, and it may
  // belong to a vendor other than the one being browsed — so it is resolved
  // and navigated to rather than used as a filter.
  useEffect(() => {
    const trimmedSearch = searchText.trim();
    if (!/^https?:\/\//i.test(trimmedSearch)) {
      setUrlLookupMessage(null);
      return;
    }
    const timer = setTimeout(() => {
      setUrlLookupMessage("Looking for that product…");
      findListingByURL.mutate(trimmedSearch, {
        onSuccess: (match) => {
          setUrlLookupMessage(null);
          setSearchText("");
          navigate(`/listings/${match.vendor_listing_id}`);
        },
        onError: () =>
          setUrlLookupMessage(
            "No product in the catalogue has that link. It may not have been scraped yet.",
          ),
      });
    }, 400);
    return () => clearTimeout(timer);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [searchText]);

  // Land on the first vendor rather than an empty screen.
  useEffect(() => {
    if (!vendorSlug && vendorsQuery.data && vendorsQuery.data.length > 0) {
      navigate(`/vendors/${vendorsQuery.data[0].vendor_slug}`, {
        replace: true,
      });
    }
  }, [vendorSlug, vendorsQuery.data, navigate]);

  const listingsQuery = useVendorListings(vendorSlug ?? "", {
    limit: PAGE_SIZE,
    offset,
    search: debouncedSearch,
    inStockOnly,
    includeDelisted: false,
  });

  if (vendorsQuery.isPending) return <Spinner label="Loading vendors" />;
  if (vendorsQuery.isError) return <ErrorNotice error={vendorsQuery.error} />;

  const vendors = vendorsQuery.data ?? [];
  const activeVendor = listingsQuery.data?.vendor;
  const totalCount = listingsQuery.data?.total_count ?? 0;

  return (
    <div>
      <PageHeading
        title="Vendors"
        subtitle={
          activeVendor
            ? // FR-X-3: staleness is always visible where a vendor is chosen.
              `Last updated ${formatRelativeTime(activeVendor.last_successful_scrape_at)}`
            : undefined
        }
      />

      {/* Vendor picker: a horizontal rail beats a dropdown on a phone — every
          option stays visible and reachable with one thumb. */}
      <div className="swipe-x -mx-4 mb-4 px-4">
        <div className="flex gap-2 pb-1">
          {vendors.map((vendor) => (
            <button
              key={vendor.vendor_id}
              onClick={() => {
                setOffset(0);
                navigate(`/vendors/${vendor.vendor_slug}`);
              }}
              className={`shrink-0 rounded-full px-4 py-2 text-sm font-medium whitespace-nowrap transition ${
                vendor.vendor_slug === vendorSlug
                  ? "bg-wick-600 text-white"
                  : "border border-wick-200 bg-surface text-ink-soft hover:bg-wick-50"
              }`}
            >
              {vendor.vendor_name}
            </button>
          ))}
        </div>
      </div>

      <div className="mb-4 flex flex-wrap items-center gap-2">
        <input
          value={searchText}
          onChange={(event) => setSearchText(event.target.value)}
          placeholder="Search by name, or paste a product link"
          className="min-w-0 flex-1 rounded-xl border border-wick-200 bg-surface px-3 py-2.5 text-base outline-none placeholder:text-ink-faint focus:border-wick-500 focus:ring-2 focus:ring-wick-500/20"
        />
        <button
          onClick={() => {
            setInStockOnly((previous) => !previous);
            setOffset(0);
          }}
          className={`min-h-11 shrink-0 rounded-xl border px-3 text-sm font-medium transition ${
            inStockOnly
              ? "border-wick-500 bg-wick-100 text-wick-800"
              : "border-wick-200 bg-surface text-ink-soft"
          }`}
        >
          In stock only
        </button>
      </div>

      {urlLookupMessage && (
        <p className="mb-3 rounded-xl border border-wick-200 bg-wick-50 px-3 py-2 text-sm text-ink-soft">
          {urlLookupMessage}
        </p>
      )}

      {listingsQuery.isError && <ErrorNotice error={listingsQuery.error} />}
      {listingsQuery.isPending && <Spinner label="Loading catalogue" />}

      {listingsQuery.data && listingsQuery.data.listings.length === 0 && (
        <EmptyState
          title="Nothing matches"
          hint={
            debouncedSearch
              ? `No products found for "${debouncedSearch}".`
              : "This vendor has no products recorded yet — it may not have been scraped."
          }
        />
      )}

      {listingsQuery.data && listingsQuery.data.listings.length > 0 && (
        <>
          <p className="mb-2 text-xs text-ink-faint">
            {totalCount.toLocaleString("en-IN")} products
          </p>
          <div className="grid grid-cols-2 gap-3 lg:grid-cols-3">
            {listingsQuery.data.listings.map((listing) => (
              <ListingCard key={listing.vendor_listing_id} listing={listing} />
            ))}
          </div>

          <div className="mt-6 flex items-center justify-between gap-3">
            <Button
              variant="ghost"
              disabled={offset === 0}
              onClick={() => setOffset(Math.max(0, offset - PAGE_SIZE))}
            >
              ← Previous
            </Button>
            <span className="text-xs text-ink-faint">
              {offset + 1}–{Math.min(offset + PAGE_SIZE, totalCount)} of{" "}
              {totalCount}
            </span>
            <Button
              variant="ghost"
              disabled={offset + PAGE_SIZE >= totalCount}
              onClick={() => setOffset(offset + PAGE_SIZE)}
            >
              Next →
            </Button>
          </div>
        </>
      )}
    </div>
  );
}

function ListingCard({ listing }: { listing: ListingSummary }) {
  return (
    <Link
      to={`/listings/${listing.vendor_listing_id}`}
      className="group flex flex-col overflow-hidden rounded-2xl border border-wick-100 bg-surface shadow-sm transition hover:border-wick-300 hover:shadow-md"
    >
      <div className="relative aspect-square bg-surface-sunk">
        {listing.primary_image_url ? (
          <img
            src={listing.primary_image_url}
            alt=""
            loading="lazy"
            /* Out of stock is greyed but never hidden (BR-15). */
            className={`size-full object-cover transition ${
              listing.is_in_stock ? "" : "opacity-45 grayscale"
            }`}
          />
        ) : (
          <div className="grid size-full place-items-center text-2xl text-ink-faint">
            🕯️
          </div>
        )}
        {listing.is_tracked && (
          <span
            className="absolute top-2 right-2 text-sm drop-shadow"
            title="Tracked — price history is being recorded"
          >
            ⭐
          </span>
        )}
      </div>

      <div className="flex flex-1 flex-col gap-1.5 p-3">
        <p className="line-clamp-2 text-sm leading-snug font-medium text-ink">
          {decodeEntities(listing.listing_name)}
        </p>

        <div className="mt-auto">
          <div className="flex items-baseline gap-1.5">
            <span className="text-lg font-semibold text-ink">
              {formatRupees(listing.current_price)}
            </span>
            <PriceArrow direction={listing.price_direction} />
          </div>

          {/* BR-5: the per-unit price sits small beneath the headline price. */}
          {listing.price_per_unit && (
            <p className="text-xs text-ink-soft">
              {formatRupees(listing.price_per_unit)} per unit
              {listing.pack_size ? ` · pack of ${listing.pack_size}` : ""}
            </p>
          )}

          {listing.has_variants && listing.variant_count > 0 && (
            <p className="text-xs text-ink-faint">
              {listing.variant_count} options
            </p>
          )}

          <div className="mt-1.5 flex flex-wrap gap-1">
            <StockBadge isInStock={listing.is_in_stock} />
            <DelistedBadge isDelisted={listing.is_delisted} />
          </div>
        </div>
      </div>
    </Link>
  );
}
