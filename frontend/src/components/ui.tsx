import type { ReactNode } from "react";
import type { PriceDirection } from "../api/types";

/** BR-16: red up, green down, nothing at all when there is no change or no
 *  earlier price. The direction is decided by the API so every view agrees. */
export function PriceArrow({ direction }: { direction: PriceDirection }) {
  if (direction === "none") return null;
  const isRise = direction === "up";
  return (
    <span
      className={`inline-flex items-center text-xs font-semibold ${
        isRise ? "text-rise" : "text-fall"
      }`}
      title={isRise ? "Price went up" : "Price came down"}
    >
      {isRise ? "▲" : "▼"}
    </span>
  );
}

export function StockBadge({ isInStock }: { isInStock: boolean }) {
  if (isInStock) return null;
  // BR-15: out of stock is flagged, never hidden.
  return (
    <span className="rounded-full bg-ink-faint/15 px-2 py-0.5 text-[11px] font-medium text-ink-soft">
      Out of stock
    </span>
  );
}

export function DelistedBadge({ isDelisted }: { isDelisted: boolean }) {
  if (!isDelisted) return null;
  return (
    <span className="rounded-full bg-rise/10 px-2 py-0.5 text-[11px] font-medium text-rise">
      Removed by vendor
    </span>
  );
}

export function Spinner({ label = "Loading" }: { label?: string }) {
  return (
    <div className="flex items-center justify-center gap-2 py-10 text-sm text-ink-soft">
      <span className="size-4 animate-spin rounded-full border-2 border-wick-300 border-t-wick-600" />
      {label}
    </div>
  );
}

export function EmptyState({
  title,
  hint,
  action,
}: {
  title: string;
  hint?: string;
  action?: ReactNode;
}) {
  return (
    <div className="rounded-2xl border border-dashed border-wick-200 bg-surface px-6 py-10 text-center">
      <p className="font-medium text-ink">{title}</p>
      {hint && <p className="mt-1 text-sm text-ink-soft">{hint}</p>}
      {action && <div className="mt-4">{action}</div>}
    </div>
  );
}

export function ErrorNotice({ error }: { error: unknown }) {
  const message =
    error instanceof Error ? error.message : "Something went wrong";
  return (
    <div
      role="alert"
      className="rounded-xl border border-rise/25 bg-rise/5 px-4 py-3 text-sm text-rise"
    >
      {message}
    </div>
  );
}

export function Button({
  children,
  onClick,
  variant = "primary",
  type = "button",
  disabled,
  className = "",
}: {
  children: ReactNode;
  onClick?: () => void;
  variant?: "primary" | "ghost" | "danger";
  type?: "button" | "submit";
  disabled?: boolean;
  className?: string;
}) {
  const variants = {
    primary: "bg-wick-600 text-white hover:bg-wick-700 active:bg-wick-800",
    ghost:
      "bg-surface text-ink border border-wick-200 hover:bg-wick-50 active:bg-wick-100",
    danger: "bg-rise text-white hover:brightness-95 active:brightness-90",
  };
  return (
    <button
      type={type}
      onClick={onClick}
      disabled={disabled}
      // 44px minimum touch target — this is used one-handed on a phone.
      className={`inline-flex min-h-11 items-center justify-center gap-1.5 rounded-xl px-4 text-sm font-medium transition disabled:cursor-not-allowed disabled:opacity-50 ${variants[variant]} ${className}`}
    >
      {children}
    </button>
  );
}

export function TextField({
  label,
  value,
  onChange,
  placeholder,
  type = "text",
  inputMode,
  required,
}: {
  label: string;
  value: string;
  onChange: (value: string) => void;
  placeholder?: string;
  type?: string;
  inputMode?: "text" | "numeric" | "decimal";
  required?: boolean;
}) {
  return (
    <label className="block">
      <span className="mb-1 block text-xs font-medium tracking-wide text-ink-soft uppercase">
        {label}
      </span>
      <input
        type={type}
        inputMode={inputMode}
        value={value}
        required={required}
        placeholder={placeholder}
        onChange={(event) => onChange(event.target.value)}
        className="w-full rounded-xl border border-wick-200 bg-surface px-3 py-2.5 text-base text-ink outline-none placeholder:text-ink-faint focus:border-wick-500 focus:ring-2 focus:ring-wick-500/20"
      />
    </label>
  );
}

export function SelectField({
  label,
  value,
  onChange,
  children,
}: {
  label: string;
  value: string;
  onChange: (value: string) => void;
  children: ReactNode;
}) {
  return (
    <label className="block">
      <span className="mb-1 block text-xs font-medium tracking-wide text-ink-soft uppercase">
        {label}
      </span>
      <select
        value={value}
        onChange={(event) => onChange(event.target.value)}
        className="w-full rounded-xl border border-wick-200 bg-surface px-3 py-2.5 text-base text-ink outline-none focus:border-wick-500 focus:ring-2 focus:ring-wick-500/20"
      >
        {children}
      </select>
    </label>
  );
}

/** A bottom sheet on a phone, a centred dialog once there is room for one. */
export function Sheet({
  title,
  isOpen,
  onClose,
  children,
}: {
  title: string;
  isOpen: boolean;
  onClose: () => void;
  children: ReactNode;
}) {
  if (!isOpen) return null;
  return (
    <div className="fixed inset-0 z-50 flex items-end justify-center sm:items-center">
      <div
        className="absolute inset-0 bg-ink/40 backdrop-blur-[2px]"
        onClick={onClose}
        aria-hidden
      />
      <div className="relative max-h-[88dvh] w-full overflow-y-auto rounded-t-3xl bg-surface p-5 shadow-2xl sm:max-w-lg sm:rounded-3xl">
        <div className="mb-4 flex items-center justify-between gap-4">
          <h2 className="text-lg font-semibold text-ink">{title}</h2>
          <button
            onClick={onClose}
            aria-label="Close"
            className="grid size-9 place-items-center rounded-full text-ink-soft hover:bg-wick-50"
          >
            ✕
          </button>
        </div>
        {children}
      </div>
    </div>
  );
}

export function Card({
  children,
  className = "",
}: {
  children: ReactNode;
  className?: string;
}) {
  return (
    <div
      className={`rounded-2xl border border-wick-100 bg-surface p-4 shadow-sm ${className}`}
    >
      {children}
    </div>
  );
}

/** A 1–10 rating, shown compactly. */
export function RatingPill({
  rating,
  count,
}: {
  rating: number;
  count?: number;
}) {
  if (rating <= 0) {
    return <span className="text-sm text-ink-faint">Not rated</span>;
  }
  return (
    <span className="inline-flex items-baseline gap-1 rounded-lg bg-wick-100 px-2 py-1 text-sm font-semibold text-wick-800">
      {rating.toFixed(1)}
      <span className="text-[11px] font-normal text-wick-700">/10</span>
      {count !== undefined && count > 0 && (
        <span className="text-[11px] font-normal text-wick-700">
          ({count})
        </span>
      )}
    </span>
  );
}
