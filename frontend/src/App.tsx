import { lazy } from "react";
import { Navigate, Route, Routes } from "react-router-dom";
import { useSession } from "./auth/SessionProvider";
import { AppShell } from "./components/AppShell";
import { Spinner } from "./components/ui";
import { LoginPage } from "./pages/LoginPage";

// Routes are split so the charting library — by far the largest dependency —
// is fetched only when a page that draws a chart is opened. This is a phone
// app on mobile data; the first screen should not carry the whole bundle.
// AppShell holds the Suspense boundary these resolve inside.
const VendorsPage = lazy(() =>
  import("./pages/VendorsPage").then((module) => ({
    default: module.VendorsPage,
  })),
);
const ListingPage = lazy(() =>
  import("./pages/ListingPage").then((module) => ({
    default: module.ListingPage,
  })),
);
const CompareListPage = lazy(() =>
  import("./pages/ComparePage").then((module) => ({
    default: module.CompareListPage,
  })),
);
const CompareEntryPage = lazy(() =>
  import("./pages/ComparePage").then((module) => ({
    default: module.CompareEntryPage,
  })),
);
const OrdersListPage = lazy(() =>
  import("./pages/OrdersPage").then((module) => ({
    default: module.OrdersListPage,
  })),
);
const OrderDetailPage = lazy(() =>
  import("./pages/OrdersPage").then((module) => ({
    default: module.OrderDetailPage,
  })),
);
const SalesListPage = lazy(() =>
  import("./pages/SalesPage").then((module) => ({
    default: module.SalesListPage,
  })),
);
const SaleOrderDetailPage = lazy(() =>
  import("./pages/SalesPage").then((module) => ({
    default: module.SaleOrderDetailPage,
  })),
);
const SpendPage = lazy(() =>
  import("./pages/SpendPage").then((module) => ({ default: module.SpendPage })),
);
const SettingsPage = lazy(() =>
  import("./pages/SettingsPage").then((module) => ({
    default: module.SettingsPage,
  })),
);

export function App() {
  const { session, isLoading } = useSession();

  // Waiting for the stored session to be read. Rendering the sign-in screen
  // here would flash it at someone who is already signed in.
  if (isLoading) {
    return (
      <div className="grid min-h-dvh place-items-center bg-surface-sunk">
        <Spinner label="" />
      </div>
    );
  }

  // The gate is here rather than per route, so a page added later is private
  // by default instead of private only if somebody remembered.
  if (!session) return <LoginPage />;

  return (
    <Routes>
      <Route element={<AppShell />}>
        {/* P1 */}
        <Route path="/" element={<VendorsPage />} />
        <Route path="/vendors/:vendorSlug" element={<VendorsPage />} />
        <Route path="/listings/:listingID" element={<ListingPage />} />
        {/* P2 */}
        <Route path="/compare" element={<CompareListPage />} />
        <Route path="/compare/:entryID" element={<CompareEntryPage />} />
        {/* P3 */}
        <Route path="/orders" element={<OrdersListPage />} />
        <Route path="/orders/:orderEntryID" element={<OrderDetailPage />} />
        {/* P6 */}
        <Route path="/sales" element={<SalesListPage />} />
        <Route
          path="/sales/:saleOrderEntryID"
          element={<SaleOrderDetailPage />}
        />
        {/* P4 */}
        <Route path="/spend" element={<SpendPage />} />
        {/* P5 */}
        <Route path="/settings" element={<SettingsPage />} />
        <Route path="*" element={<Navigate to="/" replace />} />
      </Route>
    </Routes>
  );
}
