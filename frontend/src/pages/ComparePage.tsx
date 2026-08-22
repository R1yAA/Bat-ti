import { useState } from "react";
import { Link, useParams } from "react-router-dom";
import { Line, LineChart, ResponsiveContainer } from "recharts";
import {
  useCompareEntries,
  useCompareEntry,
  useCreateCompareEntry,
  useDeleteCompareEntry,
  useDeleteCompareMember,
  useRenameCompareEntry,
} from "../api/queries";
import type { CompareMember } from "../api/types";
import { PageHeading } from "../components/AppShell";
import { useConfirm } from "../components/ConfirmDialog";
import {
  Button,
  EmptyState,
  ErrorNotice,
  RatingPill,
  Sheet,
  Spinner,
  StockBadge,
  TextField,
} from "../components/ui";
import {
  decodeEntities,
  formatDate,
  formatRupees,
  toNumber,
} from "../lib/format";

export function CompareListPage() {
  const entriesQuery = useCompareEntries();
  const createEntry = useCreateCompareEntry();
  const deleteEntry = useDeleteCompareEntry();
  const confirm = useConfirm();
  const [isSheetOpen, setIsSheetOpen] = useState(false);
  const [newEntryName, setNewEntryName] = useState("");

  if (entriesQuery.isPending) return <Spinner label="Loading comparisons" />;
  if (entriesQuery.isError) return <ErrorNotice error={entriesQuery.error} />;

  const entries = entriesQuery.data ?? [];

  return (
    <div>
      <PageHeading
        title="Compare"
        subtitle="Side-by-side groups you build yourself"
        action={<Button onClick={() => setIsSheetOpen(true)}>+ New</Button>}
      />

      {entries.length === 0 ? (
        <EmptyState
          title="No comparisons yet"
          hint="Create one, then add products to it from any vendor's catalogue. The same product can sit in several comparisons at once."
          action={
            <Button onClick={() => setIsSheetOpen(true)}>
              Create a comparison
            </Button>
          }
        />
      ) : (
        <div className="space-y-2">
          {entries.map((entry) => (
            <div
              key={entry.compare_entry_id}
              className="flex items-center gap-3 rounded-2xl border border-wick-100 bg-surface p-4 shadow-sm"
            >
              <Link
                to={`/compare/${entry.compare_entry_id}`}
                className="min-w-0 flex-1"
              >
                <p className="truncate font-medium text-ink">
                  {entry.entry_name}
                </p>
                <p className="text-xs text-ink-faint">
                  {entry.member_count}{" "}
                  {entry.member_count === 1 ? "product" : "products"}
                </p>
              </Link>
              <button
                onClick={async () => {
                  const isConfirmed = await confirm({
                    title: `Delete "${entry.entry_name}"?`,
                    message:
                      "The comparison is removed. The products in it are not affected.",
                  });
                  if (isConfirmed) deleteEntry.mutate(entry.compare_entry_id);
                }}
                aria-label={`Delete ${entry.entry_name}`}
                className="grid size-10 shrink-0 place-items-center rounded-xl text-ink-faint hover:bg-rise/10 hover:text-rise"
              >
                🗑
              </button>
            </div>
          ))}
        </div>
      )}

      <Sheet
        title="New comparison"
        isOpen={isSheetOpen}
        onClose={() => setIsSheetOpen(false)}
      >
        <div className="space-y-3">
          <TextField
            label="Name"
            value={newEntryName}
            onChange={setNewEntryName}
            placeholder="e.g. Gift wrapping — clear boxes"
          />
          {createEntry.isError && <ErrorNotice error={createEntry.error} />}
          <Button
            className="w-full"
            disabled={!newEntryName.trim() || createEntry.isPending}
            onClick={() =>
              createEntry.mutate(newEntryName.trim(), {
                onSuccess: () => {
                  setNewEntryName("");
                  setIsSheetOpen(false);
                },
              })
            }
          >
            Create
          </Button>
        </div>
      </Sheet>
    </div>
  );
}

/** The PRD's comparison table, phone-first (D9): columns are products, rows are
 *  fields, and the field-label column stays frozen while the products swipe
 *  sideways. On a wide screen the columns simply fit. */
