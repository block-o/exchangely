import { renderHook, act } from "@testing-library/react";
import { beforeEach, afterEach, describe, expect, it, vi } from "vitest";
import { useBreakpoint } from "./useBreakpoint";

type ChangeListener = (ev: MediaQueryListEvent) => void;

/**
 * Creates a mock MediaQueryList that tracks addEventListener/removeEventListener
 * and allows triggering change events programmatically.
 */
function createMockMediaQueryList(matches: boolean) {
  const listeners: ChangeListener[] = [];
  return {
    matches,
    media: "",
    onchange: null as ((ev: MediaQueryListEvent) => void) | null,
    addEventListener: vi.fn((event: string, cb: ChangeListener) => {
      if (event === "change") listeners.push(cb);
    }),
    removeEventListener: vi.fn((event: string, cb: ChangeListener) => {
      if (event === "change") {
        const idx = listeners.indexOf(cb);
        if (idx >= 0) listeners.splice(idx, 1);
      }
    }),
    addListener: vi.fn(),
    removeListener: vi.fn(),
    dispatchEvent: vi.fn(),
    /** Fire a synthetic change event to all registered listeners. */
    _fireChange(newMatches: boolean) {
      this.matches = newMatches;
      for (const cb of listeners) {
        cb({ matches: newMatches } as MediaQueryListEvent);
      }
    },
    _listenerCount() {
      return listeners.length;
    },
  };
}

describe("useBreakpoint", () => {
  let mobileMql: ReturnType<typeof createMockMediaQueryList>;
  let tabletMql: ReturnType<typeof createMockMediaQueryList>;
  const originalMatchMedia = window.matchMedia;

  function setupMatchMedia(mobileMatches: boolean, tabletMatches: boolean) {
    mobileMql = createMockMediaQueryList(mobileMatches);
    tabletMql = createMockMediaQueryList(tabletMatches);

    window.matchMedia = vi.fn((query: string) => {
      if (query === "(max-width: 639px)") return mobileMql as unknown as MediaQueryList;
      if (query === "(min-width: 640px) and (max-width: 1023px)") return tabletMql as unknown as MediaQueryList;
      return createMockMediaQueryList(false) as unknown as MediaQueryList;
    });
  }

  afterEach(() => {
    window.matchMedia = originalMatchMedia;
    vi.restoreAllMocks();
  });

  // --- Requirement 1.1: Three breakpoint tiers ---

  it("returns 'mobile' when viewport matches mobile query (≤639px)", () => {
    setupMatchMedia(true, false);
    const { result } = renderHook(() => useBreakpoint());
    expect(result.current).toBe("mobile");
  });

  it("returns 'tablet' when viewport matches tablet query (640–1023px)", () => {
    setupMatchMedia(false, true);
    const { result } = renderHook(() => useBreakpoint());
    expect(result.current).toBe("tablet");
  });

  it("returns 'desktop' when neither mobile nor tablet matches (≥1024px)", () => {
    setupMatchMedia(false, false);
    const { result } = renderHook(() => useBreakpoint());
    expect(result.current).toBe("desktop");
  });

  // --- Requirement 1.2: Responds to viewport changes ---

  it("updates to 'mobile' when the mobile media query starts matching", () => {
    setupMatchMedia(false, false);
    const { result } = renderHook(() => useBreakpoint());
    expect(result.current).toBe("desktop");

    act(() => {
      mobileMql._fireChange(true);
      tabletMql._fireChange(false);
    });

    expect(result.current).toBe("mobile");
  });

  it("updates to 'tablet' when the tablet media query starts matching", () => {
    setupMatchMedia(true, false);
    const { result } = renderHook(() => useBreakpoint());
    expect(result.current).toBe("mobile");

    act(() => {
      mobileMql._fireChange(false);
      tabletMql._fireChange(true);
    });

    expect(result.current).toBe("tablet");
  });

  it("updates to 'desktop' when both queries stop matching", () => {
    setupMatchMedia(true, false);
    const { result } = renderHook(() => useBreakpoint());
    expect(result.current).toBe("mobile");

    act(() => {
      mobileMql._fireChange(false);
      tabletMql._fireChange(false);
    });

    expect(result.current).toBe("desktop");
  });

  // --- Listener cleanup on unmount ---

  it("removes event listeners on unmount", () => {
    setupMatchMedia(false, false);
    const { unmount } = renderHook(() => useBreakpoint());

    expect(mobileMql.addEventListener).toHaveBeenCalledWith("change", expect.any(Function));
    expect(tabletMql.addEventListener).toHaveBeenCalledWith("change", expect.any(Function));

    unmount();

    expect(mobileMql.removeEventListener).toHaveBeenCalledWith("change", expect.any(Function));
    expect(tabletMql.removeEventListener).toHaveBeenCalledWith("change", expect.any(Function));
  });

  it("does not fire state updates after unmount", () => {
    setupMatchMedia(false, false);
    const { result, unmount } = renderHook(() => useBreakpoint());
    expect(result.current).toBe("desktop");

    unmount();

    // After unmount, listeners should be removed so _listenerCount is 0
    expect(mobileMql._listenerCount()).toBe(0);
    expect(tabletMql._listenerCount()).toBe(0);
  });

  // --- Fallback behavior ---

  it("falls back to 'desktop' when matchMedia is unavailable", () => {
    const saved = window.matchMedia;
    // @ts-expect-error - intentionally removing matchMedia
    delete window.matchMedia;

    const { result } = renderHook(() => useBreakpoint());
    expect(result.current).toBe("desktop");

    window.matchMedia = saved;
  });
});
