import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { SystemPanel } from "./SystemPanel";
import * as client from "../api/client";
import * as auth from "../app/auth";

// Mock the API client
vi.mock("../api/client", () => ({
  API_BASE_URL: "http://localhost:8080/api/v1",
  authFetch: vi.fn(),
  authEventSource: vi.fn(() => ({
    onmessage: null,
    close: vi.fn(),
  })),
}));

// Mock the auth context
vi.mock("../app/auth", () => ({
  useAuth: vi.fn(),
}));

describe("SystemPanel", () => {
  const mockAuthFetch = vi.mocked(client.authFetch);
  const mockUseAuth = vi.mocked(auth.useAuth);

  const mockAdminUser = {
    id: "admin-user-id",
    email: "admin@example.com",
    name: "Admin User",
    avatar_url: "",
    role: "admin" as const,
    has_google: true,
    has_password: false,
    must_change_password: false,
  };

  beforeEach(() => {
    mockUseAuth.mockReturnValue({
      user: mockAdminUser,
      isAuthenticated: true,
      isLoading: false,
      authEnabled: true,
      authMethods: { google: true, local: true },
      login: vi.fn(),
      logout: vi.fn(),
      refreshToken: vi.fn(),
    });

    // Mock API responses for different tabs
    mockAuthFetch.mockImplementation((url) => {
      if (typeof url === "string") {
        if (url.includes("/system/users")) {
          return Promise.resolve({
            ok: true,
            json: async () => ({
              data: [],
              total: 0,
              page: 1,
              limit: 50,
            }),
          } as Response);
        }
        if (url.includes("/system/sync-status")) {
          return Promise.resolve({
            ok: true,
            json: async () => ({
              pairs: [],
              warnings: [],
            }),
          } as Response);
        }
        if (url.includes("/system/tasks")) {
          return Promise.resolve({
            ok: true,
            json: async () => ({
              upcoming: [],
              upcomingTotal: 0,
              recent: [],
              recentTotal: 0,
            }),
          } as Response);
        }
      }
      return Promise.resolve({
        ok: true,
        json: async () => ({}),
      } as Response);
    });
  });

  it("renders all four tabs including Users", () => {
    render(<SystemPanel />);

    expect(screen.getByRole("tab", { name: "Overview" })).toBeInTheDocument();
    expect(screen.getByRole("tab", { name: "Coverage" })).toBeInTheDocument();
    expect(screen.getByRole("tab", { name: "Audit" })).toBeInTheDocument();
    expect(screen.getByRole("tab", { name: "Users" })).toBeInTheDocument();
  });

  it("displays Users tab for admin users", () => {
    render(<SystemPanel />);

    const usersTab = screen.getByRole("tab", { name: "Users" });
    expect(usersTab).toBeInTheDocument();
    expect(usersTab).toBeVisible();
  });

  it("switches to Users tab when clicked", async () => {
    const user = userEvent.setup();
    render(<SystemPanel />);

    const usersTab = screen.getByRole("tab", { name: "Users" });
    await user.click(usersTab);

    await waitFor(() => {
      expect(usersTab).toHaveAttribute("aria-selected", "true");
    });

    // Check that Users tab content is rendered
    await waitFor(() => {
      expect(screen.getByPlaceholderText("Search by email or name…")).toBeInTheDocument();
    });
  });

  it("updates URL when Users tab is selected", async () => {
    const user = userEvent.setup();
    render(<SystemPanel />);

    const usersTab = screen.getByRole("tab", { name: "Users" });
    await user.click(usersTab);

    await waitFor(() => {
      expect(window.location.search).toContain("tab=Users");
    });
  });

  it("loads Users tab from URL parameter", () => {
    // Set URL parameter
    const url = new URL(window.location.href);
    url.searchParams.set("tab", "Users");
    window.history.replaceState({}, "", url.toString());

    render(<SystemPanel />);

    const usersTab = screen.getByRole("tab", { name: "Users" });
    expect(usersTab).toHaveAttribute("aria-selected", "true");
  });

  it("renders Users tab content when selected", async () => {
    const user = userEvent.setup();
    render(<SystemPanel />);

    const usersTab = screen.getByRole("tab", { name: "Users" });
    await user.click(usersTab);

    await waitFor(() => {
      // Check for Users tab specific elements
      expect(screen.getByLabelText("Search users")).toBeInTheDocument();
      expect(screen.getByLabelText("Filter by role")).toBeInTheDocument();
      expect(screen.getByLabelText("Filter by status")).toBeInTheDocument();
    });
  });
});
