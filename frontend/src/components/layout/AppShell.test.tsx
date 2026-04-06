import { render, screen, waitFor } from "@testing-library/react";
import "@testing-library/jest-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { SettingsProvider } from "../../app/settings";
import { AppShell } from "./AppShell";
import * as newsApi from "../../api/news";

vi.mock("../../api/news", () => ({ getNews: vi.fn() }));

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
    vi.mocked(newsApi.getNews).mockResolvedValue([]);
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
    vi.mocked(newsApi.getNews).mockResolvedValue([
      {
        id: "1",
        title: "Test News",
        link: "https://example.com",
        source: "Source",
        published_at: new Date().toISOString(),
      },
    ]);

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

    // Wait for the news ticker to appear to avoid unhandled state updates causing act() warnings
    await waitFor(() => {
      expect(screen.getByText("Latest News")).toBeInTheDocument();
    });

    expect(screen.getByRole("link", { name: "GitHub project" })).toHaveAttribute(
      "href",
      "https://github.com/block-o/exchangely"
    );
    expect(screen.getByRole("link", { name: "API Docs" })).toHaveAttribute(
      "href",
      "http://localhost:8080/swagger"
    );
  });

  it("renders news ticker with headlines", async () => {
    vi.mocked(newsApi.getNews).mockResolvedValue([
      {
        id: "news-1",
        title: "Bitcoin breaks 100k",
        link: "https://coindesk.com/1",
        source: "CoinDesk",
        published_at: new Date().toISOString(),
      },
    ]);

    render(
      <SettingsProvider>
        <AppShell>
          <div>Page</div>
        </AppShell>
      </SettingsProvider>
    );

    // Should eventually show the ticker label and headline
    await waitFor(() => {
      expect(screen.getByText("Latest News")).toBeInTheDocument();
      expect(screen.getAllByText("Bitcoin breaks 100k")[0]).toBeInTheDocument();
    });
  });
});