export function CompareEntryPage() {
  const { entryID = "" } = useParams<{ entryID: string }>();
  const entryQuery = useCompareEntry(entryID);
  const renameEntry = useRenameCompareEntry();
  const deleteMember = useDeleteCompareMember();
  const [isRenaming, setIsRenaming] = useState(false);
  const [draftName, setDraftName] = useState("");
  const [showPastOrders, setShowPastOrders] = useState(false);

  if (entryQuery.isPending) return <Spinner label="Loading comparison" />;
  if (entryQuery.isError) return <ErrorNotice error={entryQuery.error} />;

  const { compare_entry: entry, members } = entryQuery.data;

  return (
    <div>
      <Link
        to="/compare"
        className="mb-3 inline-flex items-center gap-1 text-sm text-ink-soft hover:text-ink"
      >
        ← All comparisons
      </Link>

      <PageHeading
        title={entry.entry_name}
        subtitle={`${members.length} ${members.length === 1 ? "product" : "products"}`}
        action={
          <Button
            variant="ghost"
            onClick={() => {
              setDraftName(entry.entry_name);
              setIsRenaming(true);
            }}
          >
            Rename
          </Button>
        }
      />

      {members.length === 0 ? (
        <EmptyState
          title="Nothing added yet"
          hint="Open any product from a vendor's catalogue and use “Add to compare”."
          action={<Link to="/"><Button>Browse vendors</Button></Link>}
        />
      ) : (
        <>
          <p className="mb-2 text-xs text-ink-faint sm:hidden">
            Swipe sideways to see every product →
          </p>

          <div className="swipe-x rounded-2xl border border-wick-100 bg-surface shadow-sm">
            <table className="w-full border-collapse text-sm">
              <tbody>
                <CompareRow label="" isHeader>
                  {members.map((member) => (
                    <ProductHeaderCell
                      key={member.compare_entry_member_id}
                      member={member}
                      onRemove={() =>
                        deleteMember.mutate({
                          entryID,
                          memberID: member.compare_entry_member_id,
                        })
                      }
                    />
                  ))}
                </CompareRow>

                <CompareRow label="Price">
                  {members.map((member) => (
                    <ValueCell key={member.compare_entry_member_id}>
                      <span className="text-base font-semibold text-ink">
                        {formatRupees(member.current_price)}
                      </span>
                    </ValueCell>
                  ))}
                </CompareRow>

                <CompareRow label="Per unit">
                  {members.map((member) => (
                    <ValueCell key={member.compare_entry_member_id}>
                      {/* Only a pack has a per-unit price distinct from its
                          price. Everything else is sold singly, so the two are
                          the same number and saying so beats an empty dash. */}
                      {member.price_per_unit ? (
                        <>
                          {formatRupees(member.price_per_unit)}
                          <span className="block text-xs text-ink-faint">
                            pack of {member.pack_size}
                          </span>
                        </>
                      ) : member.current_price ? (
                        <>
                          {formatRupees(member.current_price)}
                          <span className="block text-xs text-ink-faint">
                            sold singly
                          </span>
                        </>
                      ) : (
                        <span className="text-ink-faint">—</span>
                      )}
                    </ValueCell>
                  ))}
                </CompareRow>

                <CompareRow label="Stock">
                  {members.map((member) => (
                    <ValueCell key={member.compare_entry_member_id}>
                      {member.is_in_stock ? (
                        <span className="text-fall">In stock</span>
                      ) : (
                        <StockBadge isInStock={false} />
                      )}
                    </ValueCell>
                  ))}
                </CompareRow>

                <CompareRow label="Your rating">
                  {members.map((member) => (
                    <ValueCell key={member.compare_entry_member_id}>
                      <RatingPill
                        rating={toNumber(member.average_rating)}
                        count={member.rating_count}
                      />
                    </ValueCell>
                  ))}
                </CompareRow>

                <CompareRow label="Quantity pricing">
                  {members.map((member) => (
                    <ValueCell key={member.compare_entry_member_id}>
                      {member.moq_tiers.length > 0 ? (
                        <ul className="space-y-0.5 text-xs">
                          {member.moq_tiers.map((tier) => (
                            <li key={tier.quantity_range_minimum}>
                              <span className="text-ink-faint">
                                {tier.quantity_range_maximum === null
                                  ? `${tier.quantity_range_minimum}+`
                                  : `${tier.quantity_range_minimum}–${tier.quantity_range_maximum}`}
                              </span>{" "}
                              <span className="font-medium text-ink">
                                {formatRupees(tier.price_per_unit)}
                              </span>
                            </li>
                          ))}
                        </ul>
                      ) : (
                        <Link
                          to={`/listings/${member.vendor_listing_id}`}
                          className="text-xs text-wick-700 underline underline-offset-2"
                        >
                          Track this product to collect its quantity discounts
                        </Link>
                      )}
                    </ValueCell>
                  ))}
                </CompareRow>

                <CompareRow label="Price trend">
                  {members.map((member) => (
                    <ValueCell key={member.compare_entry_member_id}>
                      <Sparkline history={member.price_history} />
                    </ValueCell>
                  ))}
                </CompareRow>

                {/* FR-P2-4: collapsed by default so the table stays readable. */}
                <tr>
                  <td
                    colSpan={members.length + 1}
                    className="border-t border-wick-100 bg-surface-sunk/60 px-3 py-2"
                  >
                    <button
                      onClick={() => setShowPastOrders((previous) => !previous)}
                      className="text-xs font-medium text-wick-700"
                    >
                      {showPastOrders ? "▾" : "▸"} Past orders
                    </button>
                  </td>
                </tr>

                {showPastOrders && (
                  <CompareRow label="Past orders">
                    {members.map((member) => (
                      <ValueCell key={member.compare_entry_member_id}>
                        {member.past_orders.length > 0 ? (
                          <ul className="space-y-1 text-xs">
                            {member.past_orders.map((order) => (
                              <li key={order.order_item_id}>
                                <span className="text-ink-faint">
                                  {formatDate(order.ordered_on)}
                                </span>{" "}
                                <span className="font-medium text-ink">
                                  {formatRupees(order.price_per_unit)}
                                </span>
                                <span className="text-ink-faint">
                                  {" "}
                                  ×{order.quantity}
                                </span>
                              </li>
                            ))}
                          </ul>
                        ) : (
                          <span className="text-xs text-ink-faint">
                            Never ordered
                          </span>
                        )}
                      </ValueCell>
                    ))}
                  </CompareRow>
                )}
              </tbody>
            </table>
          </div>
        </>
      )}

      <Sheet
        title="Rename comparison"
        isOpen={isRenaming}
        onClose={() => setIsRenaming(false)}
      >
        <div className="space-y-3">
          <TextField label="Name" value={draftName} onChange={setDraftName} />
          <Button
            className="w-full"
            disabled={!draftName.trim() || renameEntry.isPending}
            onClick={() =>
              renameEntry.mutate(
                { entryID, entryName: draftName.trim() },
                { onSuccess: () => setIsRenaming(false) },
              )
            }
          >
            Save
          </Button>
        </div>
      </Sheet>
    </div>
  );
}

