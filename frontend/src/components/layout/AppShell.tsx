import { Children, useState, useRef, useCallback, type PropsWithChildren, useEffect } from "react";
import { sections } from "../../app/router";
import { API_BASE_URL } from "../../api/client";
import { useAuth } from "../../app/auth";
import { useSettings } from "../../app/settings";
import { LoginPage } from "../../pages/LoginPage";
import { SettingsPage } from "../../pages/SettingsPage";
import { PasswordChangePage } from "../../pages/PasswordChangePage";
import { APIKeysPage } from "../../pages/APIKeysPage";
import { NavigationDrawer } from "./NavigationDrawer";
import { useBreakpoint } from "../../hooks/useBreakpoint";

function getApiDocsUrl(theme: string) {
  try {
    const apiUrl = new URL(API_BASE_URL);
    const url = new URL("/swagger", apiUrl.origin);
    url.searchParams.set("theme", theme);
    return url.toString();
  } catch {
    return `/swagger?theme=${theme}`;
  }
}

/** Build the visible nav items based on auth state. */
function getNavItems(isAuthenticated: boolean, role: string | undefined, authEnabled: boolean) {
  const items: { id: string; label: string }[] = [{ id: "market", label: "Market" }];
  // Show Portfolio for authenticated users with user, premium, or admin role
  if (isAuthenticated && (role === "user" || role === "premium" || role === "admin")) {
    items.push({ id: "portfolio", label: "Portfolio" });
  }
  // Show Operations when auth is disabled (everyone has full access)
  // or when the authenticated user is an admin.
  if (!authEnabled || (isAuthenticated && role === "admin")) {
    items.push({ id: "system", label: "Operations" });
  }
  return items;
}

const GithubIcon = ({ size = 16 }: { size?: number }) => (
  <svg xmlns="http://www.w3.org/2000/svg" width={size} height={size} viewBox="0 0 24 24" fill="currentColor" aria-hidden="true">
    <path d="M12 1.5a10.5 10.5 0 0 0-3.32 20.46c.53.1.72-.23.72-.51v-1.98c-2.94.64-3.56-1.25-3.56-1.25-.48-1.22-1.18-1.54-1.18-1.54-.96-.65.07-.64.07-.64 1.06.07 1.62 1.08 1.62 1.08.94 1.61 2.47 1.14 3.07.87.1-.68.37-1.14.67-1.4-2.35-.27-4.82-1.17-4.82-5.22 0-1.15.41-2.08 1.08-2.82-.11-.27-.47-1.37.1-2.86 0 0 .88-.28 2.89 1.08a10 10 0 0 1 5.26 0c2.01-1.36 2.89-1.08 2.89-1.08.57 1.49.21 2.59.1 2.86.67.74 1.08 1.67 1.08 2.82 0 4.06-2.47 4.94-4.83 5.21.38.33.72.98.72 1.98v2.93c0 .28.19.62.73.51A10.5 10.5 0 0 0 12 1.5Z" />
  </svg>
);

const ApiIcon = ({ size = 16 }: { size?: number }) => (
  <svg xmlns="http://www.w3.org/2000/svg" width={size} height={size} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
    <polyline points="16 18 22 12 16 6" />
    <polyline points="8 6 2 12 8 18" />
  </svg>
);

const GearIcon = ({ size = 16 }: { size?: number }) => (
  <svg xmlns="http://www.w3.org/2000/svg" width={size} height={size} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
    <circle cx="12" cy="12" r="3" />
    <path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 0 1-2.83 2.83l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-4 0v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 0 1-2.83-2.83l.06-.06A1.65 1.65 0 0 0 4.68 15a1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1 0-4h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 0 1 2.83-2.83l.06.06A1.65 1.65 0 0 0 9 4.68a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 4 0v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 0 1 2.83 2.83l-.06.06A1.65 1.65 0 0 0 19.4 9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 0 4h-.09a1.65 1.65 0 0 0-1.51 1z" />
  </svg>
);

const HamburgerIcon = () => (
  <svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
    <line x1="3" y1="6" x2="21" y2="6" />
    <line x1="3" y1="12" x2="21" y2="12" />
    <line x1="3" y1="18" x2="21" y2="18" />
  </svg>
);

/** Inline settings controls (theme + currency) reused in desktop dropdown and mobile bottom bar. */
function SettingsControls({ theme, setTheme, quoteCurrency, setQuoteCurrency }: {
  theme: string;
  setTheme: (t: "dark" | "light") => void;
  quoteCurrency: string;
  setQuoteCurrency: (c: "EUR" | "USD") => void;
}) {
  return (
    <>
      <div className="setting-group">
        <label>Theme</label>
        <div className="toggle-group">
          <button className={theme === "dark" ? "active" : ""} onClick={() => setTheme("dark")}>Dark</button>
          <button className={theme === "light" ? "active" : ""} onClick={() => setTheme("light")}>Light</button>
        </div>
      </div>
      <div className="setting-group" style={{ marginBottom: 0 }}>
        <label>Currency</label>
        <div className="toggle-group">
          <button className={quoteCurrency === "EUR" ? "active" : ""} onClick={() => setQuoteCurrency("EUR")}>EUR</button>
          <button className={quoteCurrency === "USD" ? "active" : ""} onClick={() => setQuoteCurrency("USD")}>USD</button>
        </div>
      </div>
    </>
  );
}

