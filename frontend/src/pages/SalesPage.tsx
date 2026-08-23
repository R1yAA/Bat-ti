import { useEffect, useState } from "react";
import { Link, useParams } from "react-router-dom";
import {
  useCreateSaleOrderEntry,
  useCreateSaleOrderItem,
  useDeleteSaleOrderEntry,
  useDeleteSaleOrderItem,
  useSaleOrderCategories,
  useSaleOrderEntries,
  useSaleOrderEntry,
  useUpdateSaleOrderEntry,
  useUpdateSaleOrderItem,
  type SaleOrderEntryInput,
  type SaleOrderItemInput,
} from "../api/queries";
import type { SaleOrderItem, SaleOrderStatus, UUID } from "../api/types";
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
  formatDate,
  formatRupees,
  formatSaleOrderStatus,
  today,
  toNumber,
} from "../lib/format";

// P6 — the sell side. Its buy-side mirror is OrdersPage.tsx, and the two are
// deliberately separate screens: a purchase from a vendor and a sale to a
// customer answer different questions, and one combined list would make
// neither readable.

// BR-20, in workflow order. Cancelled sits last because it is the exit, not a
// stage.
const saleOrderStatuses: SaleOrderStatus[] = [
  "pending",
  "confirmed",
  "shipped",
  "delivered",
  "cancelled",
];

const statusStyles: Record<string, string> = {
  pending: "bg-wick-200 text-wick-800",
  confirmed: "bg-wick-100 text-wick-800",
  shipped: "bg-fall/10 text-fall",
  delivered: "bg-fall/15 text-fall",
  cancelled: "bg-ink-faint/15 text-ink-soft",
};

function StatusChip({ status }: { status: string }) {
  return (
    <span
      className={`rounded-full px-2 py-0.5 text-[11px] font-medium ${
        statusStyles[status] ?? "bg-ink-faint/15 text-ink-soft"
      }`}
    >
      {formatSaleOrderStatus(status)}
    </span>
  );
}

/** BR-21: the number the customer is quoted, shown as a number rather than
 *  buried, because it is how an order is looked up over the phone. */
function OrderNumber({ saleOrderID }: { saleOrderID: number }) {
  return (
    <span className="rounded-md bg-surface-sunk px-1.5 py-0.5 font-mono text-[11px] text-ink-soft">
      #{saleOrderID}
    </span>
  );
}

export function SalesListPage() {
  // FR-P6-6. The API takes any status; the UI offers the one the requirement
  // asks for, because "pending" is the list that needs chasing.
  const [showPendingOnly, setShowPendingOnly] = useState(false);
  const entriesQuery = useSaleOrderEntries(showPendingOnly ? "pending" : "");
  const deleteEntry = useDeleteSaleOrderEntry();
  const confirm = useConfirm();
  const [isSheetOpen, setIsSheetOpen] = useState(false);

  const entries = entriesQuery.data ?? [];

  return (
    <div>
      <PageHeading
        title="Sales Orders"
        subtitle="What you sold, to whom, and where it got to"
        action={<Button onClick={() => setIsSheetOpen(true)}>+ New</Button>}
      />

      <div className="mb-3 flex gap-2">
        <button
          onClick={() => setShowPendingOnly(!showPendingOnly)}
          aria-pressed={showPendingOnly}
          className={`rounded-full px-3.5 py-2 text-sm font-medium transition ${
            showPendingOnly
              ? "bg-wick-600 text-white"
              : "border border-wick-200 bg-surface text-ink-soft hover:bg-wick-50"
          }`}
        >
          Pending only
        </button>
      </div>

      {entriesQuery.isPending && <Spinner label="Loading sales" />}
      {entriesQuery.isError && <ErrorNotice error={entriesQuery.error} />}

      {entriesQuery.isSuccess && entries.length === 0 ? (
        <EmptyState
          title={showPendingOnly ? "Nothing pending" : "No sales recorded yet"}
          hint={
            showPendingOnly
              ? "Every sale has moved past Pending. Turn the filter off to see them all."
              : "A sale is one order from one customer. It can hold as many products as you sold them."
          }
          action={
            showPendingOnly ? (
              <Button variant="ghost" onClick={() => setShowPendingOnly(false)}>
                Show all sales
              </Button>
            ) : (
              <Button onClick={() => setIsSheetOpen(true)}>
                Record a sale
              </Button>
            )
          }
        />
      ) : (
        <div className="space-y-2">
          {entries.map((entry) => (
            <div
              key={entry.sale_order_entry_id}
              className="flex items-center gap-3 rounded-2xl border border-wick-100 bg-surface p-4 shadow-sm"
            >
              <Link
                to={`/sales/${entry.sale_order_entry_id}`}
                className="min-w-0 flex-1"
              >
                {/* FR-P6-5: number, customer, date, status, category and
                    total, all legible without opening the sale. */}
                <div className="flex items-center gap-1.5">
                  <OrderNumber saleOrderID={entry.sale_order_id} />
                  <p className="truncate font-medium text-ink">
                    {entry.consumer_name}
                  </p>
                </div>
                <p className="mt-0.5 text-xs text-ink-faint">
                  {formatDate(entry.order_placed_date)} · {entry.item_count}{" "}
                  {entry.item_count === 1 ? "item" : "items"} ·{" "}
                  {entry.category_name}
                </p>
                <div className="mt-1.5">
                  <StatusChip status={entry.order_status} />
                </div>
              </Link>

              <div className="shrink-0 text-right">
                <p className="font-semibold text-ink">
                  {formatRupees(entry.total_amount)}
                </p>
              </div>

              <button
                onClick={async () => {
                  const isConfirmed = await confirm({
                    title: `Delete sale #${entry.sale_order_id}?`,
                    message: `${entry.consumer_name}'s order and all ${entry.item_count} of its items will be removed.`,
                  });
                  if (isConfirmed) {
                    deleteEntry.mutate(entry.sale_order_entry_id);
                  }
                }}
                aria-label={`Delete sale ${entry.sale_order_id}`}
                className="grid size-10 shrink-0 place-items-center rounded-xl text-ink-faint hover:bg-rise/10 hover:text-rise"
              >
                🗑
              </button>
            </div>
          ))}
        </div>
      )}

      <SaleOrderSheet
        entry={null}
        isOpen={isSheetOpen}
        onClose={() => setIsSheetOpen(false)}
      />
    </div>
  );
}

