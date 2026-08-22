import { useState } from "react";
import { Link, useParams } from "react-router-dom";
import {
  Line,
  LineChart,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from "recharts";
import {
  useAddCompareMember,
  useCompareEntries,
  useCreateCompareEntry,
  useListing,
  useSetListingTracked,
} from "../api/queries";
import type { MoqTier, PricePoint, Variant } from "../api/types";
import {
  Button,
  Card,
  DelistedBadge,
  ErrorNotice,
  PriceArrow,
  RatingPill,
  Sheet,
  Spinner,
  StockBadge,
  TextField,
} from "../components/ui";
import {
  decodeEntities,
  formatDate,
  formatOrderStatus,
  formatRelativeTime,
  formatRupees,
  formatRupeesCompact,
  toNumber,
} from "../lib/format";

export function ListingPage() {
  const { listingID = "" } = useParams<{ listingID: string }>();
  const listingQuery = useListing(listingID);
  const setTracked = useSetListingTracked();
  const [isCompareSheetOpen, setIsCompareSheetOpen] = useState(false);

  if (listingQuery.isPending) return <Spinner label="Loading product" />;
  if (listingQuery.isError) return <ErrorNotice error={listingQuery.error} />;

  const listing = listingQuery.data;
  const hasVariants = listing.variants.length > 0;

  return (
    <div className="space-y-4">
      <Link
        to={`/vendors/${listing.vendor.vendor_slug}`}
        className="inline-flex items-center gap-1 text-sm text-ink-soft hover:text-ink"
      >
        ← {listing.vendor.vendor_name}
      </Link>

      <div className="overflow-hidden rounded-2xl border border-wick-100 bg-surface shadow-sm">
        {listing.primary_image_url && (
          <img
            src={listing.primary_image_url}
            alt=""
            className={`aspect-video w-full bg-surface-sunk object-contain ${
              listing.is_in_stock ? "" : "opacity-50 grayscale"
            }`}
          />
        )}
        <div className="space-y-3 p-4">
          <div className="flex flex-wrap gap-1.5">
            <StockBadge isInStock={listing.is_in_stock} />
            <DelistedBadge isDelisted={listing.is_delisted} />
            {listing.vendor_side_category && (
              <span className="rounded-full bg-wick-100 px-2 py-0.5 text-[11px] font-medium text-wick-800">
                {decodeEntities(listing.vendor_side_category)}
              </span>
            )}
          </div>

          <h1 className="text-xl leading-snug font-semibold text-ink">
            {decodeEntities(listing.listing_name)}
          </h1>

          {!hasVariants && (
            <div className="flex items-baseline gap-2">
              <span className="text-3xl font-semibold text-ink">
                {formatRupees(listing.current_price)}
              </span>
              <PriceArrow direction={listing.price_direction} />
              {listing.previous_price && (
                <span className="text-sm text-ink-faint line-through">
                  {formatRupees(listing.previous_price)}
                </span>
              )}
            </div>
          )}
          {!hasVariants && listing.price_per_unit && (
            <p className="-mt-2 text-sm text-ink-soft">
              {formatRupees(listing.price_per_unit)} per unit
              {listing.pack_size ? ` · pack of ${listing.pack_size}` : ""}
            </p>
          )}

          <div className="flex flex-wrap items-center gap-3">
            <RatingPill
              rating={toNumber(listing.average_rating)}
              count={listing.rating_count}
            />
            <span className="text-xs text-ink-faint">
              Updated{" "}
              {formatRelativeTime(listing.vendor.last_successful_scrape_at)}
            </span>
          </div>

          <div className="flex flex-wrap gap-2 pt-1">
            {/* D1: the star is what turns on deep scraping and price history. */}
            <Button
              variant={listing.is_tracked ? "primary" : "ghost"}
              disabled={setTracked.isPending}
              onClick={() =>
                setTracked.mutate({
                  listingID,
                  isTracked: !listing.is_tracked,
                })
              }
            >
              {listing.is_tracked ? "⭐ Tracking" : "☆ Track price"}
            </Button>
            <Button variant="ghost" onClick={() => setIsCompareSheetOpen(true)}>
              ⚖️ Add to compare
            </Button>
            <a
              href={listing.product_url}
              target="_blank"
              rel="noreferrer noopener"
              className="inline-flex min-h-11 items-center rounded-xl border border-wick-200 bg-surface px-4 text-sm font-medium text-ink-soft hover:bg-wick-50"
            >
              Open on vendor site ↗
            </a>
          </div>

          {!listing.is_tracked && (
            <p className="text-xs text-ink-faint">
              Track this to start recording its price history and quantity
              discounts each day.
            </p>
          )}
        </div>
      </div>

      {hasVariants ? (
        <VariantSections variants={listing.variants} />
      ) : (
        <>
          {listing.moq_tiers.length > 0 && (
            <Card>
              <SectionTitle>Quantity pricing</SectionTitle>
              <MoqTierTable tiers={listing.moq_tiers} />
            </Card>
          )}
          {listing.price_history.length > 1 && (
            <Card>
              <SectionTitle>Price history</SectionTitle>
              <PriceHistoryChart history={listing.price_history} />
            </Card>
          )}
        </>
      )}

      {listing.past_orders.length > 0 && (
        <Card>
          {/* FR-P1-9: scoped to this listing only. */}
          <SectionTitle>Your past orders</SectionTitle>
          <ul className="divide-y divide-wick-100">
            {listing.past_orders.map((order) => (
              <li
                key={order.order_item_id}
                className="flex items-center justify-between gap-3 py-2.5 text-sm"
              >
                <div className="min-w-0">
                  <Link
                    to={`/orders/${order.order_entry_id}`}
                    className="font-medium text-ink hover:text-wick-700"
                  >
                    {order.entry_name ?? "Order"}
                  </Link>
                  <p className="text-xs text-ink-faint">
                    {formatDate(order.ordered_on)}
                    {order.variant_label ? ` · ${order.variant_label}` : ""} ·{" "}
                    {formatOrderStatus(order.order_status)}
                  </p>
                </div>
                <div className="shrink-0 text-right">
                  <p className="font-medium text-ink">
                    {formatRupees(order.price_per_unit)}
                  </p>
                  <p className="text-xs text-ink-faint">×{order.quantity}</p>
                </div>
              </li>
            ))}
          </ul>
        </Card>
      )}

      {listing.description && (
        <Card>
          <SectionTitle>Description</SectionTitle>
          <div
            className="prose-sm max-w-none text-sm leading-relaxed text-ink-soft [&_img]:max-w-full [&_li]:my-1 [&_ul]:list-disc [&_ul]:pl-5"
            // The vendor's own product copy. It is their HTML, rendered as-is.
            dangerouslySetInnerHTML={{ __html: listing.description }}
          />
        </Card>
      )}

      <AddToCompareSheet
        isOpen={isCompareSheetOpen}
        onClose={() => setIsCompareSheetOpen(false)}
        vendorListingID={listingID}
      />
    </div>
  );
}

function SectionTitle({ children }: { children: React.ReactNode }) {
  return (
    <h2 className="mb-3 text-sm font-semibold tracking-wide text-ink-soft uppercase">
      {children}
    </h2>
  );
}

/** BR-4: each variant carries its own tier table and its own history. */
function VariantSections({ variants }: { variants: Variant[] }) {
  const [activeIndex, setActiveIndex] = useState(0);
  const activeVariant = variants[activeIndex];

  return (
    <Card>
      <SectionTitle>Options</SectionTitle>
      <div className="swipe-x -mx-1 mb-4 px-1">
        <div className="flex gap-2 pb-1">
          {variants.map((variant, index) => (
            <button
              key={variant.variant_id}
              onClick={() => setActiveIndex(index)}
              className={`shrink-0 rounded-xl border px-3 py-2 text-sm font-medium whitespace-nowrap transition ${
                index === activeIndex
                  ? "border-wick-500 bg-wick-100 text-wick-800"
                  : "border-wick-200 bg-surface text-ink-soft hover:bg-wick-50"
              } ${variant.is_in_stock ? "" : "opacity-60"}`}
            >
              {variant.variant_label}
            </button>
          ))}
        </div>
      </div>

      <div className="flex items-baseline gap-2">
        <span className="text-2xl font-semibold text-ink">
          {formatRupees(activeVariant.current_price)}
        </span>
        <PriceArrow direction={activeVariant.price_direction} />
        {activeVariant.previous_price && (
          <span className="text-sm text-ink-faint line-through">
            {formatRupees(activeVariant.previous_price)}
          </span>
        )}
      </div>
      {activeVariant.price_per_unit && (
        <p className="text-sm text-ink-soft">
          {formatRupees(activeVariant.price_per_unit)} per unit
          {activeVariant.pack_size ? ` · pack of ${activeVariant.pack_size}` : ""}
        </p>
      )}
      <div className="mt-2">
        <StockBadge isInStock={activeVariant.is_in_stock} />
      </div>

      {activeVariant.moq_tiers.length > 0 && (
        <div className="mt-4">
          <SectionTitle>Quantity pricing</SectionTitle>
          <MoqTierTable tiers={activeVariant.moq_tiers} />
        </div>
      )}

      {activeVariant.price_history.length > 1 && (
        <div className="mt-4">
          <SectionTitle>Price history</SectionTitle>
          <PriceHistoryChart history={activeVariant.price_history} />
        </div>
      )}
    </Card>
  );
}

function MoqTierTable({ tiers }: { tiers: MoqTier[] }) {
  return (
    <div className="swipe-x">
      <table className="w-full min-w-[18rem] text-sm">
        <thead>
          <tr className="text-left text-xs tracking-wide text-ink-faint uppercase">
            <th className="pb-2 font-medium">Quantity</th>
            <th className="pb-2 font-medium">Discount</th>
            <th className="pb-2 text-right font-medium">Price</th>
          </tr>
        </thead>
        <tbody className="divide-y divide-wick-100">
          {tiers.map((tier) => (
            <tr key={tier.quantity_range_minimum}>
              <td className="py-2 text-ink">
                {tier.quantity_range_maximum === null
                  ? `${tier.quantity_range_minimum}+`
                  : `${tier.quantity_range_minimum}–${tier.quantity_range_maximum}`}
              </td>
              <td className="py-2 text-fall">
                {tier.discount_percent
                  ? `${toNumber(tier.discount_percent).toFixed(2)}%`
                  : "—"}
              </td>
              <td className="py-2 text-right font-medium text-ink">
                {formatRupees(tier.price_per_unit)}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

/** FR-P1-7: this vendor's own history for this listing, deliberately
 *  single-vendor. Cross-vendor comparison belongs to P2. */
function PriceHistoryChart({ history }: { history: PricePoint[] }) {
  const points = history.map((point) => ({
    date: point.date,
    price: toNumber(point.price),
  }));

  return (
    <div className="h-48 w-full">
      <ResponsiveContainer width="100%" height="100%">
        <LineChart data={points} margin={{ top: 4, right: 8, bottom: 0, left: 0 }}>
          <XAxis
            dataKey="date"
            tickFormatter={(value: string) => formatDate(value).slice(0, 6)}
            tick={{ fontSize: 11, fill: "#9a877c" }}
            tickLine={false}
            axisLine={false}
            minTickGap={24}
          />
          <YAxis
            tickFormatter={formatRupeesCompact}
            tick={{ fontSize: 11, fill: "#9a877c" }}
            tickLine={false}
            axisLine={false}
            width={52}
            domain={["auto", "auto"]}
          />
          <Tooltip
            formatter={(value) => [formatRupees(String(value)), "Price"]}
            labelFormatter={(label) => formatDate(String(label))}
            contentStyle={{
              borderRadius: 12,
              border: "1px solid #eed3ab",
              fontSize: 13,
            }}
          />
          <Line
            type="monotone"
            dataKey="price"
            stroke="#c97a35"
            strokeWidth={2}
            dot={{ r: 2.5 }}
            activeDot={{ r: 4 }}
          />
        </LineChart>
      </ResponsiveContainer>
    </div>
  );
}

/** FR-P2-2: add to an existing entry, or create one on the fly. */
function AddToCompareSheet({
  isOpen,
  onClose,
  vendorListingID,
}: {
  isOpen: boolean;
  onClose: () => void;
  vendorListingID: string;
}) {
  const entriesQuery = useCompareEntries();
  const addMember = useAddCompareMember();
  const createEntry = useCreateCompareEntry();
  const [newEntryName, setNewEntryName] = useState("");

  const addTo = (entryID: string) => {
    addMember.mutate(
      { entryID, vendorListingID },
      { onSuccess: onClose },
    );
  };

  const createAndAdd = () => {
    const trimmedName = newEntryName.trim();
    if (!trimmedName) return;
    createEntry.mutate(trimmedName, {
      onSuccess: (entry) => {
        setNewEntryName("");
        addTo(entry.compare_entry_id);
      },
    });
  };

  return (
    <Sheet title="Add to comparison" isOpen={isOpen} onClose={onClose}>
      {addMember.isError && <ErrorNotice error={addMember.error} />}

      <div className="space-y-2">
        {entriesQuery.data?.map((entry) => (
          <button
            key={entry.compare_entry_id}
            onClick={() => addTo(entry.compare_entry_id)}
            disabled={addMember.isPending}
            className="flex w-full items-center justify-between rounded-xl border border-wick-200 bg-surface px-4 py-3 text-left transition hover:bg-wick-50 disabled:opacity-50"
          >
            <span className="font-medium text-ink">{entry.entry_name}</span>
            <span className="text-xs text-ink-faint">
              {entry.member_count} items
            </span>
          </button>
        ))}
        {entriesQuery.data?.length === 0 && (
          <p className="text-sm text-ink-soft">
            No comparisons yet — name one below to start.
          </p>
        )}
      </div>

      <div className="mt-4 space-y-2 border-t border-wick-100 pt-4">
        <TextField
          label="New comparison"
          value={newEntryName}
          onChange={setNewEntryName}
          placeholder="e.g. Gift box options"
        />
        <Button
          onClick={createAndAdd}
          disabled={!newEntryName.trim() || createEntry.isPending}
          className="w-full"
        >
          Create and add
        </Button>
      </div>
    </Sheet>
  );
}
