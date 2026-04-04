import { render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { SettingsProvider } from "../../app/settings";
import { AppShell } from "./AppShell";

describe("AppShell", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    window.location.hash = "";
  });

  it("renders pages by section order instead of component names", async () => {
    function AlphaView() {
      return <div>Alpha Page</div>;
    }

    function BetaView() {
      return <div>Beta Page</div>;
    }

    window.location.hash = "#system";

    render(
      <SettingsProvider>
        <AppShell>
          <AlphaView />
          <BetaView />
        </AppShell>
      </SettingsProvider>
    );

    await waitFor(() => {
      expect(screen.getByText("Beta Page")).toBeInTheDocument();
    });
  });
});
