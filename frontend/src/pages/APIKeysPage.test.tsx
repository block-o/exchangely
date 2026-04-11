import { render, screen, fireEvent, waitFor, within } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { APIKeysPage } from "./APIKeysPage";
import type { User } from "../types/auth";

// --- Mock useAuth ---
const defaultUser: User = {
  id: "u-1",
  email: "alice@example.com",
  name: "Alice Smith",
  avatar_url: "",
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

// --- Mock authFetch (same pattern as PasswordChangePage) ---
vi.mock("../api/client", async (importOriginal) => {
  const actual = await importOriginal<typeof import("../api/client")>();
  return {
    ...actual,
    authFetch: vi.fn(),
  };
});

// --- Test data ---
const sampleTokens = [
  {
    id: "tok-1",
    label: "My Trading Bot",
    prefix: "exly_abc",
    status: "active",
    created_at: "2024-06-01T12:00:00Z",
    last_used_at: "2024-06-10T08:30:00Z",
    revoked_at: null,
    expires_at: "2024-08-30T12:00:00Z",
  },
  {
    id: "tok-2",
    label: "CI Pipeline",
    prefix: "exly_def",
    status: "revoked",
    created_at: "2024-05-15T10:00:00Z",
    last_used_at: null,
    revoked_at: "2024-06-01T00:00:00Z",
    expires_at: "2024-08-13T10:00:00Z",
  },
];

function makeHeadersMap(
  rateLimitHeaders?: { limit: string; remaining: string; reset: string },
) {
  const map = new Map<string, string>();
  if (rateLimitHeaders) {
    map.set("X-RateLimit-Limit", rateLimitHeaders.limit);
    map.set("X-RateLimit-Remaining", rateLimitHeaders.remaining);
    map.set("X-RateLimit-Reset", rateLimitHeaders.reset);
  }
  return { get: (key: string) => map.get(key) ?? null };
}

async function setupMockAuthFetch() {
  const { authFetch } = await import("../api/client");
  return authFetch as ReturnType<typeof vi.fn>;
}

function mockFetchTokensResponse(
  mock: ReturnType<typeof vi.fn>,
  tokens = sampleTokens,
  rateLimitHeaders?: { limit: string; remaining: string; reset: string },
) {
  mock.mockResolvedValueOnce({
    ok: true,
    headers: makeHeadersMap(rateLimitHeaders),
    json: async () => ({ data: tokens }),
  });
}

describe("APIKeysPage", () => {
  let mockFetch: ReturnType<typeof vi.fn>;

  beforeEach(async () => {
    vi.clearAllMocks();
    mockFetch = await setupMockAuthFetch();
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

  // Auth guards

  it("shows loading state while auth is loading", () => {
    mockAuthValue.isLoading = true;
    render(<APIKeysPage />);
    expect(screen.getByText("Loading…")).toBeInTheDocument();
  });

  it("redirects to login when not authenticated", () => {
    mockAuthValue.user = null;
    mockAuthValue.isAuthenticated = false;
    render(<APIKeysPage />);
    expect(window.location.hash).toBe("#login");
  });

  // Token list rendering
  it("renders token list with labels and status badges", async () => {
    mockFetchTokensResponse(mockFetch);
    render(<APIKeysPage />);

    await waitFor(() => {
      expect(screen.getByText("My Trading Bot")).toBeInTheDocument();
    });

    expect(screen.getByText("CI Pipeline")).toBeInTheDocument();
    expect(screen.getByText("active")).toBeInTheDocument();
    expect(screen.getByText("revoked")).toBeInTheDocument();
  });

  it("renders masked token prefixes", async () => {
    mockFetchTokensResponse(mockFetch);
    render(<APIKeysPage />);

    await waitFor(() => {
      expect(screen.getByText("exly_abc…")).toBeInTheDocument();
    });
    expect(screen.getByText("exly_def…")).toBeInTheDocument();
  });

  it("shows empty state when no tokens exist", async () => {
    mockFetchTokensResponse(mockFetch, []);
    render(<APIKeysPage />);

    await waitFor(() => {
      expect(
        screen.getByText("No API keys yet. Create one to get started."),
      ).toBeInTheDocument();
    });
  });

  it("shows Revoke button only for active tokens", async () => {
    mockFetchTokensResponse(mockFetch);
    render(<APIKeysPage />);

    await waitFor(() => {
      expect(screen.getByText("My Trading Bot")).toBeInTheDocument();
    });

    const revokeButtons = screen.getAllByText("Revoke");
    expect(revokeButtons).toHaveLength(1);
  });

  // Token creation flow
  it("opens create modal when clicking Create API Key", async () => {
    mockFetchTokensResponse(mockFetch);
    render(<APIKeysPage />);

    await waitFor(() => {
      expect(screen.getByText("Create API Key")).toBeInTheDocument();
    });

    fireEvent.click(screen.getByText("Create API Key"));

    expect(screen.getByRole("dialog")).toBeInTheDocument();
    expect(screen.getByLabelText("Label")).toBeInTheDocument();
    expect(screen.getByText("Create Token")).toBeInTheDocument();
  });

  it("creates a token and displays the raw token with copy warning", async () => {
    mockFetchTokensResponse(mockFetch);
    render(<APIKeysPage />);

    await waitFor(() => {
      expect(screen.getByText("Create API Key")).toBeInTheDocument();
    });

    fireEvent.click(screen.getByText("Create API Key"));
    fireEvent.change(screen.getByLabelText("Label"), {
      target: { value: "New Bot Token" },
    });

    const rawToken = "exly_abc123secrettoken456";
    // Mock create response then refresh list response
    mockFetch.mockResolvedValueOnce({
      ok: true,
      json: async () => ({ token: rawToken }),
    });
    mockFetchTokensResponse(mockFetch);

    fireEvent.click(screen.getByText("Create Token"));

    await waitFor(() => {
      expect(screen.getByText(rawToken)).toBeInTheDocument();
    });

    expect(
      screen.getByText(/Copy this token now. It will not be shown again./),
    ).toBeInTheDocument();
    expect(screen.getByText("Copy")).toBeInTheDocument();
  });

  it("disables Create Token button when label is empty", async () => {
    mockFetchTokensResponse(mockFetch);
    render(<APIKeysPage />);

    await waitFor(() => {
      expect(screen.getByText("Create API Key")).toBeInTheDocument();
    });

    fireEvent.click(screen.getByText("Create API Key"));

    const submitBtn = screen.getByText("Create Token");
    expect(submitBtn).toBeDisabled();
  });

  it("shows error when token creation fails", async () => {
    mockFetchTokensResponse(mockFetch);
    render(<APIKeysPage />);

    await waitFor(() => {
      expect(screen.getByText("Create API Key")).toBeInTheDocument();
    });

    fireEvent.click(screen.getByText("Create API Key"));
    fireEvent.change(screen.getByLabelText("Label"), {
      target: { value: "Test" },
    });

    mockFetch.mockResolvedValueOnce({
      ok: false,
      status: 409,
      json: async () => ({ error: "token limit reached" }),
    });

    fireEvent.click(screen.getByText("Create Token"));

    await waitFor(() => {
      expect(screen.getByText("token limit reached")).toBeInTheDocument();
    });
  });

  it("closes create modal with Done button after token is created", async () => {
    mockFetchTokensResponse(mockFetch);
    render(<APIKeysPage />);

    await waitFor(() => {
      expect(screen.getByText("Create API Key")).toBeInTheDocument();
    });

    fireEvent.click(screen.getByText("Create API Key"));
    fireEvent.change(screen.getByLabelText("Label"), {
      target: { value: "Bot" },
    });

    mockFetch.mockResolvedValueOnce({
      ok: true,
      json: async () => ({ token: "exly_newtoken" }),
    });
    mockFetchTokensResponse(mockFetch);

    fireEvent.click(screen.getByText("Create Token"));

    await waitFor(() => {
      expect(screen.getByText("exly_newtoken")).toBeInTheDocument();
    });

    fireEvent.click(screen.getByText("Done"));
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  });

  // Token revocation flow
  it("opens revoke confirmation dialog when clicking Revoke", async () => {
    mockFetchTokensResponse(mockFetch);
    render(<APIKeysPage />);

    await waitFor(() => {
      expect(screen.getByText("My Trading Bot")).toBeInTheDocument();
    });

    fireEvent.click(screen.getByText("Revoke"));

    const dialog = screen.getByRole("dialog", { name: "Revoke API Key" });
    expect(dialog).toBeInTheDocument();
    expect(
      screen.getByText(/Are you sure you want to revoke/),
    ).toBeInTheDocument();
    expect(screen.getByText("My Trading Bot", { selector: "strong" })).toBeInTheDocument();
  });

  it("cancels revocation when clicking Cancel", async () => {
    mockFetchTokensResponse(mockFetch);
    render(<APIKeysPage />);

    await waitFor(() => {
      expect(screen.getByText("My Trading Bot")).toBeInTheDocument();
    });

    fireEvent.click(screen.getByText("Revoke"));
    fireEvent.click(screen.getByText("Cancel"));

    expect(
      screen.queryByRole("dialog", { name: "Revoke API Key" }),
    ).not.toBeInTheDocument();
  });

  it("revokes a token and updates status inline", async () => {
    mockFetchTokensResponse(mockFetch);
    render(<APIKeysPage />);

    await waitFor(() => {
      expect(screen.getByText("My Trading Bot")).toBeInTheDocument();
    });

    // Open revoke dialog
    fireEvent.click(screen.getByText("Revoke"));

    const dialog = screen.getByRole("dialog", { name: "Revoke API Key" });

    // Mock the DELETE response
    mockFetch.mockResolvedValueOnce({ ok: true, status: 204 });

    // Click the confirm button inside the dialog
    const confirmBtn = within(dialog).getByText("Revoke");
    fireEvent.click(confirmBtn);

    await waitFor(() => {
      expect(
        screen.queryByRole("dialog", { name: "Revoke API Key" }),
      ).not.toBeInTheDocument();
    });

    // Both tokens should now show "revoked" status
    const revokedBadges = screen.getAllByText("revoked");
    expect(revokedBadges).toHaveLength(2);
  });

  // Rate limit display
  it("displays rate limit usage from response headers", async () => {
    mockFetchTokensResponse(mockFetch, sampleTokens, {
      limit: "100",
      remaining: "75",
      reset: String(Math.floor(Date.now() / 1000) + 60),
    });
    render(<APIKeysPage />);

    await waitFor(() => {
      expect(screen.getByText("Rate Limit Usage")).toBeInTheDocument();
    });

    expect(screen.getByText(/25 \/ 100 requests used/)).toBeInTheDocument();
    expect(screen.getByText("75 remaining")).toBeInTheDocument();
  });

  it("does not display rate limit section when headers are absent", async () => {
    mockFetchTokensResponse(mockFetch, sampleTokens);
    render(<APIKeysPage />);

    await waitFor(() => {
      expect(screen.getByText("My Trading Bot")).toBeInTheDocument();
    });

    expect(screen.queryByText("Rate Limit Usage")).not.toBeInTheDocument();
  });

  // Error handling

  it("displays error when token fetch fails", async () => {
    mockFetch.mockRejectedValueOnce(new Error("Network error"));
    render(<APIKeysPage />);

    await waitFor(() => {
      expect(screen.getByRole("alert")).toBeInTheDocument();
    });

    expect(screen.getByText("Network error")).toBeInTheDocument();
  });
});
