import { Children, useState, useRef, useCallback, type PropsWithChildren, useEffect } from "react";
import { sections } from "../../app/router";
import { API_BASE_URL } from "../../api/client";
import { SettingsModal } from "./SettingsModal";
import { NavigationDrawer } from "./NavigationDrawer";
import { useBreakpoint } from "../../hooks/useBreakpoint";

function getApiDocsUrl() {
  try {
    const apiUrl = new URL(API_BASE_URL);
    return new URL("/swagger", apiUrl.origin).toString();
  } catch {
    return "/swagger";
  }
}

export function AppShell({ children }: PropsWithChildren) {
  const [activeHash, setActiveHash] = useState("");
  const [isSettingsOpen, setIsSettingsOpen] = useState(false);
  const [isDrawerOpen, setIsDrawerOpen] = useState(false);
  const hamburgerRef = useRef<HTMLButtonElement>(null);
  const breakpoint = useBreakpoint();
  const pages = Children.toArray(children);
  const apiDocsUrl = getApiDocsUrl();

  const handleNavigate = useCallback((hash: string) => {
    window.location.hash = hash;
    setActiveHash(hash);
    setIsDrawerOpen(false);
  }, []);

  const handleDrawerClose = useCallback(() => {
    setIsDrawerOpen(false);
  }, []);

  useEffect(() => {
    setActiveHash(window.location.hash || "#market");

    const handleHashChange = () => {
      setActiveHash(window.location.hash || "#market");
    };

    window.addEventListener("hashchange", handleHashChange);
    return () => window.removeEventListener("hashchange", handleHashChange);
  }, []);

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
          <svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
            <line x1="3" y1="6" x2="21" y2="6" />
            <line x1="3" y1="12" x2="21" y2="12" />
            <line x1="3" y1="18" x2="21" y2="18" />
          </svg>
        </button>
        <div className="hero-actions">
          <nav className="top-nav tablet-up">
            {sections.map((section) => {
              const hash = `#${section.id}`;
              return (
                <a
                  key={section.id}
                  href={hash}
                  className={`nav-item ${activeHash === hash ? "active" : ""}`}
                >
                  {section.label}
                </a>
              );
            })}
            <a className="nav-item external-nav-item" href={apiDocsUrl} target="_blank" rel="noreferrer">
              API Docs
            </a>
          </nav>
          <div className="top-links tablet-up">
            <a
              className="icon-link"
              href="https://github.com/block-o/exchangely"
              target="_blank"
              rel="noreferrer"
              title="GitHub"
              aria-label="GitHub project"
            >
              <svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true">
                <path d="M12 1.5a10.5 10.5 0 0 0-3.32 20.46c.53.1.72-.23.72-.51v-1.98c-2.94.64-3.56-1.25-3.56-1.25-.48-1.22-1.18-1.54-1.18-1.54-.96-.65.07-.64.07-.64 1.06.07 1.62 1.08 1.62 1.08.94 1.61 2.47 1.14 3.07.87.1-.68.37-1.14.67-1.4-2.35-.27-4.82-1.17-4.82-5.22 0-1.15.41-2.08 1.08-2.82-.11-.27-.47-1.37.1-2.86 0 0 .88-.28 2.89 1.08a10 10 0 0 1 5.26 0c2.01-1.36 2.89-1.08 2.89-1.08.57 1.49.21 2.59.1 2.86.67.74 1.08 1.67 1.08 2.82 0 4.06-2.47 4.94-4.83 5.21.38.33.72.98.72 1.98v2.93c0 .28.19.62.73.51A10.5 10.5 0 0 0 12 1.5Z" />
              </svg>
            </a>
            <button
              className="settings-trigger"
              onClick={() => setIsSettingsOpen(true)}
              title="Settings"
              aria-label="Open Settings"
            >
              <svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                <circle cx="12" cy="12" r="3"></circle>
                <path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 0 1 0 2.83 2 2 0 0 1-2.83 0l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-2 2 2 2 0 0 1-2-2v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 0 1-2.83 0 2 2 0 0 1 0-2.83l.06-.06a1.65 1.65 0 0 0 .33-1.82 1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1-2-2 2 2 0 0 1 2-2h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 0 1 0-2.83 2 2 0 0 1 2.83 0l.06.06a1.65 1.65 0 0 0 1.82.33H9a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 2-2 2 2 0 0 1 2 2v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 0 1 2.83 0 2 2 0 0 1 0 2.83l-.06.06a1.65 1.65 0 0 0-.33 1.82V9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 2 2 2 2 0 0 1-2 2h-.09a1.65 1.65 0 0 0-1.51 1z"></path>
              </svg>
            </button>
          </div>
        </div>
      </header>
      <main className="content">
        {/* Route by configured section order so production minification cannot break page matching. */}
        <div key={activeHash} className="page-transition-wrapper">
          {pages[sections.findIndex((section) => `#${section.id}` === activeHash)] ?? pages[0] ?? null}
        </div>
      </main>
      <SettingsModal isOpen={isSettingsOpen} onClose={() => setIsSettingsOpen(false)} />
      <NavigationDrawer
        isOpen={isDrawerOpen}
        onClose={handleDrawerClose}
        activeHash={activeHash}
        onNavigate={handleNavigate}
        hamburgerRef={hamburgerRef}
      />
    </div>
  );
}
