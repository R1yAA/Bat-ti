import { useEffect, useMemo, useState } from "react";
import { Link, useParams } from "react-router-dom";
import {
  useCategories,
  useCreateOrderEntry,
  useCreateOrderItem,
  useDeleteOrderEntry,
  useDeleteOrderItem,
  useOccasionTags,
  useOrderEntries,
  useOrderEntry,
  useUpdateOrderEntry,
  useUpdateOrderItem,
  useVendorListings,
  useVendors,
  type OrderItemInput,
} from "../api/queries";
import type { OrderItem, OrderStatus, UUID } from "../api/types";
import { PageHeading } from "../components/AppShell";
import { useConfirm } from "../components/ConfirmDialog";
import {
  Button,
  Card,
  EmptyState,
  ErrorNotice,
  SelectField,
  Sheet,
  Spinner,
  TextField,
} from "../components/ui";
import {
  decodeEntities,
  formatDate,
  formatOrderStatus,
  formatRupees,
  today,
  toNumber,
} from "../lib/format";

const orderStatuses: OrderStatus[] = [
  "placed",
  "cancelled",
  "refunded",
  "partially_refunded",
];

/** BR-10: a refund amount only makes sense for the two refunded states. */
function statusTakesRefund(status: string): boolean {
  return status === "refunded" || status === "partially_refunded";
}

const statusStyles: Record<string, string> = {
  placed: "bg-fall/10 text-fall",
  cancelled: "bg-ink-faint/15 text-ink-soft",
  refunded: "bg-rise/10 text-rise",
  partially_refunded: "bg-wick-200 text-wick-800",
};

function StatusChip({ status }: { status: string }) {
  return (
    <span
      className={`rounded-full px-2 py-0.5 text-[11px] font-medium ${
        statusStyles[status] ?? "bg-ink-faint/15 text-ink-soft"
      }`}
    >
      {formatOrderStatus(status)}
    </span>
  );
}

export function OrdersListPage() {
  const entriesQuery = useOrderEntries();
  const createEntry = useCreateOrderEntry();
  const deleteEntry = useDeleteOrderEntry();
  const confirm = useConfirm();
  const [isSheetOpen, setIsSheetOpen] = useState(false);
  const [entryName, setEntryName] = useState("");
  const [orderedOn, setOrderedOn] = useState(today());

  if (entriesQuery.isPending) return <Spinner label="Loading orders" />;
  if (entriesQuery.isError) return <ErrorNotice error={entriesQuery.error} />;

  const entries = entriesQuery.data ?? [];

  return (
    <div>
      <PageHeading
        title="Orders"
        subtitle="What you actually bought, and what it cost"
        action={<Button onClick={() => setIsSheetOpen(true)}>+ New</Button>}
      />

      {entries.length === 0 ? (
        <EmptyState
          title="No orders logged yet"
          hint="An order is one shopping session. It can mix items from several vendors."
          action={
            <Button onClick={() => setIsSheetOpen(true)}>Log an order</Button>
          }
        />
      ) : (
        <div className="space-y-2">
          {entries.map((entry) => (
            <div
              key={entry.order_entry_id}
              className="flex items-center gap-3 rounded-2xl border border-wick-100 bg-surface p-4 shadow-sm"
            >
              <Link
                to={`/orders/${entry.order_entry_id}`}
                className="min-w-0 flex-1"
              >
                <p className="truncate font-medium text-ink">
                  {entry.entry_name ?? "Untitled order"}
                </p>
                <p className="text-xs text-ink-faint">
                  {formatDate(entry.ordered_on)} · {entry.item_count}{" "}
                  {entry.item_count === 1 ? "item" : "items"}
                </p>
              </Link>
              <div className="shrink-0 text-right">
                <p className="font-semibold text-ink">
                  {formatRupees(entry.total_cost)}
                </p>
              </div>
              <button
                onClick={async () => {
                  const isConfirmed = await confirm({
                    title: "Delete this order?",
                    message: `"${entry.entry_name ?? "Untitled order"}" and all ${entry.item_count} of its items will be removed.`,
                  });
                  if (isConfirmed) deleteEntry.mutate(entry.order_entry_id);
                }}
                aria-label="Delete order"
                className="grid size-10 shrink-0 place-items-center rounded-xl text-ink-faint hover:bg-rise/10 hover:text-rise"
              >
                🗑
              </button>
            </div>
          ))}
        </div>
      )}

      <Sheet
        title="New order"
        isOpen={isSheetOpen}
        onClose={() => setIsSheetOpen(false)}
      >
        <div className="space-y-3">
          <TextField
            label="Name"
            value={entryName}
            onChange={setEntryName}
            placeholder="e.g. Diwali restock"
          />
          {/* D5: the real order date, not the row-creation date — P4 groups on
              this, so a late-logged order still lands in the right month. */}
          <TextField
            label="Ordered on"
            type="date"
            value={orderedOn}
            onChange={setOrderedOn}
          />
          {createEntry.isError && <ErrorNotice error={createEntry.error} />}
          <Button
            className="w-full"
            disabled={createEntry.isPending}
            onClick={() =>
              createEntry.mutate(
                { entry_name: entryName.trim(), ordered_on: orderedOn },
                {
                  onSuccess: () => {
                    setEntryName("");
                    setOrderedOn(today());
                    setIsSheetOpen(false);
                  },
                },
              )
            }
          >
            Create
          </Button>
        </div>
      </Sheet>
    </div>
  );
}

