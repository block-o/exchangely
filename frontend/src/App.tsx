import { AppProviders } from "./app/providers";
import { AppShell } from "./components/layout/AppShell";
import { MarketPage } from "./pages/MarketPage";
import { SystemPage } from "./pages/SystemPage";

export default function App() {
  return (
    <AppProviders>
      <AppShell>
        <MarketPage />
        <SystemPage />
      </AppShell>
    </AppProviders>
  );
}
