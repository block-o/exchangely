/**
 * Feature: responsive-ui-overhaul, Property 2: Hamburger aria-expanded matches drawer state
 *
 * Validates: Requirements 2.9
 *
 * For any sequence of open and close actions on the NavigationDrawer, the
 * hamburger button's aria-expanded attribute should always equal the current
 * boolean open state of the drawer.
 */
import { render, fireEvent, cleanup } from "@testing-library/react";
import "@testing-library/jest-dom";
import { describe, expect, it, vi } from "vitest";
import fc from "fast-check";
import { useState, useRef } from "react";
import { NavigationDrawer } from "./NavigationDrawer";

/**
 * Small wrapper component that wires a hamburger button with aria-expanded
 * to the NavigationDrawer, mirroring the design's AppShell integration.
 */
function HamburgerDrawerWrapper() {
  const [isDrawerOpen, setDrawerOpen] = useState(false);
  const hamburgerRef = useRef<HTMLButtonElement>(null);

  return (
    <>
      <button
        ref={hamburgerRef}
        data-testid="hamburger"
        aria-expanded={isDrawerOpen}
        aria-controls="nav-drawer"
        aria-label="Open navigation menu"
        onClick={() => setDrawerOpen(true)}
      >
        ☰
      </button>
      <NavigationDrawer
        isOpen={isDrawerOpen}
        onClose={() => setDrawerOpen(false)}
        activeHash="#market"
        onNavigate={() => {}}
        hamburgerRef={hamburgerRef}
        navItems={[{ id: "market", label: "Market" }]}
        isAuthenticated={false}
        user={null}
        onLogout={() => {}}
      />
    </>
  );
}

/**
 * Arbitrary that generates a non-empty sequence of toggle actions.
 * true = open (click hamburger), false = close (click backdrop).
 */
const toggleSeqArb = fc.array(fc.boolean(), { minLength: 1, maxLength: 20 });

describe("Feature: responsive-ui-overhaul, Property 2: Hamburger aria-expanded matches drawer state", () => {
  it("aria-expanded on the hamburger button always matches the drawer open state after each toggle", () => {
    fc.assert(
      fc.property(toggleSeqArb, (toggles) => {
        cleanup();

        const { getByTestId, container, unmount } = render(
          <HamburgerDrawerWrapper />,
        );

        let drawerOpen = false;

        for (const wantOpen of toggles) {
          if (wantOpen && !drawerOpen) {
            // Open: click the hamburger button
            fireEvent.click(getByTestId("hamburger"));
            drawerOpen = true;
          } else if (!wantOpen && drawerOpen) {
            // Close: click the backdrop overlay
            const backdrop = container.querySelector(".nav-drawer-backdrop");
            expect(backdrop).not.toBeNull();
            fireEvent.click(backdrop!);
            drawerOpen = false;
          }
          // If already in the desired state, no action needed — state stays the same.

          const hamburger = getByTestId("hamburger");
          expect(hamburger.getAttribute("aria-expanded")).toBe(
            String(drawerOpen),
          );
        }

        unmount();
      }),
      { numRuns: 100 },
    );
  });
});
