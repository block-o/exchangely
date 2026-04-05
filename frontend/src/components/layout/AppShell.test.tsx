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

  it("renders api docs nav item and github icon link in the hero", () => {
    function AlphaView() {
      return <div>Alpha Page</div>;
    }

    function BetaView() {
      return <div>Beta Page</div>;
    }

    render(
      <SettingsProvider>
        <AppShell>
          <AlphaView />
          <BetaView />
        </AppShell>
      </SettingsProvider>
    );

    expect(screen.getByRole("link", { name: "GitHub project" })).toHaveAttribute(
      "href",
      "https://github.com/block-o/exchangely"
    );
    expect(screen.getByRole("link", { name: "API Docs" })).toHaveAttribute(
      "href",
      "http://localhost:8080/swagger"
    );
  });
});
