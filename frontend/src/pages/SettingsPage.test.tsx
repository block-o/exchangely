import { render, screen, act } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { SettingsPage } from "./SettingsPage";
import { SettingsProvider } from "../app/settings";
import type { User } from "../types/auth";

// --- Mock useAuth ---
const defaultUser: User = {
  id: "u-1",
  email: "alice@example.com",
  name: "Alice Smith",
  avatar_url: "https://example.com/avatar.jpg",
  role: "user",
  has_google: true,
  has_password: false,
  must_change_password: false,
};

let mockAuthValue = {
  user: defaultUser as User | null,
  isAuthenticated: true,
  isLoading: false,
  authEnabled: true,
  authMethods: null,
  login: vi.fn(),
  logout: vi.fn(),
  refreshToken: vi.fn(),
};

vi.mock("../app/auth", () => ({
  useAuth: () => mockAuthValue,
}));

function renderPage() {
  return render(
    <SettingsProvider>
      <SettingsPage />
    </SettingsProvider>,
  );
}

describe("SettingsPage", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    localStorage.clear();
    document.documentElement.removeAttribute("data-theme");
    mockAuthValue = {
      user: { ...defaultUser },
      isAuthenticated: true,
      isLoading: false,
      authEnabled: true,
      authMethods: null,
      login: vi.fn(),
      logout: vi.fn(),
      refreshToken: vi.fn(),
    };
  });

  it("displays user name and email", () => {
    renderPage();
    expect(screen.getByText("Alice Smith")).toBeInTheDocument();
    const emails = screen.getAllByText("alice@example.com");
    expect(emails.length).toBeGreaterThanOrEqual(1);
  });

  it("displays user avatar", () => {
    renderPage();
    const avatar = screen.getByAltText("Alice Smith avatar");
    expect(avatar).toBeInTheDocument();
    expect(avatar).toHaveAttribute("src", "https://example.com/avatar.jpg");
  });

  it("displays fallback initial when no avatar_url", () => {
    mockAuthValue.user = { ...defaultUser, avatar_url: "" };
    renderPage();
    expect(screen.getByText("A")).toBeInTheDocument();
  });

  it("displays user role badge", () => {
    renderPage();
    expect(screen.getByText("user")).toBeInTheDocument();
  });

  it("displays admin role badge for admin users", () => {
    mockAuthValue.user = { ...defaultUser, role: "admin" };
    renderPage();
    expect(screen.getByText("admin")).toBeInTheDocument();
  });

  it("shows Google when has_google is true", () => {
    mockAuthValue.user = { ...defaultUser, has_google: true, has_password: false };
    renderPage();
    expect(screen.getByText("Google")).toBeInTheDocument();
    expect(screen.queryByText("Password")).not.toBeInTheDocument();
  });

  it("shows Password when has_password is true", () => {
    mockAuthValue.user = { ...defaultUser, has_google: false, has_password: true };
    renderPage();
    expect(screen.getByText("Password")).toBeInTheDocument();
    expect(screen.queryByText("Google")).not.toBeInTheDocument();
  });

  it("shows both Google and Password when both are true", () => {
    mockAuthValue.user = { ...defaultUser, has_google: true, has_password: true };
    renderPage();
    expect(screen.getByText("Google")).toBeInTheDocument();
    expect(screen.getByText("Password")).toBeInTheDocument();
  });

  it("shows 'No connected accounts' when neither is set", () => {
    mockAuthValue.user = { ...defaultUser, has_google: false, has_password: false };
    renderPage();
    expect(screen.getByText("No connected accounts")).toBeInTheDocument();
  });

  it("redirects to login when not authenticated", () => {
    mockAuthValue.user = null;
    mockAuthValue.isAuthenticated = false;
    renderPage();
    expect(window.location.hash).toBe("#login");
  });

  it("shows loading state while auth is loading", () => {
    mockAuthValue.isLoading = true;
    renderPage();
    expect(screen.getByText("Loading…")).toBeInTheDocument();
  });

  it("displays preferences section with theme and currency controls", () => {
    renderPage();
    expect(screen.getByText("Preferences")).toBeInTheDocument();
    expect(screen.getByText("Theme")).toBeInTheDocument();
    expect(screen.getByText("Default Quote Currency")).toBeInTheDocument();
  });

  it("reflects the default dark theme as active", () => {
    renderPage();
    const darkBtn = screen.getByRole("tab", { name: "Dark" });
    const lightBtn = screen.getByRole("tab", { name: "Light" });
    expect(darkBtn.className).toContain("active");
    expect(lightBtn.className).not.toContain("active");
  });

  it("switches to light theme and persists to localStorage", () => {
    renderPage();
    const lightBtn = screen.getByRole("tab", { name: "Light" });

    act(() => { lightBtn.click(); });

    expect(lightBtn.className).toContain("active");
    expect(screen.getByRole("tab", { name: "Dark" }).className).not.toContain("active");
    expect(localStorage.getItem("exchangely_theme")).toBe("light");
    expect(document.documentElement.getAttribute("data-theme")).toBe("light");
  });

  it("loads persisted theme from localStorage", () => {
    localStorage.setItem("exchangely_theme", "light");
    renderPage();
    expect(screen.getByRole("tab", { name: "Light" }).className).toContain("active");
    expect(screen.getByRole("tab", { name: "Dark" }).className).not.toContain("active");
  });

  it("reflects the default EUR currency as active", () => {
    renderPage();
    const eurBtn = screen.getByRole("tab", { name: "EUR" });
    const usdBtn = screen.getByRole("tab", { name: "USD" });
    expect(eurBtn.className).toContain("active");
    expect(usdBtn.className).not.toContain("active");
  });

  it("switches to USD and persists to localStorage", () => {
    renderPage();
    const usdBtn = screen.getByRole("tab", { name: "USD" });

    act(() => { usdBtn.click(); });

    expect(usdBtn.className).toContain("active");
    expect(screen.getByRole("tab", { name: "EUR" }).className).not.toContain("active");
    expect(localStorage.getItem("exchangely_quote_currency")).toBe("USD");
  });

  it("loads persisted currency from localStorage", () => {
    localStorage.setItem("exchangely_quote_currency", "USD");
    renderPage();
    expect(screen.getByRole("tab", { name: "USD" }).className).toContain("active");
    expect(screen.getByRole("tab", { name: "EUR" }).className).not.toContain("active");
  });
});
