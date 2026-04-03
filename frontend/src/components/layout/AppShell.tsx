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
            className="icon-btn settings-btn" 
            onClick={() => setIsSettingsOpen(true)}
            title="Settings"
            aria-label="Open Settings"
          >
            ⚙️
          </button>
        </nav>
      </header>
      <main className="content">
        {/* Simple client-side hash router for the two views */}
        {Array.isArray(children) 
          ? children.find(child => `#${child.type.name.replace('Page', '').toLowerCase()}` === activeHash) || children[0]
          : children}
      </main>
      <SettingsModal isOpen={isSettingsOpen} onClose={() => setIsSettingsOpen(false)} />
    </div>
  );
}
