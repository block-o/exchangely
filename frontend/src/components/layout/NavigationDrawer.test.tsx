import { render, screen, fireEvent } from "@testing-library/react";
import "@testing-library/jest-dom";
import { describe, expect, it, vi } from "vitest";
import { NavigationDrawer } from "./NavigationDrawer";

const defaultNavItems = [
  { id: "market", label: "Market" },
  { id: "system", label: "Operations" },
];

const defaultProps = {
  isOpen: true,
  onClose: vi.fn(),
  activeHash: "#market",
  onNavigate: vi.fn(),
  navItems: defaultNavItems,
};

function renderDrawer(overrides: Partial<typeof defaultProps> = {}) {
  const props = { ...defaultProps, ...overrides };
  props.onClose.mockClear();
  props.onNavigate.mockClear();
  return render(<NavigationDrawer {...props} />);
}

describe("NavigationDrawer", () => {
  it("renders all section links", () => {
    renderDrawer();

    expect(screen.getByText("Market")).toBeInTheDocument();
    expect(screen.getByText("Operations")).toBeInTheDocument();
  });

  it("calls onNavigate with correct hash and onClose when a section link is clicked", () => {
    const onNavigate = vi.fn();
    const onClose = vi.fn();
    renderDrawer({ onNavigate, onClose });

    fireEvent.click(screen.getByText("Market"));

    expect(onNavigate).toHaveBeenCalledWith("#market");
    expect(onClose).toHaveBeenCalled();
  });

  it("calls onNavigate and onClose for the Operations link", () => {
    const onNavigate = vi.fn();
    const onClose = vi.fn();
    renderDrawer({ onNavigate, onClose });

    fireEvent.click(screen.getByText("Operations"));

    expect(onNavigate).toHaveBeenCalledWith("#system");
    expect(onClose).toHaveBeenCalled();
  });

  it("calls onClose when Escape key is pressed", () => {
    const onClose = vi.fn();
    renderDrawer({ onClose });

    fireEvent.keyDown(document, { key: "Escape" });

    expect(onClose).toHaveBeenCalled();
  });

  it("has aria-modal and role=dialog attributes", () => {
    renderDrawer();

    const dialog = screen.getByRole("dialog");
    expect(dialog).toHaveAttribute("aria-modal", "true");
  });

  it("has an accessible label on the drawer", () => {
    renderDrawer();

    const dialog = screen.getByRole("dialog");
    expect(dialog).toHaveAttribute("aria-label", "Navigation menu");
  });
});
