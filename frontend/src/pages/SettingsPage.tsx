import './SettingsPage.css';
import { useEffect } from "react";
import { useAuth } from "../app/auth";
import { useSettings } from "../app/settings";
import { Badge, ToggleGroup } from "../components/ui";

export function SettingsPage() {
  const { user, isAuthenticated, isLoading } = useAuth();
  const { theme, setTheme, quoteCurrency, setQuoteCurrency } = useSettings();

  useEffect(() => {
    if (!isLoading && !isAuthenticated) {
      window.location.hash = "#login";
    }
  }, [isLoading, isAuthenticated]);

  if (isLoading) {
    return (
      <section className="settings-page">
        <div className="settings-loading">Loading…</div>
      </section>
    );
  }

  if (!user) {
    return null;
  }

  return (
    <section className="settings-page">
      <div className="settings-container">
        {/* Profile Section */}
        <div className="settings-panel">
          <h2 className="settings-panel-title">Profile</h2>
          <div className="settings-profile">
            <div className="settings-avatar-wrapper">
              {user.avatar_url ? (
                <img
                  src={user.avatar_url}
                  alt={`${user.name || "User"} avatar`}
                  className="settings-avatar"
                />
              ) : (
                <div className="settings-avatar settings-avatar-fallback" aria-hidden="true">
                  {(user.name || user.email).charAt(0).toUpperCase()}
                </div>
              )}
            </div>
            <div className="settings-profile-info">
              <div className="settings-name-row">
                <span className="settings-name">{user.name || "Unnamed User"}</span>
                <Badge
                  variant={user.role === "admin" ? "accent" : "default"}
                  className={`settings-role-badge ${user.role === "admin" ? "settings-role-admin" : "settings-role-user"}`}
                >
                  {user.role}
                </Badge>
              </div>
              <span className="settings-email">{user.email}</span>
            </div>
          </div>
        </div>

        {/* Connected Accounts Section */}
        <div className="settings-panel">
          <h2 className="settings-panel-title">Connected Accounts</h2>
          {user.has_google && (
            <div className="settings-account-row">
              <div className="settings-account-provider">
                <svg width="18" height="18" viewBox="0 0 24 24" aria-hidden="true">
                  <path d="M22.56 12.25c0-.78-.07-1.53-.2-2.25H12v4.26h5.92a5.06 5.06 0 0 1-2.2 3.32v2.77h3.57c2.08-1.92 3.28-4.74 3.28-8.1z" fill="#4285F4" />
                  <path d="M12 23c2.97 0 5.46-.98 7.28-2.66l-3.57-2.77c-.98.66-2.23 1.06-3.71 1.06-2.86 0-5.29-1.93-6.16-4.53H2.18v2.84C3.99 20.53 7.7 23 12 23z" fill="#34A853" />
                  <path d="M5.84 14.09c-.22-.66-.35-1.36-.35-2.09s.13-1.43.35-2.09V7.07H2.18A11.96 11.96 0 0 0 1 12c0 1.94.46 3.77 1.18 5.07l3.66-2.98z" fill="#FBBC05" />
                  <path d="M12 5.38c1.62 0 3.06.56 4.21 1.64l3.15-3.15C17.45 2.09 14.97 1 12 1 7.7 1 3.99 3.47 2.18 7.07l3.66 2.84c.87-2.6 3.3-4.53 6.16-4.53z" fill="#EA4335" />
                </svg>
                <span>Google</span>
              </div>
              <span className="settings-account-email">{user.email}</span>
            </div>
          )}
          {user.has_password && (
            <div className="settings-account-row">
              <div className="settings-account-provider">
                <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
                  <rect x="3" y="11" width="18" height="11" rx="2" ry="2" />
                  <path d="M7 11V7a5 5 0 0 1 10 0v4" />
                </svg>
                <span>Password</span>
              </div>
              <span className="settings-account-email">{user.email}</span>
            </div>
          )}
          {!user.has_google && !user.has_password && (
            <div className="settings-account-row">
              <span className="settings-pref-muted">No connected accounts</span>
            </div>
          )}
        </div>

        {/* Preferences Section */}
        <div className="settings-panel">
          <h2 className="settings-panel-title">Preferences</h2>
          <div className="settings-pref-row">
            <div className="settings-pref-label">Theme</div>
            <ToggleGroup
              options={[{ value: "dark", label: "Dark" }, { value: "light", label: "Light" }]}
              value={theme}
              onChange={(v) => setTheme(v as "dark" | "light")}
            />
          </div>
          <div className="settings-pref-row">
            <div className="settings-pref-label">Default Quote Currency</div>
            <ToggleGroup
              options={[{ value: "EUR", label: "EUR" }, { value: "USD", label: "USD" }]}
              value={quoteCurrency}
              onChange={(v) => setQuoteCurrency(v as "EUR" | "USD")}
            />
          </div>
          <div className="settings-pref-row">
            <div className="settings-pref-label">Notifications</div>
            <div className="settings-pref-value settings-pref-muted">Coming soon</div>
          </div>
        </div>
      </div>
    </section>
  );
}