export function OrderDetailPage() {
  const { orderEntryID = "" } = useParams<{ orderEntryID: string }>();
  const entryQuery = useOrderEntry(orderEntryID);
  const updateEntry = useUpdateOrderEntry();
  const deleteItem = useDeleteOrderItem();
  const confirm = useConfirm();

  const [isEditingEntry, setIsEditingEntry] = useState(false);
  const [draftName, setDraftName] = useState("");
  const [draftDate, setDraftDate] = useState(today());
  const [itemBeingEdited, setItemBeingEdited] = useState<OrderItem | null>(null);
  const [isAddingItem, setIsAddingItem] = useState(false);

  if (entryQuery.isPending) return <Spinner label="Loading order" />;
  if (entryQuery.isError) return <ErrorNotice error={entryQuery.error} />;

  const entry = entryQuery.data;
  const items = entry.items ?? [];

  return (
    <div>
      <Link
        to="/orders"
        className="mb-3 inline-flex items-center gap-1 text-sm text-ink-soft hover:text-ink"
      >
        ← All orders
      </Link>

      <PageHeading
        title={entry.entry_name ?? "Untitled order"}
        subtitle={formatDate(entry.ordered_on)}
        action={
          <Button
            variant="ghost"
            onClick={() => {
              setDraftName(entry.entry_name ?? "");
              setDraftDate(entry.ordered_on);
              setIsEditingEntry(true);
            }}
          >
            Edit
          </Button>
        }
      />

      <Card className="mb-4 flex items-baseline justify-between">
        <span className="text-sm font-medium tracking-wide text-ink-soft uppercase">
          Total
        </span>
        {/* FR-P3-3: auto-summed from quantity × price across every item. */}
        <span className="text-2xl font-semibold text-ink">
          {formatRupees(entry.total_cost)}
        </span>
      </Card>

      <div className="mb-3 flex items-center justify-between">
        <h2 className="text-sm font-semibold tracking-wide text-ink-soft uppercase">
          Items
        </h2>
        <Button onClick={() => setIsAddingItem(true)}>+ Add item</Button>
      </div>

      {items.length === 0 ? (
        <EmptyState
          title="No items yet"
          hint="Add what you bought — each item carries its own vendor, quantity and price."
        />
      ) : (
        <div className="space-y-2">
          {items.map((item) => (
            <Card key={item.order_item_id} className="flex gap-3">
              {item.primary_image_url ? (
                <img
                  src={item.primary_image_url}
                  alt=""
                  loading="lazy"
                  className="size-16 shrink-0 rounded-lg bg-surface-sunk object-cover"
                />
              ) : (
                <div className="grid size-16 shrink-0 place-items-center rounded-lg bg-surface-sunk text-xl">
                  🕯️
                </div>
              )}

              <div className="min-w-0 flex-1">
                <p className="text-[11px] font-medium tracking-wide text-wick-700 uppercase">
                  {item.vendor_name}
                </p>
                {item.vendor_listing_id ? (
                  <Link
                    to={`/listings/${item.vendor_listing_id}`}
                    className="line-clamp-2 text-sm font-medium text-ink hover:text-wick-700"
                  >
                    {decodeEntities(item.listing_name)}
                  </Link>
                ) : (
                  <p className="line-clamp-2 text-sm font-medium text-ink">
                    {decodeEntities(item.listing_name)}
                  </p>
                )}

                <p className="mt-0.5 text-xs text-ink-soft">
                  {item.quantity} × {formatRupees(item.price_per_unit)} ={" "}
                  <span className="font-medium text-ink">
                    {formatRupees(item.line_total)}
                  </span>
                </p>

                {item.refund_amount && (
                  <p className="text-xs text-rise">
                    Refunded {formatRupees(item.refund_amount)}
                  </p>
                )}

                <div className="mt-1.5 flex flex-wrap items-center gap-1.5">
                  <StatusChip status={item.order_status} />
                  {item.rating !== null && (
                    <span className="rounded-full bg-wick-100 px-2 py-0.5 text-[11px] font-medium text-wick-800">
                      ★ {item.rating}/10
                    </span>
                  )}
                  {item.category_tags.map((category) => (
                    <span
                      key={category.category_id}
                      className="rounded-full bg-surface-sunk px-2 py-0.5 text-[11px] text-ink-soft"
                    >
                      {category.category_name}
                    </span>
                  ))}
                  {item.occasion_tags.map((tag) => (
                    <span
                      key={tag.occasion_tag_id}
                      className="rounded-full bg-surface-sunk px-2 py-0.5 text-[11px] text-ink-soft"
                    >
                      #{tag.tag_name}
                    </span>
                  ))}
                </div>
              </div>

              <div className="flex shrink-0 flex-col gap-1">
                <button
                  onClick={() => setItemBeingEdited(item)}
                  aria-label="Edit item"
                  className="grid size-9 place-items-center rounded-lg text-ink-faint hover:bg-wick-50 hover:text-ink"
                >
                  ✎
                </button>
                <button
                  onClick={async () => {
                    const isConfirmed = await confirm({
                      title: "Remove this item?",
                      message: decodeEntities(item.listing_name),
                      confirmLabel: "Remove",
                    });
                    if (isConfirmed) {
                      deleteItem.mutate({
                        orderItemID: item.order_item_id,
                        orderEntryID,
                      });
                    }
                  }}
                  aria-label="Delete item"
                  className="grid size-9 place-items-center rounded-lg text-ink-faint hover:bg-rise/10 hover:text-rise"
                >
                  🗑
                </button>
              </div>
            </Card>
          ))}
        </div>
      )}

      <Sheet
        title="Edit order"
        isOpen={isEditingEntry}
        onClose={() => setIsEditingEntry(false)}
      >
        <div className="space-y-3">
          <TextField label="Name" value={draftName} onChange={setDraftName} />
          <TextField
            label="Ordered on"
            type="date"
            value={draftDate}
            onChange={setDraftDate}
          />
          {updateEntry.isError && <ErrorNotice error={updateEntry.error} />}
          <Button
            className="w-full"
            disabled={updateEntry.isPending}
            onClick={() =>
              updateEntry.mutate(
                {
                  orderEntryID,
                  input: {
                    entry_name: draftName.trim(),
                    ordered_on: draftDate,
                  },
                },
                { onSuccess: () => setIsEditingEntry(false) },
              )
            }
          >
            Save
          </Button>
        </div>
      </Sheet>

      <OrderItemSheet
        orderEntryID={orderEntryID}
        item={itemBeingEdited}
        isOpen={isAddingItem || itemBeingEdited !== null}
        onClose={() => {
          setIsAddingItem(false);
          setItemBeingEdited(null);
        }}
      />
    </div>
  );
}

