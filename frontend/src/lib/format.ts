import type { Decimal } from "../api/types";

const rupeeFormatter = new Intl.NumberFormat("en-IN", {
  style: "currency",
  currency: "INR",
  maximumFractionDigits: 2,
});

const rupeeWholeFormatter = new Intl.NumberFormat("en-IN", {
  style: "currency",
  currency: "INR",
  maximumFractionDigits: 0,
});

/** Formats an API decimal string for display. Whole amounts drop the paise,
 *  because "₹390.00" reads as noise next to "₹390" on a narrow screen. */
export function formatRupees(value: Decimal | null | undefined): string {
  if (value === null || value === undefined) return "—";
  const amount = Number(value);
  if (Number.isNaN(amount)) return "—";
  return Number.isInteger(amount)
    ? rupeeWholeFormatter.format(amount)
    : rupeeFormatter.format(amount);
}

/** Compact form for chart axes, where "₹1,20,000" will not fit. */
export function formatRupeesCompact(value: number): string {
  if (value >= 10_000_000) return `₹${(value / 10_000_000).toFixed(1)}Cr`;
  if (value >= 100_000) return `₹${(value / 100_000).toFixed(1)}L`;
  if (value >= 1_000) return `₹${(value / 1_000).toFixed(1)}k`;
  return `₹${value}`;
}

export function toNumber(value: Decimal | null | undefined): number {
  if (value === null || value === undefined) return 0;
  const parsed = Number(value);
  return Number.isNaN(parsed) ? 0 : parsed;
}

const dateFormatter = new Intl.DateTimeFormat("en-IN", {
  day: "numeric",
  month: "short",
  year: "numeric",
});

export function formatDate(value: string | null | undefined): string {
  if (!value) return "—";
  const parsed = new Date(value);
  return Number.isNaN(parsed.getTime()) ? "—" : dateFormatter.format(parsed);
}

/** "2026-08" from the monthly trend endpoint into "Aug 26" for a chart axis. */
export function formatMonthLabel(month: string): string {
  const [year, monthNumber] = month.split("-");
  const parsed = new Date(Number(year), Number(monthNumber) - 1, 1);
  if (Number.isNaN(parsed.getTime())) return month;
  return `${parsed.toLocaleString("en-IN", { month: "short" })} ${year.slice(2)}`;
}

/** Staleness is the point of showing a scrape timestamp at all (FR-X-3), so it
 *  is phrased as an age rather than a date the reader has to subtract from. */
export function formatRelativeTime(value: string | null | undefined): string {
  if (!value) return "never scraped";
  const parsed = new Date(value);
  if (Number.isNaN(parsed.getTime())) return "never scraped";

  const elapsedMinutes = Math.round((Date.now() - parsed.getTime()) / 60_000);
  if (elapsedMinutes < 1) return "just now";
  if (elapsedMinutes < 60) return `${elapsedMinutes} min ago`;

  const elapsedHours = Math.round(elapsedMinutes / 60);
  if (elapsedHours < 24) {
    return `${elapsedHours} hour${elapsedHours === 1 ? "" : "s"} ago`;
  }

  const elapsedDays = Math.round(elapsedHours / 24);
  if (elapsedDays < 30) {
    return `${elapsedDays} day${elapsedDays === 1 ? "" : "s"} ago`;
  }
  return formatDate(value);
}

/** The date inputs and the API both speak YYYY-MM-DD. */
export function toDateInputValue(date: Date): string {
  const month = String(date.getMonth() + 1).padStart(2, "0");
  const day = String(date.getDate()).padStart(2, "0");
  return `${date.getFullYear()}-${month}-${day}`;
}

export function today(): string {
  return toDateInputValue(new Date());
}

export function daysAgo(dayCount: number): string {
  const date = new Date();
  date.setDate(date.getDate() - dayCount);
  return toDateInputValue(date);
}

export function monthsAgo(monthCount: number): string {
  const date = new Date();
  date.setMonth(date.getMonth() - monthCount);
  return toDateInputValue(date);
}

/** Vendor names arrive HTML-escaped from the scraped source ("Candle &#038;
 *  Soap"). Decoding once at the display boundary keeps the stored value
 *  faithful to what the vendor published. */
export function decodeEntities(text: string): string {
  if (!text.includes("&")) return text;
  const element = document.createElement("textarea");
  element.innerHTML = text;
  return element.value;
}

const statusLabels: Record<string, string> = {
  placed: "Placed",
  cancelled: "Cancelled",
  refunded: "Refunded",
  partially_refunded: "Partly refunded",
};

export function formatOrderStatus(status: string): string {
  return statusLabels[status] ?? status;
}
