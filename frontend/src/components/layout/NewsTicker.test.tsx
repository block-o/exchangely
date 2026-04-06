import { render, screen, waitFor } from "@testing-library/react";
import "@testing-library/jest-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { NewsTicker } from "./NewsTicker";
import * as newsApi from "../../api/news";

vi.mock("../../api/news", () => ({
  getNews: vi.fn(),
}));

class MockEventSource {
  close = vi.fn();
  onopen: ((event: Event) => void) | null = null;
  onmessage: ((event: MessageEvent<string>) => void) | null = null;
  onerror: ((event: Event) => void) | null = null;
  constructor(_url: string) {}
}

describe("NewsTicker", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    globalThis.EventSource = MockEventSource as any;
  });

  it("renders null when no news are available", async () => {
    vi.mocked(newsApi.getNews).mockResolvedValue([]);
    const { container } = render(<NewsTicker />);
    
    await waitFor(() => {
      expect(container.firstChild).toBeNull();
    });
  });

  it("renders headlines and source when news are available", async () => {
    vi.mocked(newsApi.getNews).mockResolvedValue([
      {
        id: "1",
        title: "BTC to the moon",
        link: "https://example.com/1",
        source: "CoinDesk",
        published_at: new Date().toISOString(),
      },
    ]);

    render(<NewsTicker assets={["BTC"]} />);

    await waitFor(() => {
      expect(screen.getAllByText("BTC")[0]).toBeInTheDocument();
      expect(screen.getAllByText(/to the moon/)[0]).toBeInTheDocument();
      expect(screen.getAllByText("CoinDesk")[0]).toBeInTheDocument();
    });

    // Verify highlighting
    const highlighted = screen.getAllByText("BTC")[0];
    expect(highlighted).toHaveClass("news-highlight");
  });

  it("duplicates news for smooth scrolling", async () => {
    vi.mocked(newsApi.getNews).mockResolvedValue([
      {
        id: "1",
        title: "Headline 1",
        link: "https://example.com/1",
        source: "Source",
        published_at: new Date().toISOString(),
      },
    ]);

    render(<NewsTicker />);

    await waitFor(() => {
      const items = screen.getAllByText("Headline 1");
      expect(items.length).toBe(2); // Duplicated
    });
  });
});
