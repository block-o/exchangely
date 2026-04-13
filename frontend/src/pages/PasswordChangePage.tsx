import './PasswordChangePage.css';
import { useCallback, useEffect, useMemo, useState, type FormEvent } from "react";
import { useAuth } from "../app/auth";
import { API_BASE_URL, authFetch } from "../api/client";
import { Button, Input, Alert } from "../components/ui";

/** Password complexity rules matching backend validation (12–128 chars, upper, lower, digit, special). */
const PASSWORD_RULES = [
  { id: "length", label: "12–128 characters", test: (p: string) => p.length >= 12 && p.length <= 128 },
  { id: "upper", label: "At least one uppercase letter", test: (p: string) => /[A-Z]/.test(p) },
  { id: "lower", label: "At least one lowercase letter", test: (p: string) => /[a-z]/.test(p) },
  { id: "digit", label: "At least one digit", test: (p: string) => /\d/.test(p) },
  { id: "special", label: "At least one special character", test: (p: string) => /[^A-Za-z0-9]/.test(p) },
] as const;

export function PasswordChangePage() {
  const { user, refreshToken } = useAuth();

  const [currentPassword, setCurrentPassword] = useState("");
  const [newPassword, setNewPassword] = useState("");
  const [confirmPassword, setConfirmPassword] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

  const isMustChange = user?.must_change_password === true;

  // Block hash-based navigation when must_change_password is true.
  useEffect(() => {
    if (!isMustChange) return;

    const handleHashChange = () => {
      if (window.location.hash !== "#change-password") {
        window.location.hash = "#change-password";
      }
    };

    window.addEventListener("hashchange", handleHashChange);
    return () => window.removeEventListener("hashchange", handleHashChange);
  }, [isMustChange]);

  /** Which rules the new password currently satisfies. */
  const ruleResults = useMemo(
    () => PASSWORD_RULES.map((rule) => ({ ...rule, met: rule.test(newPassword) })),
    [newPassword],
  );

  const allRulesMet = ruleResults.every((r) => r.met);
  const passwordsMatch = newPassword === confirmPassword;
  const canSubmit = currentPassword.length > 0 && allRulesMet && passwordsMatch && !submitting;

  const handleSubmit = useCallback(
    async (e: FormEvent) => {
      e.preventDefault();
      setError(null);

      if (!allRulesMet) {
        setError("New password does not meet complexity requirements.");
        return;
      }
      if (!passwordsMatch) {
        setError("Passwords do not match.");
        return;
      }

      setSubmitting(true);
      try {
        const response = await authFetch(`${API_BASE_URL}/auth/local/change-password`, {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          credentials: "include",
          body: JSON.stringify({ current_password: currentPassword, new_password: newPassword }),
        });

        if (!response.ok) {
          const body = await response.text();
          let message = "Failed to change password.";
          try {
            const parsed = JSON.parse(body) as { error?: string };
            if (parsed.error) message = parsed.error;
          } catch {
            // Use default message.
          }
          setError(message);
          return;
        }

        // Success — refresh the auth context (clears must_change_password) and navigate.
        await refreshToken();
        window.location.hash = "#market";
      } catch {
        setError("Network error. Please check your connection.");
      } finally {
        setSubmitting(false);
      }
    },
    [currentPassword, newPassword, allRulesMet, passwordsMatch, refreshToken],
  );

  return (
    <section className="login-page">
      <div className="login-card">
        <div className="login-header">
          <h2 className="login-title">Change Password</h2>
          <p className="login-subtitle">
            {isMustChange
              ? "You must change your password before continuing."
              : "Update your account password."}
          </p>
        </div>

        {error && (
          <Alert level="error">{error}</Alert>
        )}

        <form onSubmit={handleSubmit} className="login-form">
          <Input
            label="Current password"
            id="pw-current"
            type="password"
            autoComplete="current-password"
            required
            value={currentPassword}
            onChange={(e) => setCurrentPassword(e.target.value)}
            placeholder="••••••••••••"
          />

          <Input
            label="New password"
            id="pw-new"
            type="password"
            autoComplete="new-password"
            required
            value={newPassword}
            onChange={(e) => setNewPassword(e.target.value)}
            placeholder="••••••••••••"
          />

          {/* Rule feedback */}
          <ul className="pw-rules" aria-label="Password requirements">
            {ruleResults.map((r) => (
              <li key={r.id} className={`pw-rule ${r.met ? "pw-rule-met" : "pw-rule-unmet"}`}>
                <span className="pw-rule-icon" aria-hidden="true">
                  {r.met ? "✓" : "✗"}
                </span>
                {r.label}
              </li>
            ))}
          </ul>

          <Input
            label="Confirm new password"
            id="pw-confirm"
            type="password"
            autoComplete="new-password"
            required
            value={confirmPassword}
            onChange={(e) => setConfirmPassword(e.target.value)}
            placeholder="••••••••••••"
          />
          {confirmPassword.length > 0 && !passwordsMatch && (
            <p className="pw-mismatch">Passwords do not match.</p>
          )}

          <Button type="submit" variant="primary" className="login-btn-local" disabled={!canSubmit}>
            {submitting ? "Changing…" : "Change password"}
          </Button>
        </form>
      </div>
    </section>
  );
}
