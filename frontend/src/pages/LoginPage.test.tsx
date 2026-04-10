import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { LoginPage } from "./LoginPage";

// --- Mock useAuth ---
const mockLogin = vi.fn();
const mockRefreshToken = vi.fn();

vi.mock("../app/auth", () => ({
  useAuth: () => ({
    login: mockLogin,
    refreshToken: mockRefreshToken,
    user: null,
    isAuthenticated: false,
    isLoading: false,
    logout: vi.fn(),
  }),
}));

// --- Helpers ---

function mockFetchResponses(methods: { google: boolean; local: boolean }) {
  vi.stubGlobal(
    "fetch",
    vi.fn((input: string | URL | Request) => {
      const url = String(input);
      if (url.includes("/auth/methods")) {
        return Promise.resolve({
          ok: true,
          json: async () => methods,
        });
      }
      if (url.includes("/auth/local/login")) {
        return Promise.resolve({
          ok: true,
          json: async () => ({ access_token: "tok123" }),
        });
      }
      return Promise.resolve({ ok: true, json: async () => ({}) });
    }),
  );
}

describe("LoginPage", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    window.location.hash = "#login";
  });

  /**
   * Validates: Requirements 7.1
   * Google sign-in button is always rendered when google method is available.
   */
  it("renders the Google sign-in button", async () => {
    mockFetchResponses({ google: true, local: false });
    render(<LoginPage />);

    await waitFor(() => {
      expect(screen.getByText("Sign in with Google")).toBeInTheDocument();
    });
  });

  /**
   * Validates: Requirements 7.1
   * Clicking the Google button calls the login function (redirect to OAuth).
   */
  it("calls login() when Google button is clicked", async () => {
    mockFetchResponses({ google: true, local: false });
    render(<LoginPage />);

    await waitFor(() => {
      expect(screen.getByText("Sign in with Google")).toBeInTheDocument();
    });

    fireEvent.click(screen.getByText("Sign in with Google"));
    expect(mockLogin).toHaveBeenCalledOnce();
  });

  /**
   * Validates: Requirements 7.6
   * When the hash contains an OAuth error, a user-friendly message is displayed.
   */
  it("displays an error message from OAuth error redirect", async () => {
    window.location.hash = "#login?error=oauth_failed";
    mockFetchResponses({ google: true, local: false });
    render(<LoginPage />);

    await waitFor(() => {
      expect(screen.getByRole("alert")).toHaveTextContent(
        "Google sign-in failed. Please try again.",
      );
    });
  });

  it("displays CSRF error message", async () => {
    window.location.hash = "#login?error=csrf_failed";
    mockFetchResponses({ google: true, local: false });
    render(<LoginPage />);

    await waitFor(() => {
      expect(screen.getByRole("alert")).toHaveTextContent(
        "Security validation failed. Please try again.",
      );
    });
  });

  /**
   * Validates: Requirements 11.12
   * Local login form is shown only when the local method is enabled.
   */
  it("renders the local login form when local method is enabled", async () => {
    mockFetchResponses({ google: true, local: true });
    render(<LoginPage />);

    await waitFor(() => {
      expect(screen.getByLabelText("Email")).toBeInTheDocument();
      expect(screen.getByLabelText("Password")).toBeInTheDocument();
      expect(screen.getByText("Sign in with email")).toBeInTheDocument();
    });
  });

  it("does not render the local login form when local method is disabled", async () => {
    mockFetchResponses({ google: true, local: false });
    render(<LoginPage />);

    // Wait for methods to load
    await waitFor(() => {
      expect(screen.getByText("Sign in with Google")).toBeInTheDocument();
    });

    expect(screen.queryByLabelText("Email")).not.toBeInTheDocument();
    expect(screen.queryByText("Sign in with email")).not.toBeInTheDocument();
  });

  /**
   * Validates: Requirements 7.6
   * Local login shows error on 429 (rate limited).
   */
  it("shows rate limit error on 429 response", async () => {
    mockFetchResponses({ google: true, local: true });

    // Override fetch for the login call
    const originalFetch = globalThis.fetch;
    vi.stubGlobal(
      "fetch",
      vi.fn((input: string | URL | Request) => {
        const url = String(input);
        if (url.includes("/auth/local/login")) {
          return Promise.resolve({ ok: false, status: 429 });
        }
        return (originalFetch as any)(input);
      }),
    );

    // Re-stub to handle both methods and login
    vi.stubGlobal(
      "fetch",
      vi.fn((input: string | URL | Request) => {
        const url = String(input);
        if (url.includes("/auth/methods")) {
          return Promise.resolve({
            ok: true,
            json: async () => ({ google: true, local: true }),
          });
        }
        if (url.includes("/auth/local/login")) {
          return Promise.resolve({ ok: false, status: 429 });
        }
        return Promise.resolve({ ok: true, json: async () => ({}) });
      }),
    );

    render(<LoginPage />);

    await waitFor(() => {
      expect(screen.getByText("Sign in with email")).toBeInTheDocument();
    });

    fireEvent.change(screen.getByLabelText("Email"), {
      target: { value: "admin@example.com" },
    });
    fireEvent.change(screen.getByLabelText("Password"), {
      target: { value: "password123" },
    });
    fireEvent.click(screen.getByText("Sign in with email"));

    await waitFor(() => {
      expect(screen.getByRole("alert")).toHaveTextContent(
        "Too many login attempts",
      );
    });
  });

  /**
   * Validates: Requirements 7.6
   * Local login shows generic error on 401 response.
   */
  it("shows invalid credentials error on 401 response", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn((input: string | URL | Request) => {
        const url = String(input);
        if (url.includes("/auth/methods")) {
          return Promise.resolve({
            ok: true,
            json: async () => ({ google: true, local: true }),
          });
        }
        if (url.includes("/auth/local/login")) {
          return Promise.resolve({ ok: false, status: 401 });
        }
        return Promise.resolve({ ok: true, json: async () => ({}) });
      }),
    );

    render(<LoginPage />);

    await waitFor(() => {
      expect(screen.getByText("Sign in with email")).toBeInTheDocument();
    });

    fireEvent.change(screen.getByLabelText("Email"), {
      target: { value: "admin@example.com" },
    });
    fireEvent.change(screen.getByLabelText("Password"), {
      target: { value: "wrongpassword" },
    });
    fireEvent.click(screen.getByText("Sign in with email"));

    await waitFor(() => {
      expect(screen.getByRole("alert")).toHaveTextContent(
        "Invalid email or password",
      );
    });
  });
});
