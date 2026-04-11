import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { PasswordChangePage } from "./PasswordChangePage";
import type { User } from "../types/auth";

// --- Mock useAuth ---
const defaultUser: User = {
  id: "u-1",
  email: "admin@example.com",
  name: "Admin",
  avatar_url: "",
  role: "admin",
  must_change_password: true,
};

let mockAuthValue = {
  user: defaultUser as User | null,
  isAuthenticated: true,
  isLoading: false,
  login: vi.fn(),
  logout: vi.fn(),
  refreshToken: vi.fn().mockResolvedValue(true),
};

vi.mock("../app/auth", () => ({
  useAuth: () => mockAuthValue,
}));

// Mock authFetch
vi.mock("../api/client", async (importOriginal) => {
  const actual = await importOriginal<typeof import("../api/client")>();
  return {
    ...actual,
    authFetch: vi.fn().mockResolvedValue({ ok: true }),
  };
});

describe("PasswordChangePage", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockAuthValue = {
      user: { ...defaultUser },
      isAuthenticated: true,
      isLoading: false,
      login: vi.fn(),
      logout: vi.fn(),
      refreshToken: vi.fn().mockResolvedValue(true),
    };
  });

  it("renders the password change form", () => {
    render(<PasswordChangePage />);

    expect(screen.getByLabelText("Current password")).toBeInTheDocument();
    expect(screen.getByLabelText("New password")).toBeInTheDocument();
    expect(screen.getByLabelText("Confirm new password")).toBeInTheDocument();
    expect(screen.getByText("Change password")).toBeInTheDocument();
  });

  it("shows forced change message when must_change_password is true", () => {
    render(<PasswordChangePage />);

    expect(
      screen.getByText("You must change your password before continuing."),
    ).toBeInTheDocument();
  });

  it("shows optional change message when must_change_password is false", () => {
    mockAuthValue.user = { ...defaultUser, must_change_password: false };
    render(<PasswordChangePage />);

    expect(
      screen.getByText("Update your account password."),
    ).toBeInTheDocument();
  });

  it("displays password complexity rules", () => {
    render(<PasswordChangePage />);

    expect(screen.getByText("12–128 characters")).toBeInTheDocument();
    expect(screen.getByText("At least one uppercase letter")).toBeInTheDocument();
    expect(screen.getByText("At least one lowercase letter")).toBeInTheDocument();
    expect(screen.getByText("At least one digit")).toBeInTheDocument();
    expect(screen.getByText("At least one special character")).toBeInTheDocument();
  });

  it("marks rules as met when password satisfies them", () => {
    render(<PasswordChangePage />);

    fireEvent.change(screen.getByLabelText("New password"), {
      target: { value: "Abcdefghijk1!" },
    });

    // All rules should be met — check for ✓ markers
    const metRules = document.querySelectorAll(".pw-rule-met");
    expect(metRules.length).toBe(5);
  });

  it("marks rules as unmet for a weak password", () => {
    render(<PasswordChangePage />);

    fireEvent.change(screen.getByLabelText("New password"), {
      target: { value: "short" },
    });

    const unmetRules = document.querySelectorAll(".pw-rule-unmet");
    // "short" fails: length, uppercase, digit, special → 4 unmet
    expect(unmetRules.length).toBe(4);
  });

  it("shows mismatch message when confirm password differs", () => {
    render(<PasswordChangePage />);

    fireEvent.change(screen.getByLabelText("New password"), {
      target: { value: "Abcdefghijk1!" },
    });
    fireEvent.change(screen.getByLabelText("Confirm new password"), {
      target: { value: "different" },
    });

    expect(screen.getByText("Passwords do not match.")).toBeInTheDocument();
  });

  it("does not show mismatch when confirm is empty", () => {
    render(<PasswordChangePage />);

    fireEvent.change(screen.getByLabelText("New password"), {
      target: { value: "Abcdefghijk1!" },
    });

    expect(screen.queryByText("Passwords do not match.")).not.toBeInTheDocument();
  });

  it("disables submit when form is incomplete", () => {
    render(<PasswordChangePage />);

    const submitBtn = screen.getByText("Change password");
    expect(submitBtn).toBeDisabled();
  });

  it("enables submit when all fields are valid", () => {
    render(<PasswordChangePage />);

    fireEvent.change(screen.getByLabelText("Current password"), {
      target: { value: "oldpassword" },
    });
    fireEvent.change(screen.getByLabelText("New password"), {
      target: { value: "Abcdefghijk1!" },
    });
    fireEvent.change(screen.getByLabelText("Confirm new password"), {
      target: { value: "Abcdefghijk1!" },
    });

    const submitBtn = screen.getByText("Change password");
    expect(submitBtn).not.toBeDisabled();
  });

  it("displays server error on failed password change", async () => {
    const { authFetch } = await import("../api/client");
    (authFetch as ReturnType<typeof vi.fn>).mockResolvedValueOnce({
      ok: false,
      text: async () => JSON.stringify({ error: "invalid credentials" }),
    });

    render(<PasswordChangePage />);

    fireEvent.change(screen.getByLabelText("Current password"), {
      target: { value: "wrongpassword" },
    });
    fireEvent.change(screen.getByLabelText("New password"), {
      target: { value: "Abcdefghijk1!" },
    });
    fireEvent.change(screen.getByLabelText("Confirm new password"), {
      target: { value: "Abcdefghijk1!" },
    });
    fireEvent.click(screen.getByText("Change password"));

    await waitFor(() => {
      expect(screen.getByRole("alert")).toHaveTextContent("invalid credentials");
    });
  });
});