export function SaleOrderDetailPage() {
  const { saleOrderEntryID = "" } = useParams<{ saleOrderEntryID: string }>();
  const entryQuery = useSaleOrderEntry(saleOrderEntryID);
  const deleteItem = useDeleteSaleOrderItem();
  const confirm = useConfirm();

  const [isEditingEntry, setIsEditingEntry] = useState(false);
  const [itemBeingEdited, setItemBeingEdited] = useState<SaleOrderItem | null>(
    null,
  );
  const [isAddingItem, setIsAddingItem] = useState(false);

  if (entryQuery.isPending) return <Spinner label="Loading sale" />;
  if (entryQuery.isError) return <ErrorNotice error={entryQuery.error} />;

  const entry = entryQuery.data;
  const items = entry.items ?? [];

  return (
    <div>
      <Link
        to="/sales"
        className="mb-3 inline-flex items-center gap-1 text-sm text-ink-soft hover:text-ink"
      >
        ← All sales
      </Link>

      <PageHeading
        title={entry.consumer_name}
        subtitle={`#${entry.sale_order_id} · placed ${formatDate(entry.order_placed_date)}`}
        action={
          <Button variant="ghost" onClick={() => setIsEditingEntry(true)}>
            Edit
          </Button>
        }
      />

      {/* FR-P6-7: status, dates, category and total in one glance. */}
      <Card className="mb-4">
        <div className="flex items-baseline justify-between">
          <span className="text-sm font-medium tracking-wide text-ink-soft uppercase">
            Total
          </span>
          {/* FR-P6-4 / BR-19: summed from the items, never typed in. */}
          <span className="text-2xl font-semibold text-ink">
            {formatRupees(entry.total_amount)}
          </span>
        </div>
        <div className="mt-3 flex flex-wrap items-center gap-2 border-t border-wick-100 pt-3">
          <StatusChip status={entry.order_status} />
          <span className="rounded-full bg-surface-sunk px-2 py-0.5 text-[11px] text-ink-soft">
            {entry.category_name}
          </span>
          {entry.delivered_date && (
            <span className="text-xs text-ink-faint">
              Delivered {formatDate(entry.delivered_date)}
            </span>
          )}
        </div>
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
          hint="Add what you sold — the total adds itself up from these."
        />
      ) : (
        <div className="space-y-2">
          {items.map((item) => (
            <Card key={item.sale_order_item_id} className="flex gap-3">
              <div className="min-w-0 flex-1">
                <p className="line-clamp-2 text-sm font-medium text-ink">
                  {item.product_name}
                </p>
                <p className="mt-0.5 text-xs text-ink-soft">
                  {item.quantity} × {formatRupees(item.price_per_unit)} ={" "}
                  <span className="font-medium text-ink">
                    {formatRupees(item.line_total)}
                  </span>
                </p>
              </div>

              {/* Side by side, not stacked as on the purchase side: there is
                  no product image here to give the row height to fill. */}
              <div className="flex shrink-0 items-start gap-1">
                <button
                  onClick={() => setItemBeingEdited(item)}
                  aria-label={`Edit ${item.product_name}`}
                  className="grid size-9 place-items-center rounded-lg text-ink-faint hover:bg-wick-50 hover:text-ink"
                >
                  ✎
                </button>
                <button
                  onClick={async () => {
                    const isConfirmed = await confirm({
                      title: "Remove this item?",
                      message: item.product_name,
                      confirmLabel: "Remove",
                    });
                    if (isConfirmed) {
                      deleteItem.mutate({
                        saleOrderItemID: item.sale_order_item_id,
                        saleOrderEntryID,
                      });
                    }
                  }}
                  aria-label={`Delete ${item.product_name}`}
                  className="grid size-9 place-items-center rounded-lg text-ink-faint hover:bg-rise/10 hover:text-rise"
                >
                  🗑
                </button>
              </div>
            </Card>
          ))}
        </div>
      )}

      <SaleOrderSheet
        entry={entry}
        isOpen={isEditingEntry}
        onClose={() => setIsEditingEntry(false)}
      />

      <SaleOrderItemSheet
        saleOrderEntryID={saleOrderEntryID}
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

