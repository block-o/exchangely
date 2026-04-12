import { AppProviders } from "./app/providers";
import { AppShell } from "./components/layout/AppShell";
import { MarketPage } from "./pages/MarketPage";
import { PortfolioPage } from "./pages/PortfolioPage";
import { SystemPage } from "./pages/SystemPage";

export default function App() {
  return (
    <AppProviders>
      <AppShell>
        <MarketPage />
        <PortfolioPage />
        <SystemPage />
      </AppShell>
    </AppProviders>
  );
}
