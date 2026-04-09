import { render, screen, waitFor } from "@testing-library/react";
import "@testing-library/jest-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { SettingsProvider } from "../../app/settings";
import { AppShell } from "./AppShell";

class MockEventSource {
  close = vi.fn();
  onopen: ((event: Event) => void) | null = null;
  onmessage: ((event: MessageEvent<string>) => void) | null = null;
  onerror: ((event: Event) => void) | null = null;
  constructor(_url: string) {}
}

describe("AppShell", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    window.location.hash = "";
    globalThis.EventSource = MockEventSource as any;
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

  it("renders api docs nav item and github icon link in the hero", async () => {
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

    await waitFor(() => {
      expect(screen.getByText("Alpha Page")).toBeInTheDocument();
    });

    const githubLinks = screen.getAllByRole("link", { name: "GitHub project" });
    expect(githubLinks.length).toBeGreaterThanOrEqual(1);
    expect(githubLinks[0]).toHaveAttribute(
      "href",
      "https://github.com/block-o/exchangely"
    );
    const apiDocsLinks = screen.getAllByRole("link", { name: "API Docs" });
    expect(apiDocsLinks.length).toBeGreaterThanOrEqual(1);
    expect(apiDocsLinks[0]).toHaveAttribute(
      "href",
      "http://localhost:8080/swagger"
    );
  });
});