/** One sheet serves both adding and editing — the fields are identical, and
 *  keeping them together means a change to the rules lands in one place. */
function OrderItemSheet({
  orderEntryID,
  item,
  isOpen,
  onClose,
}: {
  orderEntryID: UUID;
  item: OrderItem | null;
  isOpen: boolean;
  onClose: () => void;
}) {
  const createItem = useCreateOrderItem();
  const updateItem = useUpdateOrderItem();
  const categoriesQuery = useCategories();
  const occasionTagsQuery = useOccasionTags();

  const [selectedListing, setSelectedListing] = useState<{
    id: UUID;
    name: string;
  } | null>(null);
  const [quantity, setQuantity] = useState("1");
  const [pricePerUnit, setPricePerUnit] = useState("");
  const [orderStatus, setOrderStatus] = useState<string>("placed");
  const [refundAmount, setRefundAmount] = useState("");
  const [rating, setRating] = useState("");
  const [categoryIDs, setCategoryIDs] = useState<UUID[]>([]);
  const [occasionTagIDs, setOccasionTagIDs] = useState<UUID[]>([]);
  const [isPickerOpen, setIsPickerOpen] = useState(false);

  // Reload the form whenever it is opened for a different item.
  useEffect(() => {
    if (!isOpen) return;
    if (item) {
      setSelectedListing(
        item.vendor_listing_id
          ? { id: item.vendor_listing_id, name: item.listing_name }
          : null,
      );
      setQuantity(String(item.quantity));
      setPricePerUnit(item.price_per_unit);
      setOrderStatus(item.order_status);
      setRefundAmount(item.refund_amount ?? "");
      setRating(item.rating === null ? "" : String(item.rating));
      setCategoryIDs(item.category_tags.map((tag) => tag.category_id));
      setOccasionTagIDs(item.occasion_tags.map((tag) => tag.occasion_tag_id));
    } else {
      setSelectedListing(null);
      setQuantity("1");
      setPricePerUnit("");
      setOrderStatus("placed");
      setRefundAmount("");
      setRating("");
      setCategoryIDs([]);
      setOccasionTagIDs([]);
    }
  }, [isOpen, item]);

  const mutation = item ? updateItem : createItem;

  const submit = () => {
    const input: OrderItemInput = {
      vendor_listing_id: selectedListing?.id ?? null,
      quantity: Number(quantity),
      price_per_unit: pricePerUnit.trim(),
      order_status: orderStatus,
      refund_amount: statusTakesRefund(orderStatus)
        ? refundAmount.trim() || "0"
        : null,
      rating: rating.trim() === "" ? null : Number(rating),
      category_ids: categoryIDs,
      occasion_tag_ids: occasionTagIDs,
    };

    if (item) {
      updateItem.mutate(
        { orderItemID: item.order_item_id, orderEntryID, input },
        { onSuccess: onClose },
      );
    } else {
      createItem.mutate({ orderEntryID, input }, { onSuccess: onClose });
    }
  };

  const canSubmit =
    selectedListing !== null &&
    Number(quantity) > 0 &&
    pricePerUnit.trim() !== "";

  return (
    <>
      <Sheet
        title={item ? "Edit item" : "Add item"}
        isOpen={isOpen && !isPickerOpen}
        onClose={onClose}
      >
        <div className="space-y-3">
          <div>
            <span className="mb-1 block text-xs font-medium tracking-wide text-ink-soft uppercase">
              Product
            </span>
            <button
              onClick={() => setIsPickerOpen(true)}
              className="w-full rounded-xl border border-wick-200 bg-surface px-3 py-2.5 text-left text-sm hover:bg-wick-50"
            >
              {selectedListing ? (
                <span className="line-clamp-2 text-ink">
                  {decodeEntities(selectedListing.name)}
                </span>
              ) : (
                <span className="text-ink-faint">Choose a product…</span>
              )}
            </button>
          </div>

          <div className="grid grid-cols-2 gap-3">
            <TextField
              label="Quantity"
              value={quantity}
              onChange={setQuantity}
              inputMode="numeric"
            />
            {/* FR-P3-2 stores the per-unit price, never the line total, so
                cross-vendor comparisons stay possible later. */}
            <TextField
              label="Price per unit"
              value={pricePerUnit}
              onChange={setPricePerUnit}
              inputMode="decimal"
              placeholder="39.00"
            />
          </div>

          {pricePerUnit.trim() !== "" && Number(quantity) > 0 && (
            <p className="-mt-1 text-xs text-ink-soft">
              Line total{" "}
              <span className="font-medium text-ink">
                {formatRupees(
                  String(toNumber(pricePerUnit) * Number(quantity)),
                )}
              </span>
            </p>
          )}

          <SelectField
            label="Status"
            value={orderStatus}
            onChange={setOrderStatus}
          >
            {orderStatuses.map((status) => (
              <option key={status} value={status}>
                {formatOrderStatus(status)}
              </option>
            ))}
          </SelectField>

          {statusTakesRefund(orderStatus) && (
            <TextField
              label="Refund received"
              value={refundAmount}
              onChange={setRefundAmount}
              inputMode="decimal"
              placeholder="0.00"
            />
          )}

          {/* D8: any item is ratable whatever its status. */}
          <TextField
            label="Your rating (1–10, optional)"
            value={rating}
            onChange={setRating}
            inputMode="numeric"
            placeholder="8"
          />

          <TagPicker
            label="Category"
            options={(categoriesQuery.data ?? []).map((category) => ({
              id: category.category_id,
              name: category.category_name,
            }))}
            selectedIDs={categoryIDs}
            onChange={setCategoryIDs}
          />

          <TagPicker
            label="Occasion"
            options={(occasionTagsQuery.data ?? []).map((tag) => ({
              id: tag.occasion_tag_id,
              name: tag.tag_name,
            }))}
            selectedIDs={occasionTagIDs}
            onChange={setOccasionTagIDs}
          />

          {mutation.isError && <ErrorNotice error={mutation.error} />}

          <Button
            className="w-full"
            disabled={!canSubmit || mutation.isPending}
            onClick={submit}
          >
            {item ? "Save changes" : "Add to order"}
          </Button>
        </div>
      </Sheet>

      <ListingPickerSheet
        isOpen={isPickerOpen}
        onClose={() => setIsPickerOpen(false)}
        onPick={(picked) => {
          setSelectedListing(picked);
          // Prefill from the catalogue, but leave it editable — what was paid
          // is not always what the vendor lists today.
          if (picked.price && pricePerUnit.trim() === "") {
            setPricePerUnit(picked.price);
          }
          setIsPickerOpen(false);
        }}
      />
    </>
  );
}

