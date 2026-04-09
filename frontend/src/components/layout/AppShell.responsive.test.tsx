import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import "@testing-library/jest-dom";
import { beforeEach, afterEach, describe, expect, it, vi } from "vitest";
import { SettingsProvider } from "../../app/settings";
import { AppShell } from "./AppShell";

type ChangeListener = (ev: MediaQueryListEvent) => void;

function createMockMediaQueryList(matches: boolean) {
  const listeners: ChangeListener[] = [];
  return {
    matches,
    media: "",
    onchange: null as ((ev: MediaQueryListEvent) => void) | null,
    addEventListener: vi.fn((_event: string, cb: ChangeListener) => {
      listeners.push(cb);
    }),
    removeEventListener: vi.fn((_event: string, cb: ChangeListener) => {
      const idx = listeners.indexOf(cb);
      if (idx >= 0) listeners.splice(idx, 1);
    }),
    addListener: vi.fn(),
    removeListener: vi.fn(),
    dispatchEvent: vi.fn(),
  };
}

class MockEventSource {
  close = vi.fn();
  onopen: ((event: Event) => void) | null = null;
  onmessage: ((event: MessageEvent<string>) => void) | null = null;
  onerror: ((event: Event) => void) | null = null;
  constructor(_url: string) {}
}

function setupMatchMedia(mobileMatches: boolean, tabletMatches: boolean) {
  const mobileMql = createMockMediaQueryList(mobileMatches);
  const tabletMql = createMockMediaQueryList(tabletMatches);

  window.matchMedia = vi.fn((query: string) => {
    if (query === "(max-width: 639px)") return mobileMql as unknown as MediaQueryList;
    if (query === "(min-width: 640px) and (max-width: 1023px)") return tabletMql as unknown as MediaQueryList;
    return createMockMediaQueryList(false) as unknown as MediaQueryList;
  });
}

function DummyPage() {
  return <div>Page Content</div>;
}

function renderShell() {
  return render(
    <SettingsProvider>
      <AppShell>
        <DummyPage />
        <DummyPage />
      </AppShell>
    </SettingsProvider>
  );
}

describe("AppShell responsive behavior", () => {
  const originalMatchMedia = window.matchMedia;

  beforeEach(() => {
    vi.clearAllMocks();
    window.location.hash = "#market";
    globalThis.EventSource = MockEventSource as any;
  });

  afterEach(() => {
    window.matchMedia = originalMatchMedia;
    vi.restoreAllMocks();
  });

  describe("at mobile breakpoint", () => {
    beforeEach(() => setupMatchMedia(true, false));

    it("renders the hamburger button", () => {
      const { container } = renderShell();
      const hamburger = container.querySelector(".hamburger-btn");
      expect(hamburger).toBeInTheDocument();
      expect(hamburger).toHaveClass("mobile-only");
    });

    it("hamburger has correct aria attributes when drawer is closed", () => {
      renderShell();
      const hamburger = screen.getByLabelText("Open navigation menu");
      expect(hamburger).toHaveAttribute("aria-expanded", "false");
      expect(hamburger).toHaveAttribute("aria-controls", "nav-drawer");
    });

    it("opens the navigation drawer when hamburger is clicked", async () => {
      renderShell();
      const hamburger = screen.getByLabelText("Open navigation menu");

      fireEvent.click(hamburger);

      await waitFor(() => {
        const drawer = screen.getByRole("dialog");
        expect(drawer).toBeInTheDocument();
        expect(drawer).toHaveAttribute("aria-label", "Navigation menu");
      });
      expect(hamburger).toHaveAttribute("aria-expanded", "true");
    });

    it("closes the drawer when backdrop is clicked", async () => {
      const { container } = renderShell();
      fireEvent.click(screen.getByLabelText("Open navigation menu"));

      await waitFor(() => {
        expect(screen.getByRole("dialog")).toBeInTheDocument();
      });

      const backdrop = container.querySelector(".nav-drawer-backdrop");
      fireEvent.click(backdrop!);

      await waitFor(() => {
        expect(screen.getByLabelText("Open navigation menu")).toHaveAttribute("aria-expanded", "false");
      });
    });

    it("nav pills have tablet-up class (hidden on mobile via CSS)", () => {
      const { container } = renderShell();
      const topNav = container.querySelector(".top-nav");
      expect(topNav).toHaveClass("tablet-up");
    });

    it("top-links have tablet-up class (hidden on mobile via CSS)", () => {
      const { container } = renderShell();
      const topLinks = container.querySelector(".top-links");
      expect(topLinks).toHaveClass("tablet-up");
    });
  });

  describe("at desktop breakpoint", () => {
    beforeEach(() => setupMatchMedia(false, false));

    it("renders the hamburger button with mobile-only class (hidden via CSS)", () => {
      const { container } = renderShell();
      const hamburger = container.querySelector(".hamburger-btn");
      expect(hamburger).toBeInTheDocument();
      expect(hamburger).toHaveClass("mobile-only");
    });

    it("renders nav pills without mobile-only class", () => {
      const { container } = renderShell();
      const topNav = container.querySelector(".top-nav");
      expect(topNav).toBeInTheDocument();
      expect(topNav).toHaveClass("tablet-up");
      expect(topNav).not.toHaveClass("mobile-only");
    });

    it("renders Market and Operations nav items in both nav and drawer", () => {
      renderShell();
      expect(screen.getAllByText("Market").length).toBeGreaterThanOrEqual(1);
      expect(screen.getAllByText("Operations").length).toBeGreaterThanOrEqual(1);
    });

    it("renders top-links (GitHub, Settings) visible", () => {
      const { container } = renderShell();
      const topLinks = container.querySelector(".top-links");
      expect(topLinks).toBeInTheDocument();
      expect(topLinks).toHaveClass("tablet-up");
    });
  });

  describe("at tablet breakpoint", () => {
    beforeEach(() => setupMatchMedia(false, true));

    it("renders nav pills (tablet-up is visible at tablet)", () => {
      const { container } = renderShell();
      const topNav = container.querySelector(".top-nav");
      expect(topNav).toBeInTheDocument();
      expect(topNav).toHaveClass("tablet-up");
    });

    it("hamburger button has mobile-only class (hidden at tablet via CSS)", () => {
      const { container } = renderShell();
      const hamburger = container.querySelector(".hamburger-btn");
      expect(hamburger).toHaveClass("mobile-only");
    });
  });
});
