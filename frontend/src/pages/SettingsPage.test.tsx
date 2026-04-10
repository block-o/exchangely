import { render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { SettingsPage } from "./SettingsPage";
import type { User } from "../types/auth";

// --- Mock useAuth ---
const defaultUser: User = {
  id: "u-1",
  email: "alice@example.com",
  name: "Alice Smith",
  avatar_url: "https://example.com/avatar.jpg",
  role: "user",
  must_change_password: false,
};

let mockAuthValue = {
  user: defaultUser as User | null,
  isAuthenticated: true,
  isLoading: false,
  login: vi.fn(),
  logout: vi.fn(),
  refreshToken: vi.fn(),
};

vi.mock("../app/auth", () => ({
  useAuth: () => mockAuthValue,
}));

describe("SettingsPage", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockAuthValue = {
      user: defaultUser,
      isAuthenticated: true,
      isLoading: false,
      login: vi.fn(),
      logout: vi.fn(),
      refreshToken: vi.fn(),
    };
  });

  /**
   * Validates: Requirements 8.1
   * Displays user profile information.
   */
  it("displays user name and email", () => {
    render(<SettingsPage />);

    expect(screen.getByText("Alice Smith")).toBeInTheDocument();
    // Email appears in both profile and connected accounts sections
    const emails = screen.getAllByText("alice@example.com");
    expect(emails.length).toBeGreaterThanOrEqual(1);
  });

  /**
   * Validates: Requirements 8.1
   * Displays user avatar image.
   */
  it("displays user avatar", () => {
    render(<SettingsPage />);

    const avatar = screen.getByAltText("Alice Smith avatar");
    expect(avatar).toBeInTheDocument();
    expect(avatar).toHaveAttribute("src", "https://example.com/avatar.jpg");
  });

  it("displays fallback initial when no avatar_url", () => {
    mockAuthValue.user = { ...defaultUser, avatar_url: "" };
    render(<SettingsPage />);

    expect(screen.getByText("A")).toBeInTheDocument();
  });

  /**
   * Validates: Requirements 8.2
   * Displays role badge.
   */
  it("displays user role badge", () => {
    render(<SettingsPage />);
    expect(screen.getByText("user")).toBeInTheDocument();
  });

  it("displays admin role badge for admin users", () => {
    mockAuthValue.user = { ...defaultUser, role: "admin" };
    render(<SettingsPage />);
    expect(screen.getByText("admin")).toBeInTheDocument();
  });

  /**
   * Validates: Requirements 8.3
   * Connected accounts section shows linked Google email.
   */
  it("displays connected accounts section with Google email", () => {
    render(<SettingsPage />);

    expect(screen.getByText("Connected Accounts")).toBeInTheDocument();
    expect(screen.getByText("Google")).toBeInTheDocument();
    // The email appears in both profile and connected accounts
    const emailElements = screen.getAllByText("alice@example.com");
    expect(emailElements.length).toBeGreaterThanOrEqual(2);
  });

  /**
   * Validates: Requirements 8.4
   * Preferences section is present.
   */
  it("displays preferences section", () => {
    render(<SettingsPage />);

    expect(screen.getByText("Preferences")).toBeInTheDocument();
    expect(screen.getByText("Default Quote Currency")).toBeInTheDocument();
  });

  /**
   * Validates: Requirements 8.5
   * Redirects unauthenticated visitors to login.
   */
  it("redirects to login when not authenticated", () => {
    mockAuthValue.user = null;
    mockAuthValue.isAuthenticated = false;

    render(<SettingsPage />);

    expect(window.location.hash).toBe("#login");
  });

  /**
   * Validates: Requirements 8.1
   * Shows loading state while auth is loading.
   */
  it("shows loading state while auth is loading", () => {
    mockAuthValue.isLoading = true;

    render(<SettingsPage />);

    expect(screen.getByText("Loading…")).toBeInTheDocument();
  });
});