function TagPicker({
  label,
  options,
  selectedIDs,
  onChange,
}: {
  label: string;
  options: { id: UUID; name: string }[];
  selectedIDs: UUID[];
  onChange: (ids: UUID[]) => void;
}) {
  if (options.length === 0) {
    return (
      <div>
        <span className="mb-1 block text-xs font-medium tracking-wide text-ink-soft uppercase">
          {label}
        </span>
        <p className="text-xs text-ink-faint">
          None yet — add some in Settings.
        </p>
      </div>
    );
  }

  return (
    <div>
      <span className="mb-1.5 block text-xs font-medium tracking-wide text-ink-soft uppercase">
        {label}
      </span>
      <div className="flex flex-wrap gap-1.5">
        {options.map((option) => {
          const isSelected = selectedIDs.includes(option.id);
          return (
            <button
              key={option.id}
              onClick={() =>
                onChange(
                  isSelected
                    ? selectedIDs.filter((id) => id !== option.id)
                    : [...selectedIDs, option.id],
                )
              }
              className={`rounded-full border px-3 py-1.5 text-sm transition ${
                isSelected
                  ? "border-wick-500 bg-wick-100 font-medium text-wick-800"
                  : "border-wick-200 bg-surface text-ink-soft hover:bg-wick-50"
              }`}
            >
              {option.name}
            </button>
          );
        })}
      </div>
    </div>
  );
}

