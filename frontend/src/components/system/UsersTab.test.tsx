import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { UsersTab } from "./UsersTab";
import * as client from "../../api/client";
import * as auth from "../../app/auth";

// Mock the API client
vi.mock("../../api/client", () => ({
  API_BASE_URL: "http://localhost:8080/api/v1",
  authFetch: vi.fn(),
}));

// Mock the auth context
vi.mock("../../app/auth", () => ({
  useAuth: vi.fn(),
}));

describe("UsersTab", () => {
  const mockAuthFetch = vi.mocked(client.authFetch);
  const mockUseAuth = vi.mocked(auth.useAuth);

  const mockCurrentUser = {
    id: "admin-user-id",
    email: "admin@example.com",
    name: "Admin User",
    avatar_url: "",
    role: "admin" as const,
    has_google: true,
    has_password: false,
    must_change_password: false,
  };

  const mockUsers = [
    {
      id: "user-1",
      email: "user1@example.com",
      name: "User One",
      avatar_url: "",
      role: "user" as const,
      has_google: false,
      has_password: true,
      disabled: false,
      must_change_password: false,
      created_at: "2024-01-01T00:00:00Z",
      updated_at: "2024-01-01T00:00:00Z",
    },
    {
      id: "user-2",
      email: "user2@example.com",
      name: "User Two",
      avatar_url: "",
      role: "premium" as const,
      has_google: true,
      has_password: false,
      disabled: false,
      must_change_password: false,
      created_at: "2024-01-02T00:00:00Z",
      updated_at: "2024-01-02T00:00:00Z",
    },
    {
      id: "admin-user-id",
      email: "admin@example.com",
      name: "Admin User",
      avatar_url: "",
      role: "admin" as const,
      has_google: true,
      has_password: false,
      disabled: false,
      must_change_password: false,
      created_at: "2024-01-03T00:00:00Z",
      updated_at: "2024-01-03T00:00:00Z",
    },
  ];

  beforeEach(() => {
    mockUseAuth.mockReturnValue({
      user: mockCurrentUser,
      isAuthenticated: true,
      isLoading: false,
      authEnabled: true,
      authMethods: { google: true, local: true },
      login: vi.fn(),
      logout: vi.fn(),
      refreshToken: vi.fn(),
    });

    mockAuthFetch.mockResolvedValue({
      ok: true,
      json: async () => ({
        data: mockUsers,
        total: 3,
        page: 1,
        limit: 50,
      }),
    } as Response);
  });

  afterEach(() => {
    vi.clearAllMocks();
  });

  it("renders users table with mock data", async () => {
    render(<UsersTab />);

    await waitFor(() => {
      expect(screen.getByText("user1@example.com")).toBeInTheDocument();
    });

    expect(screen.getByText("User One")).toBeInTheDocument();
    expect(screen.getByText("user2@example.com")).toBeInTheDocument();
    expect(screen.getByText("User Two")).toBeInTheDocument();
    expect(screen.getByText("admin@example.com")).toBeInTheDocument();
  });

  it("displays role badges correctly", async () => {
    render(<UsersTab />);

    await waitFor(() => {
      expect(screen.getByText("user1@example.com")).toBeInTheDocument();
    });

    const roleBadges = screen.getAllByText(/^(user|premium|admin)$/);
    expect(roleBadges).toHaveLength(3);
  });

  it("displays status badges correctly", async () => {
    render(<UsersTab />);

    await waitFor(() => {
      expect(screen.getByText("user1@example.com")).toBeInTheDocument();
    });

    const activeBadges = screen.getAllByText("Active");
    expect(activeBadges.length).toBeGreaterThan(0);
  });

  it("filters users by search term", async () => {
    const user = userEvent.setup();
    render(<UsersTab />);

    await waitFor(() => {
      expect(screen.getByText("user1@example.com")).toBeInTheDocument();
    });

    const searchInput = screen.getByPlaceholderText("Search by email or name…");
    await user.type(searchInput, "user1");

    // Wait for debounce
    await waitFor(
      () => {
        expect(mockAuthFetch).toHaveBeenCalledWith(
          expect.stringContaining("search=user1")
        );
      },
      { timeout: 500 }
    );
  });

  it("filters users by role", async () => {
    const user = userEvent.setup();
    render(<UsersTab />);

    await waitFor(() => {
      expect(screen.getByText("user1@example.com")).toBeInTheDocument();
    });

    const roleFilter = screen.getByLabelText("Filter by role");
    await user.selectOptions(roleFilter, "premium");

    await waitFor(() => {
      expect(mockAuthFetch).toHaveBeenCalledWith(
        expect.stringContaining("role=premium")
      );
    });
  });

  it("filters users by status", async () => {
    const user = userEvent.setup();
    render(<UsersTab />);

    await waitFor(() => {
      expect(screen.getByText("user1@example.com")).toBeInTheDocument();
    });

    const statusFilter = screen.getByLabelText("Filter by status");
    await user.selectOptions(statusFilter, "active");

    await waitFor(() => {
      expect(mockAuthFetch).toHaveBeenCalledWith(
        expect.stringContaining("status=active")
      );
    });
  });

  it("opens user detail view on row click", async () => {
    const user = userEvent.setup();
    render(<UsersTab />);

    await waitFor(() => {
      expect(screen.getByText("user1@example.com")).toBeInTheDocument();
    });

    const row = screen.getByText("User One").closest("tr");
    expect(row).toBeInTheDocument();
    await user.click(row!);

    await waitFor(() => {
      expect(screen.getByText("User Details")).toBeInTheDocument();
    });

    expect(screen.getByText("user-1…")).toBeInTheDocument();
  });

  it("displays role selector in detail view", async () => {
    const user = userEvent.setup();
    render(<UsersTab />);

    await waitFor(() => {
      expect(screen.getByText("user1@example.com")).toBeInTheDocument();
    });

    const row = screen.getByText("User One").closest("tr");
    await user.click(row!);

    await waitFor(() => {
      expect(screen.getByText("User Details")).toBeInTheDocument();
    });

    // Get all comboboxes and find the one in the modal (not the filter)
    const allSelects = screen.getAllByRole("combobox");
    const roleSelect = allSelects.find((select) => 
      select.closest('[style*="position: fixed"]') !== null
    );
    expect(roleSelect).toBeInTheDocument();
    expect(roleSelect).toHaveValue("user");
  });

  it("displays disable/enable toggle in detail view", async () => {
    const user = userEvent.setup();
    render(<UsersTab />);

    await waitFor(() => {
      expect(screen.getByText("user1@example.com")).toBeInTheDocument();
    });

    const row = screen.getByText("User One").closest("tr");
    await user.click(row!);

    await waitFor(() => {
      expect(screen.getByText("User Details")).toBeInTheDocument();
    });

    expect(screen.getByText("Disable Account")).toBeInTheDocument();
  });

  it("displays force password reset button for users with password", async () => {
    const user = userEvent.setup();
    render(<UsersTab />);

    await waitFor(() => {
      expect(screen.getByText("user1@example.com")).toBeInTheDocument();
    });

    const row = screen.getByText("User One").closest("tr");
    await user.click(row!);

    await waitFor(() => {
      expect(screen.getByText("User Details")).toBeInTheDocument();
    });

    expect(
      screen.getByText("Force Password Reset on Next Login")
    ).toBeInTheDocument();
  });

  it("does not display force password reset button for OAuth-only users", async () => {
    const user = userEvent.setup();
    render(<UsersTab />);

    await waitFor(() => {
      expect(screen.getByText("user2@example.com")).toBeInTheDocument();
    });

    const row = screen.getByText("User Two").closest("tr");
    await user.click(row!);

    await waitFor(() => {
      expect(screen.getByText("User Details")).toBeInTheDocument();
    });

    expect(
      screen.queryByText("Force Password Reset on Next Login")
    ).not.toBeInTheDocument();
  });

  it("disables role selector for current user", async () => {
    const user = userEvent.setup();
    render(<UsersTab />);

    await waitFor(() => {
      expect(screen.getByText("admin@example.com")).toBeInTheDocument();
    });

    const row = screen.getByText("Admin User").closest("tr");
    await user.click(row!);

    await waitFor(() => {
      expect(screen.getByText("User Details")).toBeInTheDocument();
    });

    // Get all comboboxes and find the one in the modal (not the filter)
    const allSelects = screen.getAllByRole("combobox");
    const roleSelect = allSelects.find((select) => 
      select.closest('[style*="position: fixed"]') !== null
    );
    expect(roleSelect).toBeDisabled();
    expect(
      screen.getByText("You cannot change your own role")
    ).toBeInTheDocument();
  });

  it("disables disable toggle for current user", async () => {
    const user = userEvent.setup();
    render(<UsersTab />);

    await waitFor(() => {
      expect(screen.getByText("admin@example.com")).toBeInTheDocument();
    });

    const row = screen.getByText("Admin User").closest("tr");
    await user.click(row!);

    await waitFor(() => {
      expect(screen.getByText("User Details")).toBeInTheDocument();
    });

    const disableButton = screen.getByText("Disable Account");
    expect(disableButton).toBeDisabled();
    expect(
      screen.getByText("You cannot disable your own account")
    ).toBeInTheDocument();
  });

  it("updates user role successfully", async () => {
    const user = userEvent.setup();
    const updatedUser = { ...mockUsers[0], role: "premium" as const };

    mockAuthFetch
      .mockResolvedValueOnce({
        ok: true,
        json: async () => ({
          data: mockUsers,
          total: 3,
          page: 1,
          limit: 50,
        }),
      } as Response)
      .mockResolvedValueOnce({
        ok: true,
        json: async () => updatedUser,
      } as Response)
      .mockResolvedValueOnce({
        ok: true,
        json: async () => ({
          data: [updatedUser, mockUsers[1], mockUsers[2]],
          total: 3,
          page: 1,
          limit: 50,
        }),
      } as Response);

    render(<UsersTab />);

    await waitFor(() => {
      expect(screen.getByText("user1@example.com")).toBeInTheDocument();
    });

    const row = screen.getByText("User One").closest("tr");
    await user.click(row!);

    await waitFor(() => {
      expect(screen.getByText("User Details")).toBeInTheDocument();
    });

    // Get all comboboxes and find the one in the modal (not the filter)
    const allSelects = screen.getAllByRole("combobox");
    const roleSelect = allSelects.find((select) => 
      select.closest('[style*="position: fixed"]') !== null
    );
    await user.selectOptions(roleSelect!, "premium");

    await waitFor(() => {
      const calls = mockAuthFetch.mock.calls;
      const roleUpdateCall = calls.find(call => 
        typeof call[0] === 'string' && call[0].includes("/system/users/user-1/role")
      );
      expect(roleUpdateCall).toBeDefined();
    });

    await waitFor(() => {
      expect(screen.getByText("Role updated to premium")).toBeInTheDocument();
    });
  });

  it("toggles user disabled status successfully", async () => {
    const user = userEvent.setup();
    const disabledUser = { ...mockUsers[0], disabled: true };

    mockAuthFetch
      .mockResolvedValueOnce({
        ok: true,
        json: async () => ({
          data: mockUsers,
          total: 3,
          page: 1,
          limit: 50,
        }),
      } as Response)
      .mockResolvedValueOnce({
        ok: true,
        json: async () => disabledUser,
      } as Response)
      .mockResolvedValueOnce({
        ok: true,
        json: async () => ({
          data: [disabledUser, mockUsers[1], mockUsers[2]],
          total: 3,
          page: 1,
          limit: 50,
        }),
      } as Response);

    render(<UsersTab />);

    await waitFor(() => {
      expect(screen.getByText("user1@example.com")).toBeInTheDocument();
    });

    const row = screen.getByText("User One").closest("tr");
    await user.click(row!);

    await waitFor(() => {
      expect(screen.getByText("User Details")).toBeInTheDocument();
    });

    const disableButton = screen.getByText("Disable Account");
    await user.click(disableButton);

    await waitFor(() => {
      expect(mockAuthFetch).toHaveBeenCalledWith(
        expect.stringContaining("/system/users/user-1/status"),
        expect.objectContaining({
          method: "PATCH",
          body: JSON.stringify({ disabled: true }),
        })
      );
    });

    await waitFor(() => {
      expect(screen.getByText("User disabled")).toBeInTheDocument();
    });
  });

  it("forces password reset successfully", async () => {
    const user = userEvent.setup();
    const updatedUser = { ...mockUsers[0], must_change_password: true };

    mockAuthFetch
      .mockResolvedValueOnce({
        ok: true,
        json: async () => ({
          data: mockUsers,
          total: 3,
          page: 1,
          limit: 50,
        }),
      } as Response)
      .mockResolvedValueOnce({
        ok: true,
        json: async () => updatedUser,
      } as Response)
      .mockResolvedValueOnce({
        ok: true,
        json: async () => ({
          data: [updatedUser, mockUsers[1], mockUsers[2]],
          total: 3,
          page: 1,
          limit: 50,
        }),
      } as Response);

    render(<UsersTab />);

    await waitFor(() => {
      expect(screen.getByText("user1@example.com")).toBeInTheDocument();
    });

    const row = screen.getByText("User One").closest("tr");
    await user.click(row!);

    await waitFor(() => {
      expect(screen.getByText("User Details")).toBeInTheDocument();
    });

    const resetButton = screen.getByText("Force Password Reset on Next Login");
    await user.click(resetButton);

    await waitFor(() => {
      expect(mockAuthFetch).toHaveBeenCalledWith(
        expect.stringContaining("/system/users/user-1/force-password-reset"),
        expect.objectContaining({
          method: "POST",
        })
      );
    });

    await waitFor(() => {
      expect(
        screen.getByText("Password reset required on next login")
      ).toBeInTheDocument();
    });
  });

  it("displays error message on failed action", async () => {
    const user = userEvent.setup();

    mockAuthFetch
      .mockResolvedValueOnce({
        ok: true,
        json: async () => ({
          data: mockUsers,
          total: 3,
          page: 1,
          limit: 50,
        }),
      } as Response)
      .mockResolvedValueOnce({
        ok: false,
        statusText: "Bad Request",
        json: async () => ({ error: "cannot change own role" }),
      } as Response);

    render(<UsersTab />);

    await waitFor(() => {
      expect(screen.getByText("user1@example.com")).toBeInTheDocument();
    });

    const row = screen.getByText("User One").closest("tr");
    await user.click(row!);

    await waitFor(() => {
      expect(screen.getByText("User Details")).toBeInTheDocument();
    });

    // Get all comboboxes and find the one in the modal (not the filter)
    const allSelects = screen.getAllByRole("combobox");
    const roleSelect = allSelects.find((select) => 
      select.closest('[style*="position: fixed"]') !== null
    );
    await user.selectOptions(roleSelect!, "admin");

    await waitFor(() => {
      expect(screen.getByText("cannot change own role")).toBeInTheDocument();
    }, { timeout: 2000 });
  });

  it("supports pagination", async () => {
    // First render with pagination
    mockAuthFetch.mockResolvedValueOnce({
      ok: true,
      json: async () => ({
        data: mockUsers,
        total: 100,
        page: 1,
        limit: 50,
      }),
    } as Response);

    render(<UsersTab />);

    await waitFor(() => {
      expect(screen.getByText("user1@example.com")).toBeInTheDocument();
    });

    // Should show pagination
    await waitFor(() => {
      expect(screen.getByText("Page 1 of 2")).toBeInTheDocument();
    });
  });
});