/** The label cell is sticky, so it stays put while the products swipe past. */
function CompareRow({
  label,
  children,
  isHeader = false,
}: {
  label: string;
  children: React.ReactNode;
  isHeader?: boolean;
}) {
  return (
    <tr className="border-t border-wick-100 first:border-t-0">
      <th
        scope="row"
        className={`sticky left-0 z-10 w-28 min-w-28 border-r border-wick-100 bg-surface px-3 text-left align-top text-xs font-medium tracking-wide text-ink-soft uppercase ${
          isHeader ? "py-3" : "py-3"
        }`}
      >
        {label}
      </th>
      {children}
    </tr>
  );
}

function ValueCell({ children }: { children: React.ReactNode }) {
  return (
    <td className="w-44 min-w-44 border-r border-wick-100 px-3 py-3 align-top last:border-r-0">
      {children}
    </td>
  );
}

function ProductHeaderCell({
  member,
  onRemove,
}: {
  member: CompareMember;
  onRemove: () => void;
}) {
  return (
    <td className="w-44 min-w-44 border-r border-wick-100 p-3 align-top last:border-r-0">
      <div className="relative">
        <button
          onClick={onRemove}
          aria-label="Remove from comparison"
          className="absolute -top-1 -right-1 z-10 grid size-6 place-items-center rounded-full bg-surface/90 text-xs text-ink-faint shadow hover:text-rise"
        >
          ✕
        </button>
        <Link to={`/listings/${member.vendor_listing_id}`}>
          {member.primary_image_url ? (
            <img
              src={member.primary_image_url}
              alt=""
              loading="lazy"
              className={`mb-2 aspect-square w-full rounded-lg bg-surface-sunk object-cover ${
                member.is_in_stock ? "" : "opacity-45 grayscale"
              }`}
            />
          ) : (
            <div className="mb-2 grid aspect-square w-full place-items-center rounded-lg bg-surface-sunk text-xl">
              🕯️
            </div>
          )}
          <p className="text-[11px] font-medium tracking-wide text-wick-700 uppercase">
            {member.vendor_name}
          </p>
          <p className="line-clamp-3 text-xs leading-snug font-medium text-ink">
            {decodeEntities(member.listing_name)}
          </p>
          {member.variant_label && (
            <p className="mt-0.5 text-[11px] text-ink-soft">
              {member.variant_label}
            </p>
          )}
        </Link>
      </div>
    </td>
  );
}

/** Reads live from price_history, not a snapshot taken when the product was
 *  added — resolving the open question in the product PRD. */
function Sparkline({ history }: { history: CompareMember["price_history"] }) {
  if (history.length < 2) {
    // History accrues one row a day, and only for tracked products, so this is
    // the normal state for a day or two rather than a failure.
    return (
      <span className="text-xs text-ink-faint">
        {history.length === 0 ? "Track to record prices" : "Needs another day"}
      </span>
    );
  }
  const points = history.map((point) => ({ price: toNumber(point.price) }));
  const first = points[0].price;
  const last = points[points.length - 1].price;
  const strokeColor =
    last > first ? "#c0392b" : last < first ? "#17795e" : "#c97a35";

  return (
    <div className="h-10 w-full">
      <ResponsiveContainer width="100%" height="100%">
        <LineChart data={points}>
          <Line
            type="monotone"
            dataKey="price"
            stroke={strokeColor}
            strokeWidth={1.75}
            dot={false}
            isAnimationActive={false}
          />
        </LineChart>
      </ResponsiveContainer>
    </div>
  );
}