/** Picking the product an order item refers to. Scoped by vendor first, because
 *  3000-odd listings across ten vendors is not a single searchable list on a
 *  phone. */
function ListingPickerSheet({
  isOpen,
  onClose,
  onPick,
}: {
  isOpen: boolean;
  onClose: () => void;
  onPick: (picked: { id: UUID; name: string; price: string | null }) => void;
}) {
  const vendorsQuery = useVendors();
  const [vendorSlug, setVendorSlug] = useState("");
  const [searchText, setSearchText] = useState("");
  const [debouncedSearch, setDebouncedSearch] = useState("");

  useEffect(() => {
    const timer = setTimeout(() => setDebouncedSearch(searchText), 300);
    return () => clearTimeout(timer);
  }, [searchText]);

  const firstVendorSlug = vendorsQuery.data?.[0]?.vendor_slug ?? "";
  const activeSlug = vendorSlug || firstVendorSlug;

  const filters = useMemo(
    () => ({
      limit: 25,
      offset: 0,
      search: debouncedSearch,
      inStockOnly: false,
      includeDelisted: true,
    }),
    [debouncedSearch],
  );

  const listingsQuery = useVendorListings(isOpen ? activeSlug : "", filters);

  return (
    <Sheet title="Choose a product" isOpen={isOpen} onClose={onClose}>
      <div className="space-y-3">
        <SelectField label="Vendor" value={activeSlug} onChange={setVendorSlug}>
          {vendorsQuery.data?.map((vendor) => (
            <option key={vendor.vendor_id} value={vendor.vendor_slug}>
              {vendor.vendor_name}
            </option>
          ))}
        </SelectField>

        <TextField
          label="Search"
          value={searchText}
          onChange={setSearchText}
          placeholder="Product name"
        />

        {listingsQuery.isPending && <Spinner label="Searching" />}

        <div className="max-h-80 space-y-1.5 overflow-y-auto">
          {listingsQuery.data?.listings.map((listing) => (
            <button
              key={listing.vendor_listing_id}
              onClick={() =>
                onPick({
                  id: listing.vendor_listing_id,
                  name: listing.listing_name,
                  price: listing.current_price,
                })
              }
              className="flex w-full items-center gap-3 rounded-xl border border-wick-100 bg-surface p-2 text-left transition hover:bg-wick-50"
            >
              {listing.primary_image_url ? (
                <img
                  src={listing.primary_image_url}
                  alt=""
                  loading="lazy"
                  className="size-11 shrink-0 rounded-lg bg-surface-sunk object-cover"
                />
              ) : (
                <div className="grid size-11 shrink-0 place-items-center rounded-lg bg-surface-sunk">
                  🕯️
                </div>
              )}
              <span className="min-w-0 flex-1">
                <span className="line-clamp-2 block text-sm text-ink">
                  {decodeEntities(listing.listing_name)}
                </span>
                <span className="text-xs text-ink-faint">
                  {formatRupees(listing.current_price)}
                </span>
              </span>
            </button>
          ))}
          {listingsQuery.data?.listings.length === 0 && (
            <p className="py-4 text-center text-sm text-ink-soft">
              Nothing matches that search.
            </p>
          )}
        </div>
      </div>
    </Sheet>
  );
}
