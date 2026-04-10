import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import "@testing-library/jest-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { SettingsProvider } from "../../app/settings";
import { AppShell } from "./AppShell";

let mockAuthValue = {
  user: null as any,
  isAuthenticated: false,
  isLoading: false,
  login: vi.fn(),
  logout: vi.fn().mockResolvedValue(undefined),
  refreshToken: vi.fn(),
};

vi.mock("../../app/auth", () => ({
  useAuth: () => mockAuthValue,
}));

class MockEventSource {
  close = vi.fn();
  onopen: ((event: Event) => void) | null = null;
  onmessage: ((event: MessageEvent<string>) => void) | null = null;
  onerror: ((event: Event) => void) | null = null;
  constructor(_url: string) {}
}

function DummyAlpha() {
  return <div>Alpha Page</div>;
}
function DummyBeta() {
  return <div>Beta Page</div>;
}

function renderShell() {
  return render(
    <SettingsProvider>
      <AppShell>
        <DummyAlpha />
        <DummyBeta />
      </AppShell>
    </SettingsProvider>
  );
}

describe("AppShell", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    window.location.hash = "";
    globalThis.EventSource = MockEventSource as any;
    mockAuthValue = {
      user: null,
      isAuthenticated: false,
      isLoading: false,
      login: vi.fn(),
      logout: vi.fn().mockResolvedValue(undefined),
      refreshToken: vi.fn(),
    };
  });

  it("renders pages by section order instead of component names", async () => {
    window.location.hash = "#system";
    renderShell();
    await waitFor(() => {
      expect(screen.getByText("Beta Page")).toBeInTheDocument();
    });
  });

  it("renders API docs icon link and github icon link in the hero", async () => {
    renderShell();
    await waitFor(() => {
      expect(screen.getByText("Alpha Page")).toBeInTheDocument();
    });

    const githubLinks = screen.getAllByRole("link", { name: "GitHub project" });
    expect(githubLinks.length).toBeGreaterThanOrEqual(1);
    expect(githubLinks[0]).toHaveAttribute("href", "https://github.com/block-o/exchangely");

    const apiDocsLinks = screen.getAllByRole("link", { name: "API documentation" });
    expect(apiDocsLinks.length).toBeGreaterThanOrEqual(1);
    expect(apiDocsLinks[0]).toHaveAttribute("href", "http://localhost:8080/swagger");
  });

  describe("identity pill (unauthenticated)", () => {
    it("shows 'Sign in' label in the identity pill", () => {
      renderShell();
      const signInLink = screen.getAllByRole("link", { name: "Sign in" })[0];
      expect(signInLink).toBeInTheDocument();
      expect(signInLink).toHaveAttribute("href", "#login");
    });

    it("does not show Login or Settings in the top nav bar", () => {
      const { container } = renderShell();
      const topNav = container.querySelector(".top-nav");
      expect(topNav).toBeInTheDocument();
      const navLinks = topNav!.querySelectorAll(".nav-item");
      const labels = Array.from(navLinks).map((el) => el.textContent);
      expect(labels).not.toContain("Login");
      expect(labels).not.toContain("Settings");
      expect(labels).toContain("Market");
    });

    it("shows theme and currency toggles when gear icon is clicked", async () => {
      renderShell();
      const gearBtn = screen.getAllByLabelText("Settings")[0];
      fireEvent.click(gearBtn);

      await waitFor(() => {
        expect(screen.getByText("Theme")).toBeInTheDocument();
        expect(screen.getByText("Currency")).toBeInTheDocument();
      });
    });

    it("closes the dropdown when clicking outside", async () => {
      renderShell();
      const gearBtn = screen.getAllByLabelText("Settings")[0];
      fireEvent.click(gearBtn);

      await waitFor(() => {
        expect(screen.getByText("Theme")).toBeInTheDocument();
      });

      fireEvent.mouseDown(document.body);

      await waitFor(() => {
        expect(screen.queryByText("Theme")).not.toBeInTheDocument();
      });
    });
  });

  describe("identity pill (authenticated)", () => {
    beforeEach(() => {
      mockAuthValue = {
        user: {
          id: "u-1",
          email: "test@example.com",
          name: "Test User",
          avatar_url: "",
          role: "user",
          must_change_password: false,
        },
        isAuthenticated: true,
        isLoading: false,
        login: vi.fn(),
        logout: vi.fn().mockResolvedValue(undefined),
        refreshToken: vi.fn(),
      };
    });

    it("shows the username in the identity pill", () => {
      renderShell();
      const profileLink = screen.getAllByRole("link", { name: "Profile" })[0];
      expect(profileLink).toBeInTheDocument();
      expect(profileLink).toHaveTextContent("Test User");
      expect(profileLink).toHaveAttribute("href", "#settings");
    });

    it("shows Profile and Logout in the gear dropdown", async () => {
      renderShell();
      const gearBtn = screen.getAllByLabelText("Settings")[0];
      fireEvent.click(gearBtn);

      await waitFor(() => {
        expect(screen.getByText("Theme")).toBeInTheDocument();
      });

      const menuItems = screen.getAllByRole("menuitem");
      const labels = menuItems.map((el) => el.textContent);
      expect(labels).toContain("Profile");
      expect(labels).toContain("Logout");
    });

    it("does not show Settings in the top nav bar", () => {
      const { container } = renderShell();
      const topNav = container.querySelector(".top-nav");
      const navLinks = topNav!.querySelectorAll(".nav-item");
      const labels = Array.from(navLinks).map((el) => el.textContent);
      expect(labels).not.toContain("Settings");
      expect(labels).not.toContain("Login");
    });
  });

  describe("admin user navigation", () => {
    beforeEach(() => {
      mockAuthValue = {
        user: {
          id: "u-2",
          email: "admin@example.com",
          name: "Admin",
          avatar_url: "https://example.com/avatar.png",
          role: "admin",
          must_change_password: false,
        },
        isAuthenticated: true,
        isLoading: false,
        login: vi.fn(),
        logout: vi.fn().mockResolvedValue(undefined),
        refreshToken: vi.fn(),
      };
    });

    it("shows Operations in the top nav for admin users", () => {
      const { container } = renderShell();
      const topNav = container.querySelector(".top-nav");
      const navLinks = topNav!.querySelectorAll(".nav-item");
      const labels = Array.from(navLinks).map((el) => el.textContent);
      expect(labels).toContain("Operations");
    });

    it("does not show Operations for regular users", () => {
      mockAuthValue.user = { ...mockAuthValue.user, role: "user" };
      const { container } = renderShell();
      const topNav = container.querySelector(".top-nav");
      const navLinks = topNav!.querySelectorAll(".nav-item");
      const labels = Array.from(navLinks).map((el) => el.textContent);
      expect(labels).not.toContain("Operations");
    });
  });
});
