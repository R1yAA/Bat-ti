import { useState } from "react";
import {
  Bar,
  BarChart,
  CartesianGrid,
  Cell,
  Legend,
  Pie,
  PieChart,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from "recharts";
import {
  useMonthlySpendTrend,
  useSpendByCategory,
  useSpendByOccasion,
  useSpendSummary,
} from "../api/queries";
import { PageHeading } from "../components/AppShell";
import { Card, ErrorNotice, Spinner, TextField } from "../components/ui";
import {
  daysAgo,
  formatMonthLabel,
  formatRupees,
  formatRupeesCompact,
  monthsAgo,
  today,
  toNumber,
} from "../lib/format";

// FR-P4-1. Presets are resolved to a date pair here, so the API only ever has
// to understand a custom range.
const presets = [
  { label: "1 day", start: () => daysAgo(1) },
  { label: "1 week", start: () => daysAgo(7) },
  { label: "1 month", start: () => monthsAgo(1) },
  { label: "3 months", start: () => monthsAgo(3) },
  { label: "6 months", start: () => monthsAgo(6) },
  { label: "1 year", start: () => monthsAgo(12) },
];

const sliceColours = [
  "#c97a35",
  "#8a4826",
  "#d69350",
  "#713b25",
  "#e2b478",
  "#5d3222",
  "#eed3ab",
  "#ab5f2a",
];

export function SpendPage() {
  const [startDate, setStartDate] = useState(monthsAgo(1));
  const [endDate, setEndDate] = useState(today());
  const [activePreset, setActivePreset] = useState("1 month");
  // FR-P4-4: the toggle only changes what is shown. The default number stays
  // the clean one, with cancelled and refunded left out (BR-12).
  const [includeExcluded, setIncludeExcluded] = useState(false);

  const summaryQuery = useSpendSummary(startDate, endDate);
  const categoryQuery = useSpendByCategory(startDate, endDate);
  const occasionQuery = useSpendByOccasion(startDate, endDate);
  const monthlyQuery = useMonthlySpendTrend();

  const applyPreset = (preset: (typeof presets)[number]) => {
    setStartDate(preset.start());
    setEndDate(today());
    setActivePreset(preset.label);
  };

  const summary = summaryQuery.data;
  const headlineSpend = summary
    ? includeExcluded
      ? summary.gross_spend
      : summary.net_spend
    : null;

  return (
    <div className="space-y-4">
      <PageHeading title="Spend" subtitle="Where the money went" />

      <div className="swipe-x -mx-4 px-4">
        <div className="flex gap-2 pb-1">
          {presets.map((preset) => (
            <button
              key={preset.label}
              onClick={() => applyPreset(preset)}
              className={`shrink-0 rounded-full px-4 py-2 text-sm font-medium whitespace-nowrap transition ${
                activePreset === preset.label
                  ? "bg-wick-600 text-white"
                  : "border border-wick-200 bg-surface text-ink-soft hover:bg-wick-50"
              }`}
            >
              {preset.label}
            </button>
          ))}
        </div>
      </div>

      <Card>
        <div className="grid grid-cols-2 gap-3">
          <TextField
            label="From"
            type="date"
            value={startDate}
            onChange={(value) => {
              setStartDate(value);
              setActivePreset("");
            }}
          />
          <TextField
            label="To"
            type="date"
            value={endDate}
            onChange={(value) => {
              setEndDate(value);
              setActivePreset("");
            }}
          />
        </div>
      </Card>

      {summaryQuery.isError && <ErrorNotice error={summaryQuery.error} />}
      {summaryQuery.isPending && <Spinner label="Adding it up" />}

      {summary && (
        <Card>
          <p className="text-xs font-medium tracking-wide text-ink-soft uppercase">
            {includeExcluded ? "Total including cancelled" : "Total spent"}
          </p>
          <p className="mt-1 text-4xl font-semibold tracking-tight text-ink">
            {formatRupees(headlineSpend)}
          </p>
          <p className="mt-1 text-sm text-ink-soft">
            {summary.order_count}{" "}
            {summary.order_count === 1 ? "order" : "orders"}, {summary.item_count}{" "}
            {summary.item_count === 1 ? "item" : "items"}
          </p>

          {toNumber(summary.excluded_spend) > 0 && (
            <label className="mt-3 flex items-center gap-2.5 border-t border-wick-100 pt-3 text-sm text-ink-soft">
              <input
                type="checkbox"
                checked={includeExcluded}
                onChange={(event) => setIncludeExcluded(event.target.checked)}
                className="size-4 accent-wick-600"
              />
              Include cancelled and refunded (
              {formatRupees(summary.excluded_spend)})
            </label>
          )}

          {toNumber(summary.refunded_amount) > 0 && (
            <p className="mt-2 text-xs text-ink-faint">
              {formatRupees(summary.refunded_amount)} received back in refunds.
            </p>
          )}
        </Card>
      )}

      <DonutCard
        title="By category"
        hint="Your own labels, set per order item."
        isPending={categoryQuery.isPending}
        error={categoryQuery.error}
        slices={(categoryQuery.data ?? []).map((row) => ({
          name: row.category_name,
          amount: toNumber(includeExcluded ? row.gross_spend : row.net_spend),
        }))}
      />

      <DonutCard
        title="By occasion"
        hint="Why you bought it — Diwali, wedding season, a test batch."
        isPending={occasionQuery.isPending}
        error={occasionQuery.error}
        slices={(occasionQuery.data ?? []).map((row) => ({
          name: row.tag_name,
          amount: toNumber(includeExcluded ? row.gross_spend : row.net_spend),
        }))}
      />

      <Card>
        <h2 className="text-sm font-semibold tracking-wide text-ink-soft uppercase">
          Last 12 months
        </h2>
        {/* FR-P4-6: deliberately ignores the range picker above — this graph is
            for pacing across the year, which a custom range would undermine. */}
        <p className="mb-3 text-xs text-ink-faint">
          Always the trailing year, whatever range is set above.
        </p>
        {monthlyQuery.isPending && <Spinner label="Loading" />}
        {monthlyQuery.isError && <ErrorNotice error={monthlyQuery.error} />}
        {monthlyQuery.data && (
          <div className="h-64 w-full">
            <ResponsiveContainer width="100%" height="100%">
              <BarChart
                data={monthlyQuery.data.map((row) => ({
                  month: formatMonthLabel(row.month),
                  amount: toNumber(
                    includeExcluded ? row.gross_spend : row.net_spend,
                  ),
                }))}
                margin={{ top: 4, right: 4, bottom: 28, left: 0 }}
              >
                <CartesianGrid vertical={false} stroke="#f7ead6" />
                {/* interval={0} forces every month to be labelled. Twelve
                    labels do not fit horizontally on a phone, so they are
                    angled rather than silently dropped. */}
                <XAxis
                  dataKey="month"
                  tick={{ fontSize: 10, fill: "#9a877c" }}
                  tickLine={false}
                  axisLine={false}
                  interval={0}
                  angle={-45}
                  textAnchor="end"
                  height={44}
                />
                <YAxis
                  tickFormatter={formatRupeesCompact}
                  tick={{ fontSize: 11, fill: "#9a877c" }}
                  tickLine={false}
                  axisLine={false}
                  width={52}
                />
                <Tooltip
                  cursor={{ fill: "#f7f1e7" }}
                  formatter={(value) => [formatRupees(String(value)), "Spent"]}
                  contentStyle={{
                    borderRadius: 12,
                    border: "1px solid #eed3ab",
                    fontSize: 13,
                  }}
                />
                <Bar dataKey="amount" fill="#c97a35" radius={[6, 6, 0, 0]} />
              </BarChart>
            </ResponsiveContainer>
          </div>
        )}
      </Card>
    </div>
  );
}

/** A donut rather than a pie: the hole carries the total, so the chart answers
 *  "how much altogether" and "how was it split" at once. */
function DonutCard({
  title,
  hint,
  isPending,
  error,
  slices,
}: {
  title: string;
  hint: string;
  isPending: boolean;
  error: unknown;
  slices: { name: string; amount: number }[];
}) {
  const spendingSlices = slices.filter((slice) => slice.amount > 0);
  const total = spendingSlices.reduce((sum, slice) => sum + slice.amount, 0);

  return (
    <Card>
      <h2 className="text-sm font-semibold tracking-wide text-ink-soft uppercase">
        {title}
      </h2>
      <p className="mb-2 text-xs text-ink-faint">{hint}</p>

      {isPending && <Spinner label="Loading" />}
      {Boolean(error) && <ErrorNotice error={error} />}

      {!isPending && !error && spendingSlices.length === 0 && (
        <p className="py-6 text-center text-sm text-ink-faint">
          No spending in this period.
        </p>
      )}

      {spendingSlices.length > 0 && (
        <div className="relative">
          <div className="h-72 w-full">
            <ResponsiveContainer width="100%" height="100%">
              <PieChart>
                <Pie
                  data={spendingSlices}
                  dataKey="amount"
                  nameKey="name"
                  cx="50%"
                  cy="42%"
                  innerRadius="52%"
                  outerRadius="76%"
                  paddingAngle={2}
                  stroke="none"
                >
                  {spendingSlices.map((slice, index) => (
                    <Cell
                      key={slice.name}
                      fill={sliceColours[index % sliceColours.length]}
                    />
                  ))}
                </Pie>
                <Tooltip
                  formatter={(value, name) => [
                    `${formatRupees(String(value))} · ${((Number(value) / total) * 100).toFixed(0)}%`,
                    String(name),
                  ]}
                  contentStyle={{
                    borderRadius: 12,
                    border: "1px solid #eed3ab",
                    fontSize: 13,
                  }}
                />
                <Legend
                  verticalAlign="bottom"
                  align="center"
                  iconType="circle"
                  iconSize={9}
                  formatter={(value: string) => (
                    <span style={{ fontSize: 12, color: "#6b564a" }}>
                      {value}
                    </span>
                  )}
                />
              </PieChart>
            </ResponsiveContainer>
          </div>

          {/* The total sits in the hole. pointer-events-none so it never
              intercepts a tap meant for a slice. */}
          <div className="pointer-events-none absolute inset-x-0 top-[42%] -translate-y-1/2 text-center">
            <p className="text-[11px] tracking-wide text-ink-faint uppercase">
              Total
            </p>
            <p className="text-xl font-semibold text-ink">
              {formatRupees(String(total))}
            </p>
          </div>
        </div>
      )}
    </Card>
  );
}
