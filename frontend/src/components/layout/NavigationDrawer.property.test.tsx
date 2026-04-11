/**
 * For any section link rendered in the NavigationDrawer, clicking that link
 * should call onNavigate with the corresponding section hash and call onClose.
 */
import { render, fireEvent, cleanup } from "@testing-library/react";
import "@testing-library/jest-dom";
import { describe, expect, it, vi } from "vitest";
import fc from "fast-check";
import { NavigationDrawer } from "./NavigationDrawer";

/**
 * Arbitrary that produces a valid section config with unique id and label.
 */
const sectionArb = fc.record({
  id: fc.stringMatching(/^[a-z][a-z0-9]{0,9}$/),
  label: fc.stringMatching(/^[A-Z][a-z]{1,9}$/),
});

const sectionsArb = fc
  .array(sectionArb, { minLength: 1, maxLength: 8 })
  .filter((arr) => {
    const ids = arr.map((s) => s.id);
    const labels = arr.map((s) => s.label);
    return new Set(ids).size === ids.length && new Set(labels).size === labels.length;
  });

describe("Feature: responsive-ui-overhaul, Property 1: Navigation drawer link click navigates and closes", () => {
  it("clicking any section link calls onNavigate with the correct hash and calls onClose", () => {
    fc.assert(
      fc.property(sectionsArb, (sections) => {
        // Clean up any previous render
        cleanup();

        const onNavigate = vi.fn();
        const onClose = vi.fn();

        const { unmount, container } = render(
          <NavigationDrawer
            isOpen={true}
            onClose={onClose}
            activeHash=""
            onNavigate={onNavigate}
            navItems={sections}
            isAuthenticated={false}
            user={null}
            onLogout={() => {}}
          />,
        );

        for (const section of sections) {
          onNavigate.mockClear();
          onClose.mockClear();

          // Query by href to avoid text collisions with "API Docs" or other static text
          const link = container.querySelector(
            `a[href="#${section.id}"]`,
          ) as HTMLElement;
          expect(link).not.toBeNull();

          fireEvent.click(link);

          // onNavigate must be called with the hash for this section
          expect(onNavigate).toHaveBeenCalledTimes(1);
          expect(onNavigate).toHaveBeenCalledWith(`#${section.id}`);

          // onClose must also be called (drawer closes after navigation)
          expect(onClose).toHaveBeenCalledTimes(1);
        }

        unmount();
      }),
      { numRuns: 100 },
    );
  });
});
