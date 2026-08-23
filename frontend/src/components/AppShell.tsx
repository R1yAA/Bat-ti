import { Suspense } from "react";
import { NavLink, Outlet } from "react-router-dom";
import { Spinner } from "./ui";

const navigationItems = [
  { to: "/", label: "Vendors", icon: "🏬", end: true },
  { to: "/compare", label: "Compare", icon: "⚖️", end: false },
  { to: "/orders", label: "Orders", icon: "🧾", end: false },
  { to: "/sales", label: "Sales", icon: "💰", end: false },
  { to: "/spend", label: "Spend", icon: "📊", end: false },
  { to: "/settings", label: "Settings", icon: "⚙️", end: false },
];

/** Phone-first (D9): navigation sits at the bottom, in thumb reach, and moves
 *  into the header once the screen is wide enough to carry it there. */
export function AppShell() {
  return (
    <div className="min-h-dvh bg-surface-sunk">
      <header className="sticky top-0 z-30 border-b border-wick-100 bg-surface/85 backdrop-blur">
        <div className="mx-auto flex max-w-5xl items-center gap-4 px-4 py-3">
          <NavLink to="/" className="flex items-center gap-2">
            <span className="text-xl" aria-hidden>
              🕯️
            </span>
            <span className="text-lg font-semibold tracking-tight text-ink">
              Bat-ti
            </span>
          </NavLink>

          <nav className="ml-auto hidden gap-1 sm:flex">
            {navigationItems.map((item) => (
              <NavLink
                key={item.to}
                to={item.to}
                end={item.end}
                className={({ isActive }) =>
                  `rounded-lg px-3 py-2 text-sm font-medium transition ${
                    isActive
                      ? "bg-wick-100 text-wick-800"
                      : "text-ink-soft hover:bg-wick-50 hover:text-ink"
                  }`
                }
              >
                {item.label}
              </NavLink>
            ))}
          </nav>
        </div>
      </header>

      {/* The bottom padding clears the mobile tab bar and the home indicator. */}
      <main className="mx-auto max-w-5xl px-4 pt-4 pb-[calc(5.5rem+env(safe-area-inset-bottom))] sm:pb-10">
        {/* The boundary the lazily-loaded routes in App.tsx resolve inside. */}
        <Suspense fallback={<Spinner />}>
          <Outlet />
        </Suspense>
      </main>

      <nav className="fixed inset-x-0 bottom-0 z-30 border-t border-wick-100 bg-surface/95 pb-[env(safe-area-inset-bottom)] backdrop-blur sm:hidden">
        <div className="flex">
          {navigationItems.map((item) => (
            <NavLink
              key={item.to}
              to={item.to}
              end={item.end}
              className={({ isActive }) =>
                `flex flex-1 flex-col items-center gap-0.5 py-2.5 text-[11px] font-medium transition ${
                  isActive ? "text-wick-700" : "text-ink-faint"
                }`
              }
            >
              <span className="text-lg leading-none" aria-hidden>
                {item.icon}
              </span>
              {item.label}
            </NavLink>
          ))}
        </div>
      </nav>
    </div>
  );
}

export function PageHeading({
  title,
  subtitle,
  action,
}: {
  title: string;
  subtitle?: string;
  action?: React.ReactNode;
}) {
  return (
    <div className="mb-4 flex items-start justify-between gap-3">
      <div className="min-w-0">
        <h1 className="text-2xl font-semibold tracking-tight text-ink">
          {title}
        </h1>
        {subtitle && <p className="mt-0.5 text-sm text-ink-soft">{subtitle}</p>}
      </div>
      {action}
    </div>
  );
}
