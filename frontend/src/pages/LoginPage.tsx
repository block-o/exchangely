import { useCallback, useEffect, useState, type FormEvent } from "react";
import { useAuth } from "../app/auth";
import { API_BASE_URL, setAccessToken } from "../api/client";
import type { TokenResponse } from "../types/auth";

/** Map backend error codes from the URL hash to user-friendly messages. */
function friendlyError(code: string): string {
  switch (code) {
    case "oauth_failed":
      return "Google sign-in failed. Please try again.";
    case "csrf_failed":
      return "Security validation failed. Please try again.";
    case "access_denied":
      return "Access was denied. Please contact an administrator.";
    default:
      return "An error occurred during sign-in. Please try again.";
  }
}

/** Extract the `error` param from a hash like `#login?error=oauth_failed`. */
function parseHashError(): string | null {
  const hash = window.location.hash;
  const qIdx = hash.indexOf("?");
  if (qIdx === -1) return null;
  const params = new URLSearchParams(hash.slice(qIdx + 1));
  return params.get("error");
}

export function LoginPage() {
  const { login, refreshToken, authMethods } = useAuth();
  const [oauthError, setOauthError] = useState<string | null>(null);
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [localError, setLocalError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

  // Parse any OAuth error from the URL hash on mount.
  useEffect(() => {
    const errCode = parseHashError();
    if (errCode) setOauthError(friendlyError(errCode));
  }, []);

  const handleLocalLogin = useCallback(
    async (e: FormEvent) => {
      e.preventDefault();
      setLocalError(null);
      setSubmitting(true);

      try {
        const res = await fetch(`${API_BASE_URL}/auth/local/login`, {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          credentials: "include",
          body: JSON.stringify({ email, password }),
        });

        if (!res.ok) {
          if (res.status === 429) {
            setLocalError("Too many login attempts. Please wait and try again.");
          } else {
            setLocalError("Invalid email or password.");
          }
          return;
        }

        const data = (await res.json()) as TokenResponse;
        setAccessToken(data.access_token);

        // Refresh auth context so the app picks up the new session.
        await refreshToken();

        // Navigate away from login.
        window.location.hash = "#market";
      } catch {
        setLocalError("Network error. Please check your connection.");
      } finally {
        setSubmitting(false);
      }
    },
    [email, password, refreshToken],
  );

  const displayError = oauthError ?? localError;

  return (
    <section className="login-page">
      <div className="login-card">
        <div className="login-header">
          <h2 className="login-title">Welcome to Exchangely</h2>
          <p className="login-subtitle">Sign in to access your dashboard</p>
        </div>

        {displayError && (
          <div className="login-error" role="alert">
            {displayError}
          </div>
        )}

        {/* Google OAuth button — only when SSO is enabled */}
        {authMethods?.google && (
          <button
            type="button"
            className="login-btn login-btn-google"
            onClick={login}
          >
            <svg
              width="18"
              height="18"
              viewBox="0 0 24 24"
              aria-hidden="true"
            >
              <path
                d="M22.56 12.25c0-.78-.07-1.53-.2-2.25H12v4.26h5.92a5.06 5.06 0 0 1-2.2 3.32v2.77h3.57c2.08-1.92 3.28-4.74 3.28-8.1z"
                fill="#4285F4"
              />
              <path
                d="M12 23c2.97 0 5.46-.98 7.28-2.66l-3.57-2.77c-.98.66-2.23 1.06-3.71 1.06-2.86 0-5.29-1.93-6.16-4.53H2.18v2.84C3.99 20.53 7.7 23 12 23z"
                fill="#34A853"
              />
              <path
                d="M5.84 14.09c-.22-.66-.35-1.36-.35-2.09s.13-1.43.35-2.09V7.07H2.18A11.96 11.96 0 0 0 1 12c0 1.94.46 3.77 1.18 5.07l3.66-2.98z"
                fill="#FBBC05"
              />
              <path
                d="M12 5.38c1.62 0 3.06.56 4.21 1.64l3.15-3.15C17.45 2.09 14.97 1 12 1 7.7 1 3.99 3.47 2.18 7.07l3.66 2.84c.87-2.6 3.3-4.53 6.16-4.53z"
                fill="#EA4335"
              />
            </svg>
            Sign in with Google
          </button>
        )}

        {/* Local email/password form — only when local auth is enabled */}
        {authMethods?.local && (
          <>
            {authMethods?.google && (
              <div className="login-divider">
                <span>or</span>
              </div>
            )}
            <form onSubmit={handleLocalLogin} className="login-form">
              <label className="login-label" htmlFor="login-email">
                Email
              </label>
              <input
                id="login-email"
                className="login-input"
                type="email"
                autoComplete="email"
                required
                value={email}
                onChange={(e) => setEmail(e.target.value)}
                placeholder="admin@example.com"
              />
              <label className="login-label" htmlFor="login-password">
                Password
              </label>
              <input
                id="login-password"
                className="login-input"
                type="password"
                autoComplete="current-password"
                required
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                placeholder="••••••••••••"
              />
              <button
                type="submit"
                className="login-btn login-btn-local"
                disabled={submitting}
              >
                {submitting ? "Signing in…" : "Sign in with email"}
              </button>
            </form>
          </>
        )}
      </div>
    </section>
  );
}
