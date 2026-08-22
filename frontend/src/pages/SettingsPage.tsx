import { useState } from "react";
import {
  useCategories,
  useCreateCategory,
  useCreateOccasionTag,
  useDeleteAllData,
  useDeleteCategory,
  useDeleteOccasionTag,
  useOccasionTags,
  useRenameCategory,
  useRenameOccasionTag,
  useScrapeRuns,
  useVendors,
} from "../api/queries";
import type { UUID } from "../api/types";
import { useSession } from "../auth/SessionProvider";
import { PageHeading } from "../components/AppShell";
import {
  Button,
  Card,
  ErrorNotice,
  Spinner,
  TextField,
} from "../components/ui";
import { formatRelativeTime } from "../lib/format";

/** BR-14: the exact phrase the server demands. Typed by hand, deliberately. */
const DELETE_CONFIRMATION = "DELETE ALL MY DATA";

export function SettingsPage() {
  return (
    <div className="space-y-4">
      <PageHeading title="Settings" />
      <AccountSection />
      <CategoriesSection />
      <OccasionTagsSection />
      <VendorHealthSection />
      <DangerZone />
    </div>
  );
}

function AccountSection() {
  const { session, signOut } = useSession();
  return (
    <Card>
      <h2 className="mb-1 text-sm font-semibold tracking-wide text-ink-soft uppercase">
        Account
      </h2>
      <div className="flex items-center justify-between gap-3">
        <p className="min-w-0 truncate text-sm text-ink">
          {session?.user.email ?? "Signed in"}
        </p>
        <Button variant="ghost" onClick={() => void signOut()}>
          Sign out
        </Button>
      </div>
    </Card>
  );
}

function CategoriesSection() {
  const categoriesQuery = useCategories();
  const createCategory = useCreateCategory();
  const renameCategory = useRenameCategory();
  const deleteCategory = useDeleteCategory();
  const [newName, setNewName] = useState("");

  return (
    <Card>
      <h2 className="mb-1 text-sm font-semibold tracking-wide text-ink-soft uppercase">
        Categories
      </h2>
      <p className="mb-3 text-xs text-ink-faint">
        Your own labels for order items. Deleting one moves anything using it to
        Uncategorized — nothing is lost.
      </p>

      {categoriesQuery.isPending && <Spinner label="Loading" />}
      {categoriesQuery.isError && <ErrorNotice error={categoriesQuery.error} />}

      <ul className="divide-y divide-wick-100">
        {categoriesQuery.data?.map((category) => (
          <EditableTagRow
            key={category.category_id}
            id={category.category_id}
            name={category.category_name}
            usageCount={category.usage_count}
            /* The Uncategorized row is the reassignment target for every other
               deletion, so it cannot itself be removed. */
            isProtected={category.is_system}
            onRename={(name) =>
              renameCategory.mutate({ categoryID: category.category_id, name })
            }
            onDelete={() => deleteCategory.mutate(category.category_id)}
          />
        ))}
      </ul>

      {deleteCategory.isError && <ErrorNotice error={deleteCategory.error} />}

      <div className="mt-3 flex gap-2">
        <input
          value={newName}
          onChange={(event) => setNewName(event.target.value)}
          placeholder="New category"
          className="min-w-0 flex-1 rounded-xl border border-wick-200 bg-surface px-3 py-2.5 text-base outline-none placeholder:text-ink-faint focus:border-wick-500 focus:ring-2 focus:ring-wick-500/20"
        />
        <Button
          disabled={!newName.trim() || createCategory.isPending}
          onClick={() =>
            createCategory.mutate(newName.trim(), {
              onSuccess: () => setNewName(""),
            })
          }
        >
          Add
        </Button>
      </div>
    </Card>
  );
}