/** One sheet serves both creating and editing a sale (FR-P6-1, FR-P6-8). The
 *  fields are identical either way, so keeping them together means a change to
 *  the rules lands in one place. */
function SaleOrderSheet({
  entry,
  isOpen,
  onClose,
}: {
  entry: {
    sale_order_entry_id: UUID;
    consumer_name: string;
    order_placed_date: string;
    order_status: string;
    delivered_date: string | null;
    sale_order_category_id: UUID;
  } | null;
  isOpen: boolean;
  onClose: () => void;
}) {
  const createEntry = useCreateSaleOrderEntry();
  const updateEntry = useUpdateSaleOrderEntry();
  const categoriesQuery = useSaleOrderCategories();

  const [consumerName, setConsumerName] = useState("");
  const [placedDate, setPlacedDate] = useState(today());
  const [orderStatus, setOrderStatus] = useState<string>("pending");
  const [deliveredDate, setDeliveredDate] = useState("");
  const [categoryID, setCategoryID] = useState<UUID>("");

  // Reload the form whenever it opens, so a cancelled edit leaves no residue.
  useEffect(() => {
    if (!isOpen) return;
    if (entry) {
      setConsumerName(entry.consumer_name);
      setPlacedDate(entry.order_placed_date);
      setOrderStatus(entry.order_status);
      setDeliveredDate(entry.delivered_date ?? "");
      setCategoryID(entry.sale_order_category_id);
    } else {
      setConsumerName("");
      setPlacedDate(today());
      setOrderStatus("pending");
      setDeliveredDate("");
      setCategoryID("");
    }
  }, [isOpen, entry]);

  const mutation = entry ? updateEntry : createEntry;
  const isDelivered = orderStatus === "delivered";

  const submit = () => {
    const input: SaleOrderEntryInput = {
      consumer_name: consumerName.trim(),
      order_placed_date: placedDate,
      order_status: orderStatus,
      // BR-20: the date only travels with the status it belongs to.
      delivered_date: isDelivered ? deliveredDate || null : null,
      sale_order_category_id: categoryID || null,
    };
    if (entry) {
      updateEntry.mutate(
        { saleOrderEntryID: entry.sale_order_entry_id, input },
        { onSuccess: onClose },
      );
    } else {
      createEntry.mutate(input, { onSuccess: onClose });
    }
  };

  return (
    <Sheet
      title={entry ? "Edit sale" : "New sale"}
      isOpen={isOpen}
      onClose={onClose}
    >
      <div className="space-y-3">
        <TextField
          label="Customer"
          value={consumerName}
          onChange={setConsumerName}
          placeholder="Who bought it"
        />

        <TextField
          label="Order placed on"
          type="date"
          value={placedDate}
          onChange={setPlacedDate}
        />

        <SelectField
          label="Status"
          value={orderStatus}
          onChange={setOrderStatus}
        >
          {saleOrderStatuses.map((status) => (
            <option key={status} value={status}>
              {formatSaleOrderStatus(status)}
            </option>
          ))}
        </SelectField>

        {/* BR-20: the field appears only once the order has reached Delivered,
            and moving the status back drops the date rather than stranding it
            on an order that has not arrived. */}
        {isDelivered && (
          <TextField
            label="Delivered on (optional)"
            type="date"
            value={deliveredDate}
            onChange={setDeliveredDate}
          />
        )}

        {/* BR-22: exactly one category, so a select rather than the chip
            multi-picker the purchase side uses. */}
        <SelectField
          label="Category"
          value={categoryID}
          onChange={setCategoryID}
        >
          <option value="">Uncategorized</option>
          {(categoriesQuery.data ?? [])
            .filter((category) => !category.is_system)
            .map((category) => (
              <option
                key={category.sale_order_category_id}
                value={category.sale_order_category_id}
              >
                {category.category_name}
              </option>
            ))}
        </SelectField>
        <p className="-mt-1 text-xs text-ink-faint">
          Manage this list in Settings.
        </p>

        {mutation.isError && <ErrorNotice error={mutation.error} />}

        <Button
          className="w-full"
          disabled={consumerName.trim() === "" || mutation.isPending}
          onClick={submit}
        >
          {entry ? "Save changes" : "Create sale"}
        </Button>
        {!entry && (
          <p className="text-center text-xs text-ink-faint">
            An order number is assigned automatically.
          </p>
        )}
      </div>
    </Sheet>
  );
}

