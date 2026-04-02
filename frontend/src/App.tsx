import { AppProviders } from "./app/providers";
import { AppShell } from "./components/layout/AppShell";
import { DashboardPage } from "./pages/DashboardPage";
import { PairDetailPage } from "./pages/PairDetailPage";
import { PairsPage } from "./pages/PairsPage";
import { SystemStatusPage } from "./pages/SystemStatusPage";

export default function App() {
  return (
    <AppProviders>
      <AppShell>
        <DashboardPage />
        <PairsPage />
        <PairDetailPage />
        <SystemStatusPage />
      </AppShell>
    </AppProviders>
  );
}
