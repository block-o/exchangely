import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { LoginPage } from "./LoginPage";

// --- Mock useAuth ---
const mockLogin = vi.fn();
const mockRefreshToken = vi.fn();
let mockAuthMethods: { google: boolean; local: boolean } | null = null;

vi.mock("../app/auth", () => ({
  useAuth: () => ({
    login: mockLogin,
    refreshToken: mockRefreshToken,
    user: null,
    isAuthenticated: false,
    isLoading: false,
    authEnabled: true,
    authMethods: mockAuthMethods,
    logout: vi.fn(),
  }),
}));

// --- Helpers ---

function mockFetchForLogin(loginResponse?: { ok: boolean; status?: number }) {
  vi.stubGlobal(
    "fetch",
    vi.fn((input: string | URL | Request) => {
      const url = String(input);
      if (url.includes("/auth/local/login")) {
        if (loginResponse) {
          return Promise.resolve({
            ok: loginResponse.ok,
            status: loginResponse.status ?? (loginResponse.ok ? 200 : 401),
            json: async () => ({ access_token: "tok123" }),
          });
        }
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
    mockAuthMethods = { google: true, local: false };
  });

  it("renders the Google sign-in button", async () => {
    mockAuthMethods = { google: true, local: false };
    render(<LoginPage />);
    expect(screen.getByText("Sign in with Google")).toBeInTheDocument();
  });

  it("calls login() when Google button is clicked", async () => {
    mockAuthMethods = { google: true, local: false };
    render(<LoginPage />);
    fireEvent.click(screen.getByText("Sign in with Google"));
    expect(mockLogin).toHaveBeenCalledOnce();
  });

  it("displays an error message from OAuth error redirect", async () => {
    window.location.hash = "#login?error=oauth_failed";
    mockAuthMethods = { google: true, local: false };
    render(<LoginPage />);
    expect(screen.getByRole("alert")).toHaveTextContent(
      "Google sign-in failed. Please try again.",
    );
  });

  it("displays CSRF error message", async () => {
    window.location.hash = "#login?error=csrf_failed";
    mockAuthMethods = { google: true, local: false };
    render(<LoginPage />);
    expect(screen.getByRole("alert")).toHaveTextContent(
      "Security validation failed. Please try again.",
    );
  });

  it("renders the local login form when local method is enabled", async () => {
    mockAuthMethods = { google: true, local: true };
    render(<LoginPage />);
    expect(screen.getByLabelText("Email")).toBeInTheDocument();
    expect(screen.getByLabelText("Password")).toBeInTheDocument();
    expect(screen.getByText("Sign in with email")).toBeInTheDocument();
  });

  it("does not render the local login form when local method is disabled", async () => {
    mockAuthMethods = { google: true, local: false };
    render(<LoginPage />);
    expect(screen.getByText("Sign in with Google")).toBeInTheDocument();
    expect(screen.queryByLabelText("Email")).not.toBeInTheDocument();
    expect(screen.queryByText("Sign in with email")).not.toBeInTheDocument();
  });

  it("shows rate limit error on 429 response", async () => {
    mockAuthMethods = { google: true, local: true };
    mockFetchForLogin({ ok: false, status: 429 });
    render(<LoginPage />);

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

  it("shows invalid credentials error on 401 response", async () => {
    mockAuthMethods = { google: true, local: true };
    mockFetchForLogin({ ok: false, status: 401 });
    render(<LoginPage />);

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