/** FR-P6-3: the product is typed in, not picked. There is no catalogue of
 *  finished goods yet — only of the raw materials that go into them. */
function SaleOrderItemSheet({
  saleOrderEntryID,
  item,
  isOpen,
  onClose,
}: {
  saleOrderEntryID: UUID;
  item: SaleOrderItem | null;
  isOpen: boolean;
  onClose: () => void;
}) {
  const createItem = useCreateSaleOrderItem();
  const updateItem = useUpdateSaleOrderItem();

  const [productName, setProductName] = useState("");
  const [quantity, setQuantity] = useState("1");
  const [pricePerUnit, setPricePerUnit] = useState("");

  useEffect(() => {
    if (!isOpen) return;
    if (item) {
      setProductName(item.product_name);
      setQuantity(String(item.quantity));
      setPricePerUnit(item.price_per_unit);
    } else {
      setProductName("");
      setQuantity("1");
      setPricePerUnit("");
    }
  }, [isOpen, item]);

  const mutation = item ? updateItem : createItem;

  const submit = () => {
    const input: SaleOrderItemInput = {
      product_name: productName.trim(),
      quantity: Number(quantity),
      price_per_unit: pricePerUnit.trim(),
    };
    if (item) {
      updateItem.mutate(
        {
          saleOrderItemID: item.sale_order_item_id,
          saleOrderEntryID,
          input,
        },
        { onSuccess: onClose },
      );
    } else {
      createItem.mutate({ saleOrderEntryID, input }, { onSuccess: onClose });
    }
  };

  const canSubmit =
    productName.trim() !== "" &&
    Number(quantity) > 0 &&
    pricePerUnit.trim() !== "";

  return (
    <Sheet
      title={item ? "Edit item" : "Add item"}
      isOpen={isOpen}
      onClose={onClose}
    >
      <div className="space-y-3">
        <TextField
          label="Product"
          value={productName}
          onChange={setProductName}
          placeholder="e.g. Soy pillar candle, 6 inch"
        />

        <div className="grid grid-cols-2 gap-3">
          <TextField
            label="Quantity"
            value={quantity}
            onChange={setQuantity}
            inputMode="numeric"
          />
          <TextField
            label="Price per unit"
            value={pricePerUnit}
            onChange={setPricePerUnit}
            inputMode="decimal"
            placeholder="185.00"
          />
        </div>

        {pricePerUnit.trim() !== "" && Number(quantity) > 0 && (
          <p className="-mt-1 text-xs text-ink-soft">
            Line total{" "}
            <span className="font-medium text-ink">
              {formatRupees(String(toNumber(pricePerUnit) * Number(quantity)))}
            </span>
          </p>
        )}

        {mutation.isError && <ErrorNotice error={mutation.error} />}

        <Button
          className="w-full"
          disabled={!canSubmit || mutation.isPending}
          onClick={submit}
        >
          {item ? "Save changes" : "Add to sale"}
        </Button>
      </div>
    </Sheet>
  );
}
