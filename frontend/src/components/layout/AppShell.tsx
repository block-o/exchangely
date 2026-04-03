import { useState, type PropsWithChildren, useEffect } from "react";
import { sections } from "../../app/router";
import { SettingsModal } from "./SettingsModal";

export function AppShell({ children }: PropsWithChildren) {
  const [activeHash, setActiveHash] = useState("");
  const [isSettingsOpen, setIsSettingsOpen] = useState(false);

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
          <h1>Market intelligence, engineered for reliability.</h1>
        </div>
        <nav className="top-nav">
          {sections.map((section) => {
            const hash = `#${section.id}`;
            return (
              <a 
                key={section.id} 
                href={hash} 
                className={`nav-item ${activeHash === hash ? 'active' : ''}`}
              >
                {section.label}
              </a>
            );
          })}
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
        </nav>
      </header>
      <main className="content">
        {/* Simple client-side hash router for the two views */}
        <div key={activeHash} className="page-transition-wrapper">
          {Array.isArray(children) 
            ? children.find(child => `#${child.type.name.replace('Page', '').toLowerCase()}` === activeHash) || children[0]
            : children}
        </div>
      </main>
      <SettingsModal isOpen={isSettingsOpen} onClose={() => setIsSettingsOpen(false)} />
    </div>
  );
}