function OccasionTagsSection() {
  const tagsQuery = useOccasionTags();
  const createTag = useCreateOccasionTag();
  const renameTag = useRenameOccasionTag();
  const deleteTag = useDeleteOccasionTag();
  const [newName, setNewName] = useState("");

  return (
    <Card>
      <h2 className="mb-1 text-sm font-semibold tracking-wide text-ink-soft uppercase">
        Occasion tags
      </h2>
      <p className="mb-3 text-xs text-ink-faint">
        Why you bought something — Diwali, wedding season, a test batch.
      </p>

      {tagsQuery.isPending && <Spinner label="Loading" />}
      {tagsQuery.isError && <ErrorNotice error={tagsQuery.error} />}

      <ul className="divide-y divide-wick-100">
        {tagsQuery.data?.map((tag) => (
          <EditableTagRow
            key={tag.occasion_tag_id}
            id={tag.occasion_tag_id}
            name={tag.tag_name}
            usageCount={tag.usage_count}
            onRename={(name) =>
              renameTag.mutate({ tagID: tag.occasion_tag_id, name })
            }
            onDelete={() => deleteTag.mutate(tag.occasion_tag_id)}
          />
        ))}
      </ul>

      <div className="mt-3 flex gap-2">
        <input
          value={newName}
          onChange={(event) => setNewName(event.target.value)}
          placeholder="New occasion tag"
          className="min-w-0 flex-1 rounded-xl border border-wick-200 bg-surface px-3 py-2.5 text-base outline-none placeholder:text-ink-faint focus:border-wick-500 focus:ring-2 focus:ring-wick-500/20"
        />
        <Button
          disabled={!newName.trim() || createTag.isPending}
          onClick={() =>
            createTag.mutate(newName.trim(), {
              onSuccess: () => setNewName(""),
            })
          }
        >
          Add
        </Button>
      </div>
    </Card>
  );
}

function EditableTagRow({
  name,
  usageCount,
  isProtected = false,
  onRename,
  onDelete,
}: {
  id: UUID;
  name: string;
  usageCount: number;
  isProtected?: boolean;
  onRename: (name: string) => void;
  onDelete: () => void;
}) {
  const [isEditing, setIsEditing] = useState(false);
  const [draftName, setDraftName] = useState(name);

  if (isEditing) {
    return (
      <li className="flex items-center gap-2 py-2">
        <input
          value={draftName}
          onChange={(event) => setDraftName(event.target.value)}
          autoFocus
          className="min-w-0 flex-1 rounded-lg border border-wick-300 bg-surface px-2.5 py-2 text-base outline-none focus:border-wick-500"
        />
        <Button
          onClick={() => {
            const trimmed = draftName.trim();
            if (trimmed && trimmed !== name) onRename(trimmed);
            setIsEditing(false);
          }}
        >
          Save
        </Button>
        <button
          onClick={() => {
            setDraftName(name);
            setIsEditing(false);
          }}
          className="px-2 text-sm text-ink-faint"
        >
          Cancel
        </button>
      </li>
    );
  }

  return (
    <li className="flex items-center gap-2 py-2.5">
      <span className="min-w-0 flex-1 truncate text-ink">{name}</span>
      {usageCount > 0 && (
        <span className="shrink-0 text-xs text-ink-faint">
          used {usageCount}×
        </span>
      )}
      {isProtected ? (
        <span className="shrink-0 rounded-full bg-surface-sunk px-2 py-0.5 text-[11px] text-ink-faint">
          built in
        </span>
      ) : (
        <>
          <button
            onClick={() => setIsEditing(true)}
            aria-label={`Rename ${name}`}
            className="grid size-9 shrink-0 place-items-center rounded-lg text-ink-faint hover:bg-wick-50 hover:text-ink"
          >
            ✎
          </button>
          <button
            onClick={() => {
              if (window.confirm(`Delete "${name}"?`)) onDelete();
            }}
            aria-label={`Delete ${name}`}
            className="grid size-9 shrink-0 place-items-center rounded-lg text-ink-faint hover:bg-rise/10 hover:text-rise"
          >
            🗑
          </button>
        </>
      )}
    </li>
  );
}