export function AppShell({ children }: PropsWithChildren) {
  const { user, isAuthenticated, isLoading, authEnabled, logout } = useAuth();
  const { theme, setTheme, quoteCurrency, setQuoteCurrency } = useSettings();
  const [activeHash, setActiveHash] = useState("");
  const [isDrawerOpen, setIsDrawerOpen] = useState(false);
  const [isGearDropdownOpen, setIsGearDropdownOpen] = useState(false);
  const hamburgerRef = useRef<HTMLButtonElement>(null);
  const gearDropdownRef = useRef<HTMLDivElement>(null);
  const breakpoint = useBreakpoint();
  const pages = Children.toArray(children);
  const apiDocsUrl = getApiDocsUrl(theme);

  const navItems = getNavItems(isAuthenticated, user?.role, authEnabled);

  const handleNavigate = useCallback((hash: string) => {
    window.location.hash = hash;
    setActiveHash(hash);
    setIsDrawerOpen(false);
  }, []);

  const handleDrawerClose = useCallback(() => {
    setIsDrawerOpen(false);
  }, []);

  const handleLogout = useCallback(async () => {
    setIsGearDropdownOpen(false);
    await logout();
    window.location.hash = "#market";
  }, [logout]);

  // Sync hash on mount and changes
  useEffect(() => {
    setActiveHash(window.location.hash || "#market");
    const handleHashChange = () => setActiveHash(window.location.hash || "#market");
    window.addEventListener("hashchange", handleHashChange);
    return () => window.removeEventListener("hashchange", handleHashChange);
  }, []);

  // Force redirect to #change-password when must_change_password is true
  useEffect(() => {
    if (user?.must_change_password && activeHash !== "#change-password") {
      window.location.hash = "#change-password";
      setActiveHash("#change-password");
    }
  }, [user?.must_change_password, activeHash]);

  // Close gear dropdown on outside click
  useEffect(() => {
    if (!isGearDropdownOpen) return;
    const handleClick = (e: MouseEvent) => {
      if (gearDropdownRef.current && !gearDropdownRef.current.contains(e.target as Node)) {
        setIsGearDropdownOpen(false);
      }
    };
    document.addEventListener("mousedown", handleClick);
    return () => document.removeEventListener("mousedown", handleClick);
  }, [isGearDropdownOpen]);

  // Resolve which page to render for the current hash
  const renderPage = () => {
    if (activeHash === "#login") return <LoginPage />;
    if (activeHash === "#settings") return <SettingsPage />;
    if (activeHash === "#change-password") return <PasswordChangePage />;
    if (activeHash === "#api-keys") return <APIKeysPage />;
    const idx = sections.findIndex((s) => `#${s.id}` === activeHash);
    return pages[idx] ?? pages[0] ?? null;
  };

  if (isLoading) {
    return (
      <div className="app-shell">
        <div className="auth-loading-spinner">
          <div className="auth-spinner" />
          <span>Loading…</span>
        </div>
      </div>
    );
  }

  return (
    <div className="app-shell">
      <header className="hero">
        <div className="hero-text">
          <p className="eyebrow">Exchangely</p>
        </div>
        <button
          ref={hamburgerRef}
          className="hamburger-btn mobile-only"
          onClick={() => setIsDrawerOpen(true)}
          aria-expanded={isDrawerOpen}
          aria-controls="nav-drawer"
          aria-label="Open navigation menu"
        >
          <HamburgerIcon />
        </button>
        <div className="hero-actions">
          <nav className="top-nav tablet-up">
            {navItems.map((item) => {
              const hash = `#${item.id}`;
              return (
                <a key={item.id} href={hash} className={`nav-item ${activeHash === hash ? "active" : ""}`}>
                  {item.label}
                </a>
              );
            })}
          </nav>
          <div className="top-links tablet-up">
            {/* GitHub */}
            <a className="icon-link" href="https://github.com/block-o/exchangely" target="_blank" rel="noreferrer" title="GitHub" aria-label="GitHub project">
              <GithubIcon />
            </a>
            {/* API Docs */}
            <a className="icon-link" href={apiDocsUrl} target="_blank" rel="noreferrer" title="API Docs" aria-label="API documentation">
              <ApiIcon />
            </a>
            {/* Identity pill: [ text | ⚙ ] */}
            <div className="identity-pill" ref={gearDropdownRef}>
              {isAuthenticated && user ? (
                <a href="#settings" className="identity-pill-label" aria-label="Profile">
                  {user.name || user.email}
                </a>
              ) : authEnabled ? (
                <a href="#login" className="identity-pill-label" aria-label="Sign in">
                  Sign in
                </a>
              ) : null}
              <button
                className="identity-pill-gear"
                onClick={() => setIsGearDropdownOpen((prev) => !prev)}
                aria-expanded={isGearDropdownOpen}
                aria-haspopup="true"
                aria-label="Settings"
                title="Settings"
              >
                <GearIcon />
              </button>
              {isGearDropdownOpen && (
                <div className="gear-dropdown" role="menu" aria-label="Settings">
                  <SettingsControls theme={theme} setTheme={setTheme} quoteCurrency={quoteCurrency} setQuoteCurrency={setQuoteCurrency} />
                  {isAuthenticated && user && (
                    <>
                      <div className="gear-dropdown-divider" />
                      <div className="gear-dropdown-user">
                        <a href="#settings" className="avatar-dropdown-item" role="menuitem" onClick={() => setIsGearDropdownOpen(false)}>
                          Profile
                        </a>
                        <a href="#api-keys" className="avatar-dropdown-item" role="menuitem" onClick={() => setIsGearDropdownOpen(false)}>
                          API Keys
                        </a>
                        <button className="avatar-dropdown-item" role="menuitem" onClick={handleLogout}>
                          Logout
                        </button>
                      </div>
                    </>
                  )}
                </div>
              )}
            </div>
          </div>
        </div>
      </header>

      <main className="content">
        <div key={activeHash} className="page-transition-wrapper">
          {renderPage()}
        </div>
      </main>

      {/* Mobile bottom bar */}
      {breakpoint === "mobile" && (
        <nav className="mobile-bottom-bar" aria-label="Quick actions">
          <a className="bottom-bar-item" href="https://github.com/block-o/exchangely" target="_blank" rel="noreferrer" aria-label="GitHub project">
            <GithubIcon size={20} />
            <span className="bottom-bar-label">GitHub</span>
          </a>
          <a className="bottom-bar-item" href={apiDocsUrl} target="_blank" rel="noreferrer" aria-label="API documentation">
            <ApiIcon size={20} />
            <span className="bottom-bar-label">API</span>
          </a>
          {/* Fused gear — settings + user in one dropdown */}
          <div className="bottom-bar-item-wrapper" ref={breakpoint === "mobile" ? gearDropdownRef : undefined}>
            <button
              className={`bottom-bar-item${isGearDropdownOpen ? " active" : ""}`}
              onClick={() => setIsGearDropdownOpen((prev) => !prev)}
              aria-expanded={isGearDropdownOpen}
              aria-haspopup="true"
              aria-label="Settings"
            >
              <GearIcon size={20} />
              <span className="bottom-bar-label">{isAuthenticated && user ? "Settings" : "Menu"}</span>
            </button>
            {isGearDropdownOpen && (
              <div className="gear-dropdown gear-dropdown-mobile" role="menu" aria-label="Settings">
                <SettingsControls theme={theme} setTheme={setTheme} quoteCurrency={quoteCurrency} setQuoteCurrency={setQuoteCurrency} />
                <div className="gear-dropdown-divider" />
                {isAuthenticated && user ? (
                  <div className="gear-dropdown-user">
                    <div className="gear-dropdown-user-info">
                      {user.avatar_url ? (
                        <img src={user.avatar_url} alt="" className="avatar-img-small" />
                      ) : (
                        <span className="avatar-fallback" aria-hidden="true" style={{ width: 24, height: 24, fontSize: "0.7rem" }}>
                          {(user.name || user.email).charAt(0).toUpperCase()}
                        </span>
                      )}
                      <span className="gear-dropdown-user-name">{user.name || user.email}</span>
                    </div>
                    <a href="#settings" className="avatar-dropdown-item" role="menuitem" onClick={() => setIsGearDropdownOpen(false)}>
                      Profile
                    </a>
                    <a href="#api-keys" className="avatar-dropdown-item" role="menuitem" onClick={() => setIsGearDropdownOpen(false)}>
                      API Keys
                    </a>
                    <button className="avatar-dropdown-item" role="menuitem" onClick={handleLogout}>
                      Logout
                    </button>
                  </div>
                ) : authEnabled ? (
                  <div className="gear-dropdown-guest">
                    <span className="guest-nudge-hint">Sign in for alerts and more.</span>
                    <a href="#login" className="guest-signin-btn" role="menuitem" onClick={() => setIsGearDropdownOpen(false)}>
                      Sign in
                    </a>
                  </div>
                ) : null}
              </div>
            )}
          </div>
        </nav>
      )}

      <NavigationDrawer
        isOpen={isDrawerOpen}
        onClose={handleDrawerClose}
        activeHash={activeHash}
        onNavigate={handleNavigate}
        hamburgerRef={hamburgerRef}
        navItems={navItems}
      />
    </div>
  );
}