/** Scraping can fail quietly, so the last run per vendor is surfaced where it
 *  can be checked deliberately rather than only noticed by accident. */
function VendorHealthSection() {
  const vendorsQuery = useVendors();
  const runsQuery = useScrapeRuns();

  const latestRunByVendor = new Map(
    (runsQuery.data ?? [])
      .slice()
      .reverse()
      .map((run) => [run.vendor_slug, run]),
  );

  return (
    <Card>
      <h2 className="mb-1 text-sm font-semibold tracking-wide text-ink-soft uppercase">
        Vendor scraping
      </h2>
      <p className="mb-3 text-xs text-ink-faint">
        When each vendor's catalogue was last read successfully.
      </p>

      {vendorsQuery.isPending && <Spinner label="Loading" />}

      <ul className="divide-y divide-wick-100">
        {vendorsQuery.data?.map((vendor) => {
          const latestRun = latestRunByVendor.get(vendor.vendor_slug);
          const hasFailed = Boolean(vendor.last_scrape_error);
          return (
            <li key={vendor.vendor_id} className="py-2.5">
              <div className="flex items-center gap-2">
                <span className="min-w-0 flex-1 truncate text-sm text-ink">
                  {vendor.vendor_name}
                </span>
                <span
                  className={`shrink-0 text-xs ${
                    vendor.last_successful_scrape_at
                      ? "text-ink-faint"
                      : "text-rise"
                  }`}
                >
                  {formatRelativeTime(vendor.last_successful_scrape_at)}
                </span>
              </div>
              {hasFailed && (
                <p className="mt-1 line-clamp-2 text-xs text-rise">
                  Last error: {vendor.last_scrape_error}
                </p>
              )}
              {latestRun && (
                <p className="mt-0.5 text-[11px] text-ink-faint">
                  {latestRun.listings_seen} seen ·{" "}
                  {latestRun.listings_updated} updated ·{" "}
                  {latestRun.listings_delisted} delisted
                </p>
              )}
            </li>
          );
        })}
      </ul>
    </Card>
  );
}

function DangerZone() {
  const deleteAllData = useDeleteAllData();
  const [typedConfirmation, setTypedConfirmation] = useState("");
  const [isArmed, setIsArmed] = useState(false);

  return (
    <Card className="border-rise/30">
      <h2 className="mb-1 text-sm font-semibold tracking-wide text-rise uppercase">
        Delete all data
      </h2>
      <p className="mb-3 text-xs text-ink-soft">
        Removes every order, comparison, category and occasion tag. Vendors,
        products and their price history are kept — that history is collected one
        day at a time and cannot be recovered, while orders can be re-entered
        from receipts.
      </p>

      {!isArmed ? (
        <Button variant="ghost" onClick={() => setIsArmed(true)}>
          I want to delete everything
        </Button>
      ) : (
        <div className="space-y-3">
          <TextField
            label={`Type ${DELETE_CONFIRMATION} to confirm`}
            value={typedConfirmation}
            onChange={setTypedConfirmation}
            placeholder={DELETE_CONFIRMATION}
          />
          {deleteAllData.isError && <ErrorNotice error={deleteAllData.error} />}
          {deleteAllData.isSuccess && (
            <p className="text-sm text-fall">Your data has been deleted.</p>
          )}
          <div className="flex gap-2">
            <Button
              variant="danger"
              disabled={
                typedConfirmation !== DELETE_CONFIRMATION ||
                deleteAllData.isPending
              }
              onClick={() => deleteAllData.mutate(typedConfirmation)}
            >
              Delete everything
            </Button>
            <Button
              variant="ghost"
              onClick={() => {
                setIsArmed(false);
                setTypedConfirmation("");
              }}
            >
              Cancel
            </Button>
          </div>
        </div>
      )}
    </Card>
  );
}
